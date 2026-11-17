package consultation

import (
	"context"
	"sync"
	"testing"

	"healthcare-platform/internal/platform/apperr"
)

func concernInput(title string) ConcernInput {
	return ConcernInput{
		Title:          title,
		Description:    "Recurring headaches every morning for two weeks, worse after screen time.",
		Specialization: "Neurology",
	}
}

func TestConcernOffersFlow(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	patient := f.patientWithTokens(t, 1)
	other := f.patientWithTokens(t, 0)
	docA := f.onlineDoctor(t, 2)
	docB := f.onlineDoctor(t, 4)

	c, err := f.store.PostConcern(ctx, patient, concernInput("Morning headaches"))
	if err != nil {
		t.Fatal(err)
	}

	// The doctor feed must not reveal who posted the concern.
	feed, err := f.store.OpenConcerns(ctx, "neuro", 20, 0)
	if err != nil || len(feed) != 1 || feed[0].PatientID != "" {
		t.Fatalf("feed = %+v, %v", feed, err)
	}
	if none, _ := f.store.OpenConcerns(ctx, "cardio", 20, 0); len(none) != 0 {
		t.Fatalf("specialization filter returned %d concerns", len(none))
	}

	offerA, err := f.store.MakeOffer(ctx, docA, c.ID, "I can help with this, please share any scans.")
	if err != nil || offerA.FeeTokens != 2 {
		t.Fatalf("offer A: %+v, %v", offerA, err)
	}
	_, err = f.store.MakeOffer(ctx, docA, c.ID, "Second offer from the same doctor.")
	wantKind(t, err, apperr.KindConflict, "duplicate offer")

	offerB, err := f.store.MakeOffer(ctx, docB, c.ID, "Neurologist here, happy to review.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.WithdrawOffer(ctx, docB, offerB.ID); err != nil {
		t.Fatal(err)
	}
	if offerB, err = f.store.MakeOffer(ctx, docB, c.ID, "Renewed offer after withdrawing."); err != nil || offerB.Status != OfferPending {
		t.Fatalf("renew withdrawn offer: %+v, %v", offerB, err)
	}

	mine, err := f.store.PatientConcern(ctx, patient, c.ID)
	if err != nil || len(mine.Offers) != 2 || mine.OfferCount != 2 {
		t.Fatalf("patient view: %+v, %v", mine, err)
	}
	_, err = f.store.PatientConcern(ctx, other, c.ID)
	wantKind(t, err, apperr.KindNotFound, "another patient reads the concern")
	_, err = f.store.AcceptOffer(ctx, other, offerA.ID)
	wantKind(t, err, apperr.KindNotFound, "another patient accepts an offer")

	cons, err := f.store.AcceptOffer(ctx, patient, offerA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cons.Status != StatusActive || cons.DoctorID != docA || cons.FeeTokens != 2 || cons.Subject != "Morning headaches" {
		t.Fatalf("consultation = %+v", cons)
	}
	if got := f.tokens(t, patient); got != 8 {
		t.Fatalf("tokens = %d, want 8", got)
	}

	mine, _ = f.store.PatientConcern(ctx, patient, c.ID)
	if mine.Status != ConcernConverted || mine.ConsultationID == nil || *mine.ConsultationID != cons.ID {
		t.Fatalf("concern after accept = %+v", mine)
	}
	for _, o := range mine.Offers {
		want := OfferDeclined
		if o.ID == offerA.ID {
			want = OfferAccepted
		}
		if o.Status != want {
			t.Errorf("offer %s status = %s, want %s", o.DoctorID, o.Status, want)
		}
	}

	_, err = f.store.AcceptOffer(ctx, patient, offerB.ID)
	wantKind(t, err, apperr.KindConflict, "accept a second offer")
	_, err = f.store.MakeOffer(ctx, docB, c.ID, "Late offer on a converted concern.")
	wantKind(t, err, apperr.KindConflict, "offer on converted concern")

	// Only the accepted doctor learns which consultation the concern became.
	viewA, _ := f.store.DoctorConcern(ctx, docA, c.ID)
	viewB, _ := f.store.DoctorConcern(ctx, docB, c.ID)
	if viewA.ConsultationID == nil || viewB.ConsultationID != nil || viewB.MyOffer.Status != OfferDeclined {
		t.Fatalf("doctor views: A=%+v B=%+v", viewA, viewB)
	}
}

func TestConcernLimitsAndClose(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	patient := f.patientWithTokens(t, 0)
	doc := f.onlineDoctor(t, 1)

	var ids []string
	for i := range MaxOpenConcerns {
		c, err := f.store.PostConcern(ctx, patient, concernInput("Concern number "+string(rune('A'+i))))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	_, err := f.store.PostConcern(ctx, patient, concernInput("One too many"))
	wantKind(t, err, apperr.KindConflict, "open concern limit")

	offer, err := f.store.MakeOffer(ctx, doc, ids[0], "I can take a look at this.")
	if err != nil {
		t.Fatal(err)
	}

	// Accepting without enough tokens fails and leaves everything as it was.
	_, err = f.store.AcceptOffer(ctx, patient, offer.ID)
	wantKind(t, err, apperr.KindConflict, "accept without tokens")
	c, _ := f.store.PatientConcern(ctx, patient, ids[0])
	if c.Status != ConcernOpen || c.Offers[0].Status != OfferPending {
		t.Fatalf("after failed accept: %+v", c)
	}

	c, err = f.store.CloseConcern(ctx, patient, ids[0])
	if err != nil || c.Status != ConcernClosed || c.Offers[0].Status != OfferDeclined {
		t.Fatalf("close: %+v, %v", c, err)
	}
	if _, err := f.store.PostConcern(ctx, patient, concernInput("Room for another")); err != nil {
		t.Fatalf("post after closing one: %v", err)
	}
	if feed, _ := f.store.OpenConcerns(ctx, "", 20, 0); len(feed) != MaxOpenConcerns {
		t.Fatalf("feed size = %d", len(feed))
	}

	notOnline := f.onlineDoctor(t, 1)
	f.doctors.SetConsultationSettings(ctx, notOnline, consultationOff())
	_, err = f.store.MakeOffer(ctx, notOnline, ids[1], "I would like to help.")
	wantKind(t, err, apperr.KindForbidden, "offer from doctor without online consultations")
}

func TestConcurrentOfferAcceptsChargeOnce(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	patient := f.patientWithTokens(t, 1)

	c, err := f.store.PostConcern(ctx, patient, concernInput("Need advice quickly"))
	if err != nil {
		t.Fatal(err)
	}
	var offers []string
	for range 4 {
		doc := f.onlineDoctor(t, 2)
		o, err := f.store.MakeOffer(ctx, doc, c.ID, "I am available right now.")
		if err != nil {
			t.Fatal(err)
		}
		offers = append(offers, o.ID)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for _, id := range offers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.store.AcceptOffer(ctx, patient, id)
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

	if ok != 1 || f.tokens(t, patient) != 8 {
		t.Fatalf("accepted=%d tokens=%d; want 1 accepted and 8 tokens left", ok, f.tokens(t, patient))
	}
}
