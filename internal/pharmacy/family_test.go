package pharmacy

import (
	"context"
	"testing"

	"healthcare-platform/internal/family"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/user"
)

// Medicines for a child: the pharmacy sees the child's age and gender but no
// name until the order; the rider never sees who it is for.
func TestRequestForAFamilyMember(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	near := f.pharmacy(t, "Lazz Pharma", 23.7465, 90.3750, 5, true)
	parent := dbtest.CreateUser(t, f.pool, "Karim Uddin", "01811111111", user.RolePatient)
	stranger := dbtest.CreateUser(t, f.pool, "Someone", "01822222222", user.RolePatient)
	child, err := family.NewStore(f.pool).Add(ctx, parent, family.Input{Name: "Rahim", Relation: "child", Gender: "male", BirthDate: "2019-05-01"})
	if err != nil {
		t.Fatal(err)
	}

	in := deliveryRequest(f.prescriptionFor(t, parent))
	in.FamilyMemberID = child.ID
	_, err = f.store.CreateRequest(ctx, stranger, RequestInput{
		PrescriptionID: f.prescriptionFor(t, stranger), Fulfilment: FulfilDelivery, Address: "Road 1",
		Latitude: ptr(homeLat), Longitude: ptr(homeLng), FamilyMemberID: child.ID,
	}, nil)
	wantKind(t, err, apperr.KindNotFound, "request for someone else's child")

	rq, err := f.store.CreateRequest(ctx, parent, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rq.ForName == nil || *rq.ForName != "Rahim" || rq.PatientAge == nil || *rq.PatientAge < 6 || *rq.PatientGender != "male" {
		t.Fatalf("patient's view = %+v", rq)
	}

	feed, err := f.store.Feed(ctx, near.id, 20, 0)
	if err != nil || len(feed) != 1 {
		t.Fatalf("feed = %+v, %v", feed, err)
	}
	if feed[0].ForName != nil || feed[0].FamilyMemberID != nil || feed[0].PatientAge == nil || *feed[0].PatientGender != "male" {
		t.Fatalf("pharmacy's view of the request = %+v", feed[0])
	}

	q, err := f.store.SubmitQuote(ctx, near.id, rq.ID, quote(15000, false, 3000))
	if err != nil {
		t.Fatal(err)
	}
	order, err := f.store.AcceptQuote(ctx, parent, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if order.PatientName != "Karim Uddin" || order.ForName == nil || *order.ForName != "Rahim" {
		t.Fatalf("patient's order = %+v", order)
	}
	po, err := f.store.OrderForPharmacy(ctx, near.id, order.ID)
	if err != nil || po.ForName == nil || *po.ForName != "Rahim" || po.PatientAge == nil {
		t.Fatalf("pharmacy's order = %+v, %v", po, err)
	}
	if v := f.store.shape(po, asRider); v.ForName != nil || v.PatientAge != nil || v.PatientGender != nil {
		t.Fatalf("rider sees who it is for: %+v", v)
	}
}
