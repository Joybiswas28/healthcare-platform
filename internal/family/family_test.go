package family_test

import (
	"context"
	"testing"
	"time"

	"healthcare-platform/internal/appointment"
	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/family"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/prescription"
	"healthcare-platform/internal/user"
)

func wantKind(t *testing.T, err error, k apperr.Kind, what string) {
	t.Helper()
	if !apperr.Is(err, k) {
		t.Fatalf("%s: got error %v, want kind %d", what, err, k)
	}
}

func TestMembers(t *testing.T) {
	pool := dbtest.New(t)
	s := family.NewStore(pool)
	ctx := context.Background()
	owner := dbtest.CreateUser(t, pool, "Karim", "01811111111", user.RolePatient)
	other := dbtest.CreateUser(t, pool, "Other", "01822222222", user.RolePatient)

	_, err := s.Add(ctx, owner, family.Input{Name: "R", Relation: "child"})
	wantKind(t, err, apperr.KindValidation, "short name")
	_, err = s.Add(ctx, owner, family.Input{Name: "Rahim", Relation: "cousin"})
	wantKind(t, err, apperr.KindValidation, "unknown relation")
	_, err = s.Add(ctx, owner, family.Input{Name: "Rahim", Relation: "child", BirthDate: "2999-01-01"})
	wantKind(t, err, apperr.KindValidation, "birth date in the future")

	m, err := s.Add(ctx, owner, family.Input{Name: "  Rahim   Uddin ", Relation: "child", Gender: "male", BirthDate: "2018-03-09"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "Rahim Uddin" || *m.BirthDate != "2018-03-09" || *m.Gender != "male" {
		t.Fatalf("member = %+v", m)
	}

	_, err = s.Update(ctx, other, m.ID, family.Input{Name: "Taken", Relation: "child"})
	wantKind(t, err, apperr.KindNotFound, "edit someone else's member")
	m, err = s.Update(ctx, owner, m.ID, family.Input{Name: "Rahim Uddin", Relation: "child"})
	if err != nil || m.Gender != nil || m.BirthDate != nil {
		t.Fatalf("cleared fields = %+v, %v", m, err)
	}

	if err := family.Check(ctx, pool, other, m.ID); !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("other account may use the member: %v", err)
	}
	wantKind(t, s.Remove(ctx, other, m.ID), apperr.KindNotFound, "remove someone else's member")
	if err := s.Remove(ctx, owner, m.ID); err != nil {
		t.Fatal(err)
	}
	if ms, _ := s.List(ctx, owner); len(ms) != 0 {
		t.Fatalf("removed member still listed: %+v", ms)
	}
	wantKind(t, family.Check(ctx, pool, owner, m.ID), apperr.KindNotFound, "book for a removed member")

	for i := range family.MaxMembers {
		if _, err := s.Add(ctx, owner, family.Input{Name: "Child " + string(rune('A'+i)), Relation: "child"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = s.Add(ctx, owner, family.Input{Name: "One too many", Relation: "other"})
	wantKind(t, err, apperr.KindConflict, "more than the limit")
}

// A parent books themself and their child into the same session; the doctor
// sees and prescribes for the child by name.
func TestBookingForAFamilyMember(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	ds, as := doctor.NewStore(pool), appointment.NewStore(pool)
	fs, rx := family.NewStore(pool), prescription.NewStore(pool)
	doc := dbtest.CreateUser(t, pool, "Dr One", "01711111111", user.RoleDoctor)
	parent := dbtest.CreateUser(t, pool, "Karim", "01811111111", user.RolePatient)
	stranger := dbtest.CreateUser(t, pool, "Stranger", "01822222222", user.RolePatient)

	if _, err := ds.UpdateProfile(ctx, doc, doctor.ProfileInput{BMDCRegNo: "A-12345", Specialization: "Paediatrics", Degrees: "MBBS"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Verify(ctx, doc, doctor.VerifyInput{Status: doctor.StatusApproved}); err != nil {
		t.Fatal(err)
	}
	ch, err := ds.CreateChamber(ctx, doc, doctor.ChamberInput{Name: "City Chamber", Address: "12 Station Road", District: "Dhaka", FeePaisa: 50000})
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().In(appointment.Dhaka).AddDate(0, 0, 7)
	sc, err := ds.AddSchedule(ctx, doctor.DoctorScope(doc), ch.ID, doctor.ScheduleInput{Weekday: int(day.Weekday()), StartTime: "17:00", EndTime: "21:00", MaxPatients: 10})
	if err != nil {
		t.Fatal(err)
	}
	date := day.Format(time.DateOnly)
	child, err := fs.Add(ctx, parent, family.Input{Name: "Rahim", Relation: "child", Gender: "male", BirthDate: "2019-05-01"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = as.BookFor(ctx, stranger, child.ID, sc.ID, date)
	wantKind(t, err, apperr.KindNotFound, "book for someone else's child")

	own, err := as.Book(ctx, parent, sc.ID, date)
	if err != nil {
		t.Fatal(err)
	}
	a, err := as.BookFor(ctx, parent, child.ID, sc.ID, date)
	if err != nil {
		t.Fatalf("book the child alongside the parent: %v", err)
	}
	if own.FamilyMemberID != nil || own.PatientName != "Karim" {
		t.Fatalf("own booking = %+v", own)
	}
	if a.PatientName != "Rahim" || a.AccountName != "Karim" || *a.Relation != "child" || *a.PatientBirthDate != "2019-05-01" {
		t.Fatalf("child's booking = %+v", a)
	}
	_, err = as.BookFor(ctx, parent, child.ID, sc.ID, date)
	wantKind(t, err, apperr.KindConflict, "book the child twice")

	// Removing the member later keeps the booking and its name.
	if err := fs.Remove(ctx, parent, child.ID); err != nil {
		t.Fatal(err)
	}
	day1, err := as.ListForDoctor(ctx, doc, date)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, x := range day1 {
		names[x.PatientName] = true
	}
	if !names["Rahim"] || !names["Karim"] {
		t.Fatalf("doctor's list = %v", names)
	}

	if _, err := pool.Exec(ctx, `UPDATE appointments SET appointment_date = now()::date, status = 'completed' WHERE id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	p, err := rx.Issue(ctx, doc, prescription.IssueInput{
		AppointmentID: a.ID,
		Diagnosis:     "Viral fever",
		Medicines:     []prescription.Medicine{{Name: "Napa", Dosage: "1+1+1", Duration: "3 days"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.PatientName != "Rahim" || p.PatientID != parent {
		t.Fatalf("prescription for %q (account %s)", p.PatientName, p.PatientID)
	}
}
