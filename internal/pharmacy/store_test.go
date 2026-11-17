package pharmacy

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/platform/storage"
	"healthcare-platform/internal/user"
)

type fixture struct {
	pool  *pgxpool.Pool
	store *Store
}

var seq = 0

func phone() string {
	seq++
	return fmt.Sprintf("0171%07d", seq)
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := dbtest.New(t)
	files, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return fixture{pool: pool, store: NewStore(pool, files, 300)}
}

func ptr[T any](v T) *T { return &v }

// Dhanmondi, Dhaka.
const homeLat, homeLng = 23.7461, 90.3742

type ph struct{ id, owner string }

func (f fixture) pharmacy(t *testing.T, name string, lat, lng, radius float64, delivery bool) ph {
	t.Helper()
	ctx := context.Background()
	owner := dbtest.CreateUser(t, f.pool, name+" owner", phone(), user.RolePharmacy)
	p, err := f.store.Create(ctx, owner, PharmacyInput{
		Name: name, DrugLicenseNo: "DL-" + name, Address: "Road 2, Dhanmondi", District: "Dhaka", Phone: "01700000000",
		Latitude: &lat, Longitude: &lng, DeliveryRadiusKm: radius, OffersDelivery: &delivery,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Verify(ctx, p.ID, VerifyInput{Status: StatusApproved}); err != nil {
		t.Fatal(err)
	}
	return ph{p.ID, owner}
}

func (f fixture) exec(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var id string
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func (f fixture) prescriptionFor(t *testing.T, patientID string) string {
	t.Helper()
	doc := dbtest.CreateUser(t, f.pool, "Dr Rahman", phone(), user.RoleDoctor)
	cid := f.exec(t, `INSERT INTO consultations (patient_id, doctor_id, subject, description, fee_tokens, commission_bps, status)
		VALUES ($1, $2, 'Fever', 'Fever', 1, 5000, 'completed') RETURNING id`, patientID, doc)
	return f.exec(t, `INSERT INTO prescriptions (code, doctor_id, patient_id, consultation_id, doctor_name, doctor_bmdc, patient_name, medicines)
		VALUES (md5(random()::text), $1, $2, $3, 'Dr Rahman', 'A-1', 'Karim',
		'[{"name":"Napa","strength":"500 mg","form":"Tab.","dosage":"1+1+1","duration":"5 days"},
		  {"name":"Fexo","strength":"120 mg","form":"Tab.","dosage":"0+0+1","duration":"7 days"}]') RETURNING id`,
		doc, patientID, cid)
}

func (f fixture) notifications(t *testing.T, userID, kind string) int {
	t.Helper()
	var n int
	f.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE user_id = $1 AND kind = $2`, userID, kind).Scan(&n)
	return n
}

func wantKind(t *testing.T, err error, k apperr.Kind, what string) {
	t.Helper()
	if !apperr.Is(err, k) {
		t.Fatalf("%s: got error %v, want kind %d", what, err, k)
	}
}

func deliveryRequest(rxID string) RequestInput {
	return RequestInput{PrescriptionID: rxID, Fulfilment: FulfilDelivery, Address: "House 5, Road 7, Dhanmondi",
		Latitude: ptr(homeLat), Longitude: ptr(homeLng)}
}

func quote(available int64, unavailable bool, delivery int64) QuoteInput {
	items := []QuotedItem{{Name: "Napa", Strength: "500 mg", Quantity: "15 tablets", Available: true, PricePaisa: available}}
	if unavailable {
		items = append(items, QuotedItem{Name: "Fexo", Strength: "120 mg", Quantity: "7 tablets"})
	}
	return QuoteInput{Items: items, DeliveryChargePaisa: delivery, EtaMinutes: 45}
}

func TestDeliveryByRider(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	near := f.pharmacy(t, "Lazz Pharma", 23.7465, 90.3750, 5, true) // ~0.1 km
	far := f.pharmacy(t, "Far Pharma", 23.7800, 90.4000, 3, true)   // ~4.5 km, radius 3
	noDelivery := f.pharmacy(t, "Counter Only", 23.7500, 90.3800, 5, false)
	second := f.pharmacy(t, "Tamanna Pharmacy", 23.7470, 90.3760, 5, true)
	patient := dbtest.CreateUser(t, f.pool, "Karim Uddin", "01811111111", user.RolePatient)
	rx := f.prescriptionFor(t, patient)

	// Nobody serves Chattogram yet.
	ctg := deliveryRequest(rx)
	ctg.Latitude, ctg.Longitude = ptr(22.3569), ptr(91.7832)
	_, err := f.store.CreateRequest(ctx, patient, ctg, nil)
	wantKind(t, err, apperr.KindConflict, "request with no pharmacy nearby")

	rq, err := f.store.CreateRequest(ctx, patient, deliveryRequest(rx), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rq.Items) != 2 || rq.Items[0].Quantity != "1+1+1, 5 days" || rq.ContactPhone != "01811111111" || rq.Prescription == nil {
		t.Fatalf("request = %+v", rq)
	}
	if f.notifications(t, near.owner, "medicine_request") != 1 || f.notifications(t, far.owner, "medicine_request") != 0 ||
		f.notifications(t, noDelivery.owner, "medicine_request") != 0 {
		t.Fatal("request was announced to the wrong pharmacies")
	}

	// Pharmacies see the request without who or where exactly.
	feed, err := f.store.Feed(ctx, near.id, 20, 0)
	if err != nil || len(feed) != 1 || feed[0].Address != nil || feed[0].ContactPhone != "" || feed[0].DistanceKm == nil {
		t.Fatalf("feed = %+v, %v", feed, err)
	}
	if list, _ := f.store.Feed(ctx, far.id, 20, 0); len(list) != 0 {
		t.Fatal("far pharmacy sees the request")
	}
	if list, _ := f.store.Feed(ctx, noDelivery.id, 20, 0); len(list) != 0 {
		t.Fatal("non-delivering pharmacy sees a delivery request")
	}
	_, err = f.store.SubmitQuote(ctx, far.id, rq.ID, quote(15000, true, 3000))
	wantKind(t, err, apperr.KindNotFound, "far pharmacy quotes")

	bad := quote(15000, true, 3000)
	bad.Items[1].PricePaisa = 500
	_, err = f.store.SubmitQuote(ctx, near.id, rq.ID, bad)
	wantKind(t, err, apperr.KindValidation, "price on an unavailable item")
	none := QuoteInput{Items: []QuotedItem{{Name: "Napa"}}, EtaMinutes: 30}
	_, err = f.store.SubmitQuote(ctx, near.id, rq.ID, none)
	wantKind(t, err, apperr.KindValidation, "nothing available")

	if _, err := f.store.SubmitQuote(ctx, near.id, rq.ID, quote(16000, true, 3000)); err != nil {
		t.Fatal(err)
	}
	q, err := f.store.SubmitQuote(ctx, near.id, rq.ID, quote(15000, true, 3000)) // revised
	if err != nil || q.TotalPaisa != 18000 || q.AvailableCount != 1 {
		t.Fatalf("quote = %+v, %v", q, err)
	}
	q2, err := f.store.SubmitQuote(ctx, second.id, rq.ID, quote(14000, false, 4000))
	if err != nil {
		t.Fatal(err)
	}

	mine, _ := f.store.RequestForPatient(ctx, patient, rq.ID)
	if len(mine.Quotes) != 2 || mine.Quotes[0].ID != q.ID {
		t.Fatalf("patient sees %d quotes, cheapest first = %v", len(mine.Quotes), mine.Quotes)
	}

	other := dbtest.CreateUser(t, f.pool, "Someone", "01822222222", user.RolePatient)
	_, err = f.store.AcceptQuote(ctx, other, q.ID)
	wantKind(t, err, apperr.KindNotFound, "another patient accepts")

	order, err := f.store.AcceptQuote(ctx, patient, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != OrderConfirmed || order.TotalPaisa != 18000 || !regexp.MustCompile(`^\d{6}$`).MatchString(order.DeliveryCode) {
		t.Fatalf("order = %+v", order)
	}
	code := order.DeliveryCode
	_, err = f.store.AcceptQuote(ctx, patient, q2.ID)
	wantKind(t, err, apperr.KindConflict, "accept a second quote")
	if po, _ := f.store.OrderForPharmacy(ctx, near.id, order.ID); po.DeliveryCode != "" {
		t.Fatal("pharmacy can see the delivery code")
	}
	if f.notifications(t, near.owner, "medicine_order") != 1 {
		t.Fatal("pharmacy not told about the order")
	}

	// Pharmacy prepares and hands it to the rider network.
	for _, step := range []string{OrderPreparing, OrderReady} {
		if _, err := f.store.Advance(ctx, near.id, near.owner, order.ID, step); err != nil {
			t.Fatal(err)
		}
	}
	_, err = f.store.Complete(ctx, near.id, near.owner, order.ID, code)
	wantKind(t, err, apperr.KindConflict, "complete an undispatched delivery")
	if _, err := f.store.Dispatch(ctx, near.id, near.owner, order.ID, ModeRider); err != nil {
		t.Fatal(err)
	}

	// Riders.
	r1 := dbtest.CreateUser(t, f.pool, "Rider One", phone(), user.RoleRider)
	r2 := dbtest.CreateUser(t, f.pool, "Rider Two", phone(), user.RoleRider)
	_, err = f.store.RegisterRider(ctx, r1, RiderInput{NIDNumber: "12345", Vehicle: "motorcycle"})
	wantKind(t, err, apperr.KindValidation, "bad NID")
	for i, r := range []string{r1, r2} {
		if _, err := f.store.RegisterRider(ctx, r, RiderInput{NIDNumber: fmt.Sprintf("199012345678%d", i), Vehicle: "motorcycle"}); err != nil {
			t.Fatal(err)
		}
	}
	onDuty := AvailabilityInput{Available: true, Latitude: ptr(23.7480), Longitude: ptr(90.3770)}
	_, err = f.store.SetAvailability(ctx, r1, onDuty)
	wantKind(t, err, apperr.KindForbidden, "unverified rider goes on duty")
	for _, r := range []string{r1, r2} {
		if _, err := f.store.VerifyRider(ctx, r, VerifyInput{Status: StatusApproved}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.SetAvailability(ctx, r, onDuty); err != nil {
			t.Fatal(err)
		}
	}
	jobs, err := f.store.Jobs(ctx, r1)
	if err != nil || len(jobs) != 1 || jobs[0].EarningPaisa != 3000 || jobs[0].CashToCollectPaisa != 18000 {
		t.Fatalf("jobs = %+v, %v", jobs, err)
	}
	if _, err := f.store.AcceptJob(ctx, r1, order.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.AcceptJob(ctx, r2, order.ID)
	wantKind(t, err, apperr.KindConflict, "second rider takes the same job")
	_, err = f.store.Deliver(ctx, r1, order.ID, code)
	wantKind(t, err, apperr.KindConflict, "deliver before pickup")
	if _, err := f.store.PickedUp(ctx, r1, order.ID); err != nil {
		t.Fatal(err)
	}
	tracked, _ := f.store.OrderForPatient(ctx, patient, order.ID)
	if tracked.Status != OrderOutForDelivery || tracked.RiderLocation == nil || *tracked.RiderName != "Rider One" {
		t.Fatalf("tracking = %+v", tracked)
	}

	_, err = f.store.Deliver(ctx, r1, order.ID, "000000")
	if code == "000000" {
		err = errWrongCode
	}
	wantKind(t, err, apperr.KindValidation, "wrong delivery code")
	done, err := f.store.Deliver(ctx, r1, order.ID, code)
	if err != nil || done.Status != OrderDelivered {
		t.Fatalf("deliver: %+v, %v", done, err)
	}
	po, _ := f.store.OrderForPharmacy(ctx, near.id, order.ID)
	if po.CommissionPaisa == nil || *po.CommissionPaisa != 450 || len(po.Events) < 6 {
		t.Fatalf("pharmacy view after delivery: commission=%v events=%d", po.CommissionPaisa, len(po.Events))
	}

	if err := f.store.Review(ctx, patient, order.ID, ReviewInput{Rating: 5, Comment: "Fast delivery"}); err != nil {
		t.Fatal(err)
	}
	wantKind(t, f.store.Review(ctx, patient, order.ID, ReviewInput{Rating: 4}), apperr.KindConflict, "review twice")
	p, reviews, _ := f.store.Public(ctx, near.id)
	if p.Rating == nil || *p.Rating != 5 || len(reviews) != 1 {
		t.Fatalf("rating = %v, reviews = %d", p.Rating, len(reviews))
	}

	today := time.Now().In(dhaka).Format(time.DateOnly)
	st, err := f.store.Statement(ctx, near.id, today, today)
	if err != nil || st.DeliveredOrders != 1 || st.CommissionDuePaisa != 450 || st.MedicinesTotalPaisa != 15000 {
		t.Fatalf("statement = %+v, %v", st, err)
	}
	if e, _ := f.store.Earnings(ctx, r1); e.Deliveries != 1 || e.EarningPaisa != 3000 {
		t.Fatalf("rider earnings = %+v", e)
	}

	nearby, err := f.store.Nearby(ctx, homeLat, homeLng, 2, 20, 0)
	if err != nil || len(nearby) != 3 || nearby[0].ID != near.id {
		t.Fatalf("nearby = %d, %v", len(nearby), err)
	}
}

func TestPickupUploadAndCancellations(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	counter := f.pharmacy(t, "Counter Only", 23.7465, 90.3750, 5, false)
	far := f.pharmacy(t, "Far Pharma", 23.9000, 90.5000, 2, true)
	patient := dbtest.CreateUser(t, f.pool, "Nusrat", "01833333333", user.RolePatient)

	pickup := RequestInput{Fulfilment: FulfilPickup, Latitude: ptr(homeLat), Longitude: ptr(homeLng),
		Items: []Item{{Name: "Seclo", Strength: "20 mg", Quantity: "14 capsules"}}}
	pdf := "%PDF-1.4 scanned prescription"

	both := pickup
	both.PrescriptionID = "6f1c1c3e-0000-4000-8000-000000000000"
	_, err := f.store.CreateRequest(ctx, patient, both, &Upload{ContentType: "application/pdf", Body: strings.NewReader(pdf)})
	wantKind(t, err, apperr.KindValidation, "prescription id and upload together")

	rq, err := f.store.CreateRequest(ctx, patient, pickup, &Upload{ContentType: "application/pdf", Body: strings.NewReader(pdf)})
	if err != nil || !rq.HasAttachment {
		t.Fatalf("upload request: %+v, %v", rq, err)
	}
	ct, rc, err := f.store.OpenAttachment(ctx, "", counter.id, rq.ID)
	if err != nil || ct != "application/pdf" {
		t.Fatalf("pharmacy opens upload: %s, %v", ct, err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if string(body) != pdf {
		t.Fatalf("upload content = %q", body)
	}
	_, _, err = f.store.OpenAttachment(ctx, "", far.id, rq.ID)
	wantKind(t, err, apperr.KindNotFound, "far pharmacy opens upload")

	_, err = f.store.SubmitQuote(ctx, counter.id, rq.ID, quote(28000, false, 2000))
	wantKind(t, err, apperr.KindValidation, "delivery charge on pickup")
	q, err := f.store.SubmitQuote(ctx, counter.id, rq.ID, quote(28000, false, 0))
	if err != nil {
		t.Fatal(err)
	}
	order, err := f.store.AcceptQuote(ctx, patient, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.Dispatch(ctx, counter.id, counter.owner, order.ID, ModeRider)
	wantKind(t, err, apperr.KindConflict, "dispatch a pickup order")
	if _, err := f.store.Advance(ctx, counter.id, counter.owner, order.ID, OrderReady); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.CancelByPatient(ctx, patient, order.ID, "changed my mind")
	wantKind(t, err, apperr.KindConflict, "patient cancels a ready order")

	// Five wrong codes lock the handover, even for the right code afterwards.
	wrong := "111111"
	if order.DeliveryCode == wrong {
		wrong = "222222"
	}
	for range MaxCodeAttempts {
		_, err := f.store.Complete(ctx, counter.id, counter.owner, order.ID, wrong)
		wantKind(t, err, apperr.KindValidation, "wrong code")
	}
	_, err = f.store.Complete(ctx, counter.id, counter.owner, order.ID, order.DeliveryCode)
	wantKind(t, err, apperr.KindConflict, "right code after lockout")

	// Patient cancels early; pharmacy cancels another with a reason.
	mk := func() Order {
		rq, err := f.store.CreateRequest(ctx, patient, pickup, &Upload{ContentType: "image/png", Body: strings.NewReader("\x89PNG..")})
		if err != nil {
			t.Fatal(err)
		}
		q, err := f.store.SubmitQuote(ctx, counter.id, rq.ID, quote(28000, false, 0))
		if err != nil {
			t.Fatal(err)
		}
		o, err := f.store.AcceptQuote(ctx, patient, q.ID)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	o1 := mk()
	if o, err := f.store.CancelByPatient(ctx, patient, o1.ID, ""); err != nil || o.Status != OrderCancelled || *o.CancelledBy != "patient" {
		t.Fatalf("patient cancel: %+v, %v", o, err)
	}
	o2 := mk()
	_, err = f.store.CancelByPharmacy(ctx, counter.id, counter.owner, o2.ID, "no")
	wantKind(t, err, apperr.KindValidation, "cancel reason too short")
	if o, err := f.store.CancelByPharmacy(ctx, counter.id, counter.owner, o2.ID, "Seclo is out of stock"); err != nil || *o.CancelledBy != "pharmacy" {
		t.Fatalf("pharmacy cancel: %+v, %v", o, err)
	}

	// At most three open requests.
	for range MaxOpenRequests {
		if _, err := f.store.CreateRequest(ctx, patient, pickup, &Upload{ContentType: "image/png", Body: strings.NewReader("x")}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = f.store.CreateRequest(ctx, patient, pickup, &Upload{ContentType: "image/png", Body: strings.NewReader("x")})
	wantKind(t, err, apperr.KindConflict, "fourth open request")
}

func TestConcurrentAcceptsCreateOneOrder(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	patient := dbtest.CreateUser(t, f.pool, "Rahim", "01844444444", user.RolePatient)
	rq, err := func() (Request, error) {
		for i := range 4 {
			f.pharmacy(t, fmt.Sprintf("Pharmacy %d", i), 23.7465+float64(i)*0.001, 90.3750, 5, true)
		}
		return f.store.CreateRequest(ctx, patient, deliveryRequest(f.prescriptionFor(t, patient)), nil)
	}()
	if err != nil {
		t.Fatal(err)
	}
	var quotes []string
	rows, _ := f.pool.Query(ctx, `SELECT id FROM pharmacies`)
	for rows.Next() {
		var id string
		rows.Scan(&id)
		q, err := f.store.SubmitQuote(ctx, id, rq.ID, quote(10000, false, 2000))
		if err != nil {
			t.Fatal(err)
		}
		quotes = append(quotes, q.ID)
	}
	rows.Close()

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for _, id := range quotes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.store.AcceptQuote(ctx, patient, id)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
			} else if !apperr.Is(err, apperr.KindConflict) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	var orders int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM medicine_orders`).Scan(&orders)
	if ok != 1 || orders != 1 {
		t.Fatalf("accepted=%d orders=%d; want 1 and 1", ok, orders)
	}
}
