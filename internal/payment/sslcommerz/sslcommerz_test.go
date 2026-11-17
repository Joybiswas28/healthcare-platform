package sslcommerz

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/user"
	"healthcare-platform/internal/wallet"
)

// fakeSSLCommerz mimics the two SSLCommerz endpoints we call.
type fakeSSLCommerz struct {
	mu          sync.Mutex
	sessions    map[string]url.Values // tran_id -> init form
	validations map[string]Validation // val_id -> response
	refuse      bool
}

func (f *fakeSSLCommerz) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/gwprocess/v4/api.php":
		_ = r.ParseForm()
		if r.PostForm.Get("store_id") != "teststore" || r.PostForm.Get("store_passwd") != "secret" {
			json.NewEncoder(w).Encode(map[string]string{"status": "FAILED", "failedreason": "Store Credential Error"})
			return
		}
		if f.refuse {
			json.NewEncoder(w).Encode(map[string]string{"status": "FAILED", "failedreason": "maintenance"})
			return
		}
		f.sessions[r.PostForm.Get("tran_id")] = r.PostForm
		json.NewEncoder(w).Encode(map[string]string{
			"status":         "SUCCESS",
			"GatewayPageURL": "https://sandbox.example/pay/" + r.PostForm.Get("tran_id"),
		})
	case "/validator/api/validationserverAPI.php":
		q := r.URL.Query()
		if q.Get("store_id") != "teststore" || q.Get("store_passwd") != "secret" {
			http.Error(w, "bad credentials", http.StatusForbidden)
			return
		}
		v, ok := f.validations[q.Get("val_id")]
		if !ok {
			v = Validation{Status: "INVALID_TRANSACTION"}
		}
		json.NewEncoder(w).Encode(v)
	default:
		http.NotFound(w, r)
	}
}

