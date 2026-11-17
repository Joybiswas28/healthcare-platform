package consultation

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/platform/storage"
	"healthcare-platform/internal/user"
	"healthcare-platform/internal/wallet"
)

type fixture struct {
	pool    *pgxpool.Pool
	store   *Store
	wallet  *wallet.Store
	doctors *doctor.Store
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := dbtest.New(t)
	files, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return fixture{pool: pool, store: NewStore(pool, files), wallet: wallet.NewStore(pool), doctors: doctor.NewStore(pool)}
}

var phoneSeq = 0

func nextPhone() string {
	phoneSeq++
	return fmt.Sprintf("0171%07d", phoneSeq)
}

// onlineDoctor creates an approved doctor who takes online consultations.
func (f fixture) onlineDoctor(t *testing.T, feeTokens int) string {
	t.Helper()
	ctx := context.Background()
	id := dbtest.CreateUser(t, f.pool, "Dr "+nextPhone(), nextPhone(), user.RoleDoctor)
	if _, err := f.doctors.UpdateProfile(ctx, id, doctor.ProfileInput{
		BMDCRegNo: fmt.Sprintf("A-%d", 10000+phoneSeq), Specialization: "Medicine", Degrees: "MBBS",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.doctors.Verify(ctx, id, doctor.VerifyInput{Status: doctor.StatusApproved}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.doctors.SetConsultationSettings(ctx, id, doctor.ConsultationSettingsInput{Enabled: true, FeeTokens: feeTokens}); err != nil {
		t.Fatal(err)
	}
	return id
}

// patientWithTokens creates a patient holding 10 tokens per package bought.
func (f fixture) patientWithTokens(t *testing.T, packages int) string {
	t.Helper()
	ctx := context.Background()
	id := dbtest.CreateUser(t, f.pool, "Patient "+nextPhone(), "018"+nextPhone()[3:], user.RolePatient)
	ps, _ := f.wallet.Packages(ctx)
	for range packages {
		p, err := f.wallet.StartPurchase(ctx, id, ps[0].ID, "test")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.wallet.CompletePayment(ctx, p.ID, "ref"); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func (f fixture) tokens(t *testing.T, patientID string) int {
	t.Helper()
	w, err := f.wallet.Wallet(context.Background(), patientID)
	if err != nil {
		t.Fatal(err)
	}
	return w.TokensAvailable
}

func request(subject string, doctorID string) RequestInput {
	return RequestInput{DoctorID: doctorID, Subject: subject, Description: "Fever and headache for three days, no other symptoms."}
}

func wantKind(t *testing.T, err error, k apperr.Kind, what string) {
	t.Helper()
	if !apperr.Is(err, k) {
		t.Fatalf("%s: got error %v, want kind %d", what, err, k)
	}
}

func TestFullConsultationFlow(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	doc := f.onlineDoctor(t, 3)
	patient := f.patientWithTokens(t, 1)
	outsider := f.patientWithTokens(t, 0)

	c, err := f.store.Request(ctx, patient, request("Persistent fever", doc))
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusRequested || c.FeeTokens != 3 {
		t.Fatalf("requested = %+v", c)
	}

	// Price changes after the request do not affect it.
	if _, err := f.doctors.SetConsultationSettings(ctx, doc, doctor.ConsultationSettingsInput{Enabled: true, FeeTokens: 5}); err != nil {
		t.Fatal(err)
	}

	_, err = f.store.Request(ctx, patient, request("Another question", doc))
	wantKind(t, err, apperr.KindConflict, "second open request to same doctor")

	_, err = f.store.Get(ctx, outsider, c.ID)
	wantKind(t, err, apperr.KindNotFound, "outsider reads consultation")

	_, err = f.store.Accept(ctx, patient, c.ID)
	wantKind(t, err, apperr.KindForbidden, "patient accepts")

	_, err = f.store.PostMessage(ctx, patient, c.ID, "hello?")
	wantKind(t, err, apperr.KindConflict, "message before active")

	if c, err = f.store.Accept(ctx, doc, c.ID); err != nil || c.Status != StatusAccepted {
		t.Fatalf("accept: %+v, %v", c, err)
	}
	if c, err = f.store.Confirm(ctx, patient, c.ID); err != nil || c.Status != StatusActive {
		t.Fatalf("confirm: %+v, %v", c, err)
	}
	if got := f.tokens(t, patient); got != 7 {
		t.Fatalf("tokens after confirm = %d, want 7", got)
	}

	if _, err := f.store.PostMessage(ctx, patient, c.ID, "My temperature is 102F."); err != nil {
		t.Fatal(err)
	}
	pdf := "%PDF-1.4\n% fake lab report\n"
	up, err := f.store.Upload(ctx, patient, c.ID, "cbc-report.pdf", "application/pdf", strings.NewReader(pdf))
	if err != nil || up.Attachment == nil || up.Attachment.SizeBytes != int64(len(pdf)) {
		t.Fatalf("upload: %+v, %v", up, err)
	}
	if _, err := f.store.PostMessage(ctx, doc, c.ID, "Please take paracetamol and rest."); err != nil {
		t.Fatal(err)
	}

	msgs, err := f.store.Messages(ctx, doc, c.ID, 0, 50)
	if err != nil || len(msgs) != 3 || msgs[1].Attachment == nil || msgs[2].SenderID != doc {
		t.Fatalf("messages: %+v, %v", msgs, err)
	}
	if later, _ := f.store.Messages(ctx, patient, c.ID, msgs[1].ID, 50); len(later) != 1 {
		t.Fatalf("messages after id: %d", len(later))
	}

	_, rc, err := f.store.OpenAttachment(ctx, doc, up.Attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	if string(body) != pdf {
		t.Fatalf("downloaded %q", body)
	}
	_, _, err = f.store.OpenAttachment(ctx, outsider, up.Attachment.ID)
	wantKind(t, err, apperr.KindNotFound, "outsider downloads report")

	_, err = f.store.Cancel(ctx, patient, c.ID)
	wantKind(t, err, apperr.KindConflict, "patient cancels active consultation")

	if c, err = f.store.Complete(ctx, doc, c.ID); err != nil || c.Status != StatusCompleted {
		t.Fatalf("complete: %+v, %v", c, err)
	}
	e, err := f.wallet.Earnings(ctx, doc, 20, 0)
	if err != nil || e.BalancePaisa != 15000 {
		t.Fatalf("doctor earnings = %+v, %v (want 15000: half of 3 tokens)", e, err)
	}

	_, err = f.store.PostMessage(ctx, patient, c.ID, "thanks")
	wantKind(t, err, apperr.KindConflict, "message after completion")

	// Closed consultations no longer block a new request.
	if _, err := f.store.Request(ctx, patient, request("Follow-up question", doc)); err != nil {
		t.Fatalf("new request after completion: %v", err)
	}
}

func TestRejectCancelAndRefund(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	doc := f.onlineDoctor(t, 2)
	patient := f.patientWithTokens(t, 1)

	c, _ := f.store.Request(ctx, patient, request("Skin rash", doc))
	_, err := f.store.Reject(ctx, doc, c.ID, "no")
	wantKind(t, err, apperr.KindValidation, "reject reason too short")
	c, err = f.store.Reject(ctx, doc, c.ID, "Please see a dermatologist in person.")
	if err != nil || c.Status != StatusRejected || c.RejectReason == nil {
		t.Fatalf("reject: %+v, %v", c, err)
	}
	_, err = f.store.Confirm(ctx, patient, c.ID)
	wantKind(t, err, apperr.KindConflict, "confirm rejected consultation")

	c, _ = f.store.Request(ctx, patient, request("Skin rash again", doc))
	f.store.Accept(ctx, doc, c.ID)
	if c, err = f.store.Cancel(ctx, patient, c.ID); err != nil || c.Status != StatusCancelled {
		t.Fatalf("patient cancels accepted: %+v, %v", c, err)
	}
	if got := f.tokens(t, patient); got != 10 {
		t.Fatalf("tokens = %d, nothing should have been charged", got)
	}

	c, _ = f.store.Request(ctx, patient, request("Third request", doc))
	f.store.Accept(ctx, doc, c.ID)
	f.store.Confirm(ctx, patient, c.ID)
	if got := f.tokens(t, patient); got != 8 {
		t.Fatalf("tokens after confirm = %d", got)
	}
	if c, err = f.store.Cancel(ctx, doc, c.ID); err != nil || c.Status != StatusCancelled {
		t.Fatalf("doctor cancels active: %+v, %v", c, err)
	}
	if got := f.tokens(t, patient); got != 10 {
		t.Fatalf("tokens after refund = %d, want 10", got)
	}
}

func TestConfirmNeedsEnoughTokens(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	doc := f.onlineDoctor(t, 3)
	broke := f.patientWithTokens(t, 0)

	c, _ := f.store.Request(ctx, broke, request("Chest pain question", doc))
	f.store.Accept(ctx, doc, c.ID)
	_, err := f.store.Confirm(ctx, broke, c.ID)
	wantKind(t, err, apperr.KindConflict, "confirm without tokens")
	if c, _ = f.store.Get(ctx, broke, c.ID); c.Status != StatusAccepted {
		t.Fatalf("status after failed confirm = %s, want accepted", c.Status)
	}

	off := f.onlineDoctor(t, 1)
	if _, err := f.doctors.SetConsultationSettings(ctx, off, doctor.ConsultationSettingsInput{Enabled: false, FeeTokens: 1}); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.Request(ctx, broke, request("Any question", off))
	wantKind(t, err, apperr.KindConflict, "doctor not taking consultations")
}

func TestConcurrentConfirmsNeverOverspend(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	patient := f.patientWithTokens(t, 1) // 10 tokens

	// Five doctors at 3 tokens each: only three consultations can be paid for.
	var ids []string
	for range 5 {
		doc := f.onlineDoctor(t, 3)
		c, err := f.store.Request(ctx, patient, request("Same question", doc))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.Accept(ctx, doc, c.ID); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, conflicts := 0, 0
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.store.Confirm(ctx, patient, id)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case apperr.Is(err, apperr.KindConflict):
				conflicts++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if ok != 3 || conflicts != 2 || f.tokens(t, patient) != 1 {
		t.Fatalf("ok=%d conflicts=%d tokens left=%d; want 3, 2, 1", ok, conflicts, f.tokens(t, patient))
	}
}
