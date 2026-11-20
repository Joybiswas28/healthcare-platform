package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	legacyrouter "github.com/getkin/kin-openapi/routers/legacy"
	"golang.org/x/crypto/bcrypt"

	"healthcare-platform/internal/appointment"
	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/consultation"
	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/hospital"
	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/pharmacy"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/platform/storage"
	"healthcare-platform/internal/prescription"
	"healthcare-platform/internal/realtime"
	"healthcare-platform/internal/user"
	"healthcare-platform/internal/wallet"
)

// smsBox keeps sent SMS so the test can read OTP codes.
type smsBox struct {
	mu   sync.Mutex
	last map[string]string
}

func (s *smsBox) Send(_ context.Context, phone, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last[phone] = text
	return nil
}

var otpPattern = regexp.MustCompile(`\b\d{6}\b`)

func (s *smsBox) code(phone string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return otpPattern.FindString(s.last[phone])
}

// specValidator checks every request and response against api/openapi.yaml.
type specValidator struct {
	t    *testing.T
	next http.Handler
	find func(*http.Request) (*openapi3filter.RequestValidationInput, error)
	seen map[string]bool
	mu   sync.Mutex
}

func newSpecValidator(t *testing.T, next http.Handler) *specValidator {
	doc := loadSpec(t)
	doc.Servers = nil // match any host
	router, err := legacyrouter.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	v := &specValidator{t: t, next: next, seen: map[string]bool{}}
	v.find = func(r *http.Request) (*openapi3filter.RequestValidationInput, error) {
		route, params, err := router.FindRoute(r)
		if err != nil {
			return nil, err
		}
		return &openapi3filter.RequestValidationInput{
			Request: r, PathParams: params, Route: route,
			Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		}, nil
	}
	return v
}

func (v *specValidator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))

	check := r.Clone(r.Context())
	check.Body = io.NopCloser(bytes.NewReader(body))
	in, err := v.find(check)
	if err != nil {
		v.t.Errorf("%s %s: not in the spec: %v", r.Method, r.URL.Path, err)
		v.next.ServeHTTP(w, r)
		return
	}
	v.mu.Lock()
	v.seen[r.Method+" "+in.Route.Path] = true
	v.mu.Unlock()

	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if err := openapi3filter.ValidateRequest(r.Context(), in); err != nil {
			v.t.Errorf("%s %s: request does not match the spec: %v", r.Method, r.URL.Path, err)
		}
	}

	rec := httptest.NewRecorder()
	v.next.ServeHTTP(rec, r)
	out := rec.Body.Bytes()

	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") || rec.Code == http.StatusNoContent {
		resp := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: in, Status: rec.Code, Header: rec.Header(),
			Options: &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
		}
		resp.SetBodyBytes(out)
		if err := openapi3filter.ValidateResponse(r.Context(), resp); err != nil {
			v.t.Errorf("%s %s -> %d: response does not match the spec:\n%v\nbody: %.400s", r.Method, r.URL.Path, rec.Code, err, out)
		}
	}
	for k, vals := range rec.Header() {
		w.Header()[k] = vals
	}
	w.WriteHeader(rec.Code)
	_, _ = w.Write(out)
}

type client struct {
	t   *testing.T
	url string
}

// call sends a JSON request and decodes a JSON object response, failing unless the
// status matches.
func (c client) call(method, path, token string, body any, want int) map[string]any {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.url+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, want, raw)
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func str(m map[string]any, keys ...string) string {
	var cur any = m
	for _, k := range keys {
		switch x := cur.(type) {
		case map[string]any:
			cur = x[k]
		case []any:
			var i int
			fmt.Sscan(k, &i)
			cur = x[i]
		}
	}
	s, _ := cur.(string)
	return s
}