func TestSSLCommerzFlow(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	ws := wallet.NewStore(pool)
	patient := dbtest.CreateUser(t, pool, "Karim Uddin", "01811111111", user.RolePatient)
	pkgs, _ := ws.Packages(ctx)

	fake := &fakeSSLCommerz{sessions: map[string]url.Values{}, validations: map[string]Validation{}}
	api := httptest.NewServer(fake)
	defer api.Close()

	client := New(Config{StoreID: "teststore", StorePassword: "secret", Sandbox: true,
		PublicBaseURL: "https://api.example.com/", APIURL: api.URL})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cb := NewCallbacks(client, ws, "https://app.example.com", log)

	start := func() wallet.Payment {
		t.Helper()
		p, err := ws.StartPurchase(ctx, patient, pkgs[0].ID, client.Name())
		if err != nil {
			t.Fatal(err)
		}
		checkout, err := client.Checkout(ctx, p, wallet.Payer{Name: "Karim Uddin", Phone: "01811111111"})
		if err != nil || !strings.HasSuffix(checkout, p.ID) {
			t.Fatalf("checkout: %q, %v", checkout, err)
		}
		return p
	}
	post := func(h http.HandlerFunc, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}
	tokens := func() int {
		w, _ := ws.Wallet(ctx, patient)
		return w.TokensAvailable
	}
	status := func(id string) string {
		p, _ := ws.PaymentByID(ctx, id)
		return p.Status
	}

	// 1. The session is opened with our amount, ids and callback URLs.
	p1 := start()
	form := fake.sessions[p1.ID]
	if form.Get("total_amount") != "1000.00" || form.Get("currency") != "BDT" ||
		form.Get("ipn_url") != "https://api.example.com/api/v1/payments/sslcommerz/ipn" || form.Get("cus_phone") != "01811111111" {
		t.Fatalf("init form = %v", form)
	}

	// 2. A valid IPN credits tokens once, however many times it arrives.
	fake.validations["val-ok"] = Validation{Status: "VALID", TranID: p1.ID, Amount: "1000.00", Currency: "BDT", BankTranID: "BANK1", RiskLevel: "0"}
	for range 2 {
		if rec := post(cb.IPN, url.Values{"val_id": {"val-ok"}, "tran_id": {p1.ID}, "status": {"VALID"}}); rec.Code != http.StatusOK {
			t.Fatalf("ipn status %d", rec.Code)
		}
	}
	if tokens() != 10 || status(p1.ID) != "paid" {
		t.Fatalf("after IPN: tokens=%d status=%s", tokens(), status(p1.ID))
	}

	// 3. The success redirect re-checks (already validated) and sends the browser on.
	fake.validations["val-ok"] = Validation{Status: "VALIDATED", TranID: p1.ID, Amount: "1000.00", Currency: "BDT", RiskLevel: "0"}
	rec := post(cb.Success, url.Values{"val_id": {"val-ok"}, "tran_id": {p1.ID}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "https://app.example.com/payments/result?status=success&payment_id="+p1.ID {
		t.Fatalf("success redirect: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if tokens() != 10 {
		t.Fatalf("success redirect credited again: %d", tokens())
	}

	// 4. A forged form is ignored: the amount check uses SSLCommerz's answer, not the POST.
	p2 := start()
	fake.validations["val-cheap"] = Validation{Status: "VALID", TranID: p2.ID, Amount: "10.00", Currency: "BDT", RiskLevel: "0"}
	post(cb.IPN, url.Values{"val_id": {"val-cheap"}, "amount": {"1000.00"}})
	post(cb.IPN, url.Values{"val_id": {"made-up"}, "tran_id": {p2.ID}, "status": {"VALID"}})
	if tokens() != 10 || status(p2.ID) != "pending" {
		t.Fatalf("forged IPNs credited: tokens=%d status=%s", tokens(), status(p2.ID))
	}

	// 5. Risky transactions are held for review.
	fake.validations["val-risky"] = Validation{Status: "VALID", TranID: p2.ID, Amount: "1000.00", Currency: "BDT", RiskLevel: "1"}
	post(cb.IPN, url.Values{"val_id": {"val-risky"}})
	if status(p2.ID) != "pending" {
		t.Fatalf("risky payment status = %s", status(p2.ID))
	}

	// 6. A fail redirect marks the payment failed, but a later genuine IPN still credits it.
	rec = post(cb.Fail, url.Values{"tran_id": {p2.ID}})
	if status(p2.ID) != "failed" || !strings.Contains(rec.Header().Get("Location"), "status=failed") {
		t.Fatalf("fail redirect: status=%s location=%s", status(p2.ID), rec.Header().Get("Location"))
	}
	fake.validations["val-late"] = Validation{Status: "VALID", TranID: p2.ID, Amount: "1000.00", Currency: "BDT", RiskLevel: "0"}
	post(cb.IPN, url.Values{"val_id": {"val-late"}})
	if tokens() != 20 || status(p2.ID) != "paid" {
		t.Fatalf("late IPN: tokens=%d status=%s", tokens(), status(p2.ID))
	}

	// 7. If SSLCommerz refuses to open a session, checkout fails.
	fake.refuse = true
	p3, _ := ws.StartPurchase(ctx, patient, pkgs[0].ID, client.Name())
	if _, err := client.Checkout(ctx, p3, wallet.Payer{Name: "K", Phone: "01811111111"}); err == nil {
		t.Fatal("expected checkout error")
	}
}

func TestParseTaka(t *testing.T) {
	cases := map[string]int64{"1000": 100000, "1000.5": 100050, "1000.00": 100000, "0.01": 1}
	for in, want := range cases {
		if got, err := ParseTaka(in); err != nil || got != want {
			t.Errorf("ParseTaka(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "10.001", "-5", ".5"} {
		if _, err := ParseTaka(bad); err == nil {
			t.Errorf("ParseTaka(%q) should fail", bad)
		}
	}
	if FormatTaka(100050) != "1000.50" {
		t.Errorf("FormatTaka = %s", FormatTaka(100050))
	}
}