// TestResponsesMatchSpec drives the main journeys of every role through the real
// router and database, validating each request and response against the spec.
func TestResponsesMatchSpec(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	files, _ := storage.NewLocal(t.TempDir())
	sms := &smsBox{last: map[string]string{}}

	users := user.NewRepository(pool)
	sessions := auth.NewSessionChecker(users)
	tokens := auth.NewTokenManager("test-secret-test-secret-test-secret", time.Hour)
	otp := auth.NewOTPStore(pool, sms, users, "test-secret-test-secret-test-secret")
	ds, as, ws := doctor.NewStore(pool), appointment.NewStore(pool), wallet.NewStore(pool)
	router := NewRouter(Deps{
		Log: log, DB: pool, Tokens: tokens, Sessions: sessions,
		Auth:          auth.NewHandler(auth.NewService(users, tokens, otp), otp, log).WithSessions(sessions),
		AdminUsers:    auth.NewAdminHandler(users, sessions, log),
		Doctors:       doctor.NewHandler(ds, log),
		Appointments:  appointment.NewHandler(as, log),
		Wallet:        wallet.NewHandler(ws, wallet.DevGateway{}, log),
		Consultations: consultation.NewHandler(consultation.NewStore(pool, files), realtime.NewHub(), log),
		Prescriptions: prescription.NewHandler(prescription.NewStore(pool), "http://test", log),
		Hospitals:     hospital.NewHandler(hospital.NewStore(pool, ds, files), ds, as, log),
		Pharmacies:    pharmacy.NewHandler(pharmacy.NewStore(pool, files, 300), log),
		Notifications: notify.NewHandler(notify.NewStore(pool), log),
		DevRoutes:     true,
	})
	v := newSpecValidator(t, router)
	srv := httptest.NewServer(v)
	defer srv.Close()
	c := client{t: t, url: srv.URL}
	const api = "/api/v1"

	register := func(name, phone, role string) (token, id string) {
		c.call("POST", api+"/auth/otp", "", map[string]any{"phone": phone, "purpose": "register"}, 202)
		res := c.call("POST", api+"/auth/register", "", map[string]any{
			"name": name, "phone": phone, "password": "secret123", "role": role, "otp_code": sms.code(phone),
		}, 201)
		return str(res, "access_token"), str(res, "user", "id")
	}
	pt, patientID := register("Karim Uddin", "01811111111", "patient")
	dt, doctorID := register("Dr Ayesha Siddiqua", "01711111111", "doctor")
	ht, _ := register("Square Admin", "01611111111", "hospital_admin")
	pht, _ := register("Lazz Owner", "01511111111", "pharmacy")
	rt, riderID := register("Rashed Rider", "01911111111", "rider")

	hash, _ := bcrypt.GenerateFromPassword([]byte("admin-pass-123"), bcrypt.MinCost)
	if _, err := users.Create(ctx, user.User{Name: "Admin", Phone: "01311111111", PasswordHash: string(hash), Role: user.RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	at := str(c.call("POST", api+"/auth/login", "", map[string]any{"phone": "01311111111", "password": "admin-pass-123"}, 200), "access_token")
	c.call("GET", api+"/me", pt, nil, 200)
	c.call("GET", api+"/doctors/not-a-uuid", "", nil, 422)

	// Account settings: profile, then a password change that signs out the old
	// token but keeps this client logged in with the new one.
	c.call("PUT", api+"/me", pt, map[string]any{"name": "Karim Uddin", "email": "karim@example.com"}, 200)
	c.call("PUT", api+"/me", pt, map[string]any{"name": "Karim Uddin", "email": "not-an-email"}, 422)
	c.call("POST", api+"/me/password", pt, map[string]any{"current_password": "wrong-one", "new_password": "brand-new-pass"}, 422)
	oldPT := pt
	pt = str(c.call("POST", api+"/me/password", pt, map[string]any{"current_password": "secret123", "new_password": "brand-new-pass"}, 200), "access_token")
	c.call("GET", api+"/me", oldPT, nil, 401)
	c.call("GET", api+"/me", pt, nil, 200)

	// Doctor onboarding and chamber booking.
	c.call("PUT", api+"/doctor/profile", dt, map[string]any{"bmdc_reg_no": "A-54321", "specialization": "Medicine", "degrees": "MBBS, FCPS", "experience_years": 7}, 200)
	c.call("GET", api+"/admin/doctors?status=pending", at, nil, 200)
	c.call("POST", api+"/admin/doctors/"+doctorID+"/verification", at, map[string]any{"status": "approved"}, 200)
	c.call("PUT", api+"/admin/doctors/"+doctorID+"/commission", at, map[string]any{"commission_bps": 5000}, 200)
	c.call("PUT", api+"/doctor/consultation-settings", dt, map[string]any{"enabled": true, "fee_tokens": 10}, 200)
	ch := c.call("POST", api+"/doctor/chambers", dt, map[string]any{"name": "Green Life", "address": "Road 2, Dhanmondi", "district": "Dhaka", "fee_paisa": 80000}, 201)
	day := time.Now().In(appointment.Dhaka).AddDate(0, 0, 7)
	date := day.Format(time.DateOnly)
	sc := c.call("POST", api+"/doctor/chambers/"+str(ch, "id")+"/schedules", dt, map[string]any{"weekday": int(day.Weekday()), "start_time": "17:00", "end_time": "21:00", "max_patients": 20}, 201)
	c.call("GET", api+"/doctor/chambers", dt, nil, 200)
	c.call("GET", api+"/doctor/profile", dt, nil, 200)
	c.call("GET", api+"/doctors?name=ayesha", "", nil, 200)
	c.call("GET", api+"/doctors/"+doctorID, "", nil, 200)
	c.call("GET", api+"/schedules/"+str(sc, "id")+"/availability?date="+date, "", nil, 200)
	appt := c.call("POST", api+"/appointments", pt, map[string]any{"schedule_id": str(sc, "id"), "date": date}, 201)
	c.call("GET", api+"/appointments", pt, nil, 200)
	c.call("GET", api+"/doctor/appointments?date="+date, dt, nil, 200)
	c.call("GET", api+"/doctor/tickets/"+str(appt, "ticket_code"), dt, nil, 200)
	c.call("POST", api+"/appointments/"+str(appt, "id")+"/cancel", pt, nil, 200)

	// Tokens and a private consultation.
	pkgs := c.call("GET", api+"/token-packages", "", nil, 200)
	pkg := pkgs["packages"].([]any)[0].(map[string]any)["id"].(string)
	pay := c.call("POST", api+"/wallet/purchases", pt, map[string]any{"package_id": pkg}, 201)
	c.call("POST", api+"/dev/payments/"+str(pay, "id")+"/succeed", pt, nil, 200)
	c.call("GET", api+"/wallet/purchases/"+str(pay, "id"), pt, nil, 200)
	c.call("GET", api+"/wallet", pt, nil, 200)
	cons := c.call("POST", api+"/consultations", pt, map[string]any{"doctor_id": doctorID, "subject": "Fever for 3 days", "description": "High fever since Saturday with body ache."}, 201)
	cid := str(cons, "id")
	c.call("POST", api+"/consultations/"+cid+"/accept", dt, nil, 200)
	c.call("POST", api+"/consultations/"+cid+"/confirm", pt, nil, 200)
	c.call("POST", api+"/consultations/"+cid+"/messages", pt, map[string]any{"body": "Temperature is 102F"}, 201)
	c.call("GET", api+"/consultations/"+cid+"/messages", dt, nil, 200)
	c.call("GET", api+"/consultations?status=active", dt, nil, 200)
	rx := c.call("POST", api+"/prescriptions", dt, map[string]any{
		"consultation_id": cid, "patient_age": "32 years", "patient_gender": "male", "diagnosis": "Viral fever",
		"medicines":      []map[string]any{{"name": "Napa", "strength": "500 mg", "form": "Tab.", "dosage": "1+1+1", "duration": "5 days"}},
		"investigations": []string{"CBC"}, "advice": "প্রচুর পানি পান করুন",
	}, 201)
	c.call("POST", api+"/consultations/"+cid+"/complete", dt, nil, 200)
	c.call("GET", api+"/consultations/"+cid, pt, nil, 200)
	c.call("GET", api+"/prescriptions", pt, nil, 200)
	c.call("GET", api+"/prescriptions/"+str(rx, "id"), pt, nil, 200)
	c.call("GET", api+"/prescriptions/verify/"+str(rx, "code"), "", nil, 200)
	c.call("GET", api+"/wallet/history", pt, nil, 200)

	// Earnings and a withdrawal (10 tokens at 50% = BDT 500).
	c.call("GET", api+"/doctor/earnings", dt, nil, 200)
	wd := c.call("POST", api+"/doctor/withdrawals", dt, map[string]any{"amount_paisa": 50000, "method": "bkash", "account_details": "01711111111"}, 201)
	c.call("GET", api+"/doctor/withdrawals", dt, nil, 200)
	c.call("GET", api+"/admin/withdrawals?status=requested", at, nil, 200)
	c.call("POST", api+"/admin/withdrawals/"+str(wd, "id")+"/process", at, map[string]any{"status": "paid", "payout_ref": "BKASH-1"}, 200)

	// Public concern and offer (patient buys more tokens first).
	pay = c.call("POST", api+"/wallet/purchases", pt, map[string]any{"package_id": pkg}, 201)
	c.call("POST", api+"/dev/payments/"+str(pay, "id")+"/succeed", pt, nil, 200)
	concern := c.call("POST", api+"/concerns", pt, map[string]any{"title": "Ear pain", "description": "My child has ear pain since last night.", "specialization": "ENT"}, 201)
	c.call("GET", api+"/concerns", dt, nil, 200)
	offer := c.call("POST", api+"/concerns/"+str(concern, "id")+"/offers", dt, map[string]any{"message": "I can review this today."}, 201)
	c.call("GET", api+"/concerns/"+str(concern, "id"), pt, nil, 200)
	c.call("GET", api+"/concerns/"+str(concern, "id"), dt, nil, 200)
	c.call("GET", api+"/doctor/offers", dt, nil, 200)
	c.call("POST", api+"/offers/"+str(offer, "id")+"/accept", pt, nil, 201)
	c.call("GET", api+"/notifications", pt, nil, 200)

	// Hospital.
	lat, lng := 23.7525, 90.3816
	h := c.call("POST", api+"/hospitals", ht, map[string]any{"name": "Square Hospitals", "registration_no": "DGHS-1", "address": "West Panthapath, Dhaka", "district": "Dhaka", "phone": "10616", "departments": []string{"Medicine"}, "facilities": []string{"ICU"}, "latitude": lat, "longitude": lng}, 201)
	hid := str(h, "id")
	c.call("GET", api+"/admin/hospitals?status=pending", at, nil, 200)
	c.call("POST", api+"/admin/hospitals/"+hid+"/verification", at, map[string]any{"status": "approved"}, 200)
	c.call("GET", api+"/manage/hospitals", ht, nil, 200)
	c.call("POST", api+"/manage/hospitals/"+hid+"/doctors", ht, map[string]any{"bmdc_reg_no": "A-54321", "department": "Medicine", "designation": "Consultant"}, 201)
	c.call("GET", api+"/doctor/hospitals", dt, nil, 200)
	c.call("POST", api+"/doctor/hospitals/"+hid+"/accept", dt, nil, 204)
	hch := c.call("POST", api+"/manage/hospitals/"+hid+"/doctors/"+doctorID+"/chambers", ht, map[string]any{"fee_paisa": 150000, "room": "Room 412"}, 201)
	c.call("POST", api+"/manage/hospitals/"+hid+"/chambers/"+str(hch, "id")+"/schedules", ht, map[string]any{"weekday": int(day.Weekday()), "start_time": "10:00", "end_time": "12:00", "max_patients": 15}, 201)
	c.call("GET", api+"/manage/hospitals/"+hid+"/doctors", ht, nil, 200)
	c.call("GET", api+"/manage/hospitals/"+hid+"/appointments?date="+date, ht, nil, 200)
	c.call("GET", api+"/manage/hospitals/"+hid+"/members", ht, nil, 200)
	c.call("GET", api+"/manage/hospitals/"+hid+"/photos", ht, nil, 200)
	c.call("GET", api+"/hospitals/"+hid, "", nil, 200)
	c.call("GET", api+"/hospitals?department=medicine", "", nil, 200)

	// Pharmacy order delivered by a rider.
	ph := c.call("POST", api+"/pharmacies", pht, map[string]any{"name": "Lazz Pharma", "drug_license_no": "DGDA-1", "address": "Road 7A, Dhanmondi", "district": "Dhaka", "phone": "01711000000", "latitude": 23.7465, "longitude": 90.3750}, 201)
	phid := str(ph, "id")
	c.call("GET", api+"/admin/pharmacies?status=pending", at, nil, 200)
	c.call("POST", api+"/admin/pharmacies/"+phid+"/verification", at, map[string]any{"status": "approved"}, 200)
	req := c.call("POST", api+"/medicine-requests", pt, map[string]any{"prescription_id": str(rx, "id"), "fulfilment": "delivery", "address": "House 5, Road 7, Dhanmondi", "latitude": 23.7461, "longitude": 90.3742}, 201)
	c.call("GET", api+"/manage/pharmacies/"+phid+"/requests", pht, nil, 200)
	c.call("GET", api+"/manage/pharmacies/"+phid+"/requests/"+str(req, "id"), pht, nil, 200)
	q := c.call("POST", api+"/manage/pharmacies/"+phid+"/requests/"+str(req, "id")+"/quote", pht, map[string]any{
		"items":                 []map[string]any{{"name": "Napa", "strength": "500 mg", "quantity": "15 tablets", "available": true, "price_paisa": 1500}},
		"delivery_charge_paisa": 4000, "eta_minutes": 40,
	}, 201)
	c.call("GET", api+"/medicine-requests/"+str(req, "id"), pt, nil, 200)
	c.call("GET", api+"/medicine-requests", pt, nil, 200)
	order := c.call("POST", api+"/quotes/"+str(q, "id")+"/accept", pt, nil, 201)
	oid := str(order, "id")
	for _, step := range []string{"preparing", "ready"} {
		c.call("POST", api+"/manage/pharmacies/"+phid+"/orders/"+oid+"/"+step, pht, nil, 200)
	}
	c.call("POST", api+"/manage/pharmacies/"+phid+"/orders/"+oid+"/dispatch", pht, map[string]any{"mode": "rider"}, 200)
	c.call("PUT", api+"/rider/profile", rt, map[string]any{"nid_number": "1990123456789", "vehicle": "motorcycle"}, 200)
	c.call("GET", api+"/admin/riders?status=pending", at, nil, 200)
	c.call("POST", api+"/admin/riders/"+riderID+"/verification", at, map[string]any{"status": "approved"}, 200)
	c.call("PUT", api+"/rider/availability", rt, map[string]any{"available": true, "latitude": 23.748, "longitude": 90.377}, 200)
	c.call("GET", api+"/rider/jobs", rt, nil, 200)
	c.call("POST", api+"/rider/jobs/"+oid+"/accept", rt, nil, 200)
	c.call("POST", api+"/rider/orders/"+oid+"/picked-up", rt, nil, 200)
	c.call("PUT", api+"/rider/location", rt, map[string]any{"latitude": 23.747, "longitude": 90.3745}, 204)
	c.call("GET", api+"/medicine-orders/"+oid, pt, nil, 200)
	c.call("POST", api+"/rider/orders/"+oid+"/delivered", rt, map[string]any{"code": str(order, "delivery_code")}, 200)
	c.call("POST", api+"/medicine-orders/"+oid+"/review", pt, map[string]any{"rating": 5, "comment": "Fast"}, 204)
	c.call("GET", api+"/medicine-orders", pt, nil, 200)
	c.call("GET", api+"/manage/pharmacies/"+phid+"/orders?status=active", pht, nil, 200)
	c.call("GET", api+"/manage/pharmacies/"+phid+"/orders/"+oid, pht, nil, 200)
	today := time.Now().In(appointment.Dhaka).Format(time.DateOnly)
	c.call("GET", api+"/manage/pharmacies/"+phid+"/statement?from="+today+"&to="+today, pht, nil, 200)
	c.call("GET", api+"/rider/orders", rt, nil, 200)
	c.call("GET", api+"/rider/earnings", rt, nil, 200)
	c.call("GET", api+"/rider/profile", rt, nil, 200)
	c.call("GET", api+"/pharmacies?latitude=23.7461&longitude=90.3742", "", nil, 200)
	c.call("GET", api+"/pharmacies/"+phid, "", nil, 200)

	// Account search and suspension.
	c.call("GET", api+"/admin/users?q=017&role=patient", at, nil, 200)
	c.call("POST", api+"/admin/users/"+patientID+"/status", at, map[string]any{"status": "suspended"}, 200)
	c.call("GET", api+"/me", pt, nil, 401)

	t.Logf("validated %d distinct operations against the spec", len(v.seen))
}
