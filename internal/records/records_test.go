package records_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"healthcare-platform/internal/appointment"
	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/family"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/platform/storage"
	"healthcare-platform/internal/records"
	"healthcare-platform/internal/user"
)

func wantKind(t *testing.T, err error, k apperr.Kind, what string) {
	t.Helper()
	if !apperr.Is(err, k) {
		t.Fatalf("%s: got error %v, want kind %d", what, err, k)
	}
}

var pdf = []byte("%PDF-1.4\nCBC report: Hb 13.2 g/dL\n%%EOF\n")

func TestRecordsAndSharing(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	files, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := records.NewStore(pool, files)
	ds, as, fs := doctor.NewStore(pool), appointment.NewStore(pool), family.NewStore(pool)

	doc := dbtest.CreateUser(t, pool, "Dr One", "01711111111", user.RoleDoctor)
	otherDoc := dbtest.CreateUser(t, pool, "Dr Two", "01722222222", user.RoleDoctor)
	pat := dbtest.CreateUser(t, pool, "Karim", "01811111111", user.RolePatient)
	stranger := dbtest.CreateUser(t, pool, "Stranger", "01822222222", user.RolePatient)

	// A doctor with a session next week, and the patient's booking in it.
	if _, err := ds.UpdateProfile(ctx, doc, doctor.ProfileInput{BMDCRegNo: "A-12345", Specialization: "Medicine", Degrees: "MBBS"}); err != nil {
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
	appt, err := as.Book(ctx, pat, sc.ID, day.Format(time.DateOnly))
	if err != nil {
		t.Fatal(err)
	}
	child, err := fs.Add(ctx, pat, family.Input{Name: "Rahim", Relation: "child"})
	if err != nil {
		t.Fatal(err)
	}

	// Upload: validation, then one for the patient and one for the child.
	_, err = s.Add(ctx, pat, records.Meta{Title: "X"}, "r.pdf", "application/pdf", bytes.NewReader(pdf))
	wantKind(t, err, apperr.KindValidation, "short title")
	_, err = s.Add(ctx, pat, records.Meta{Title: "CBC", FamilyMemberID: child.ID}, "r.pdf", "application/pdf", bytes.NewReader(nil))
	wantKind(t, err, apperr.KindValidation, "empty file")
	_, err = s.Add(ctx, stranger, records.Meta{Title: "CBC", FamilyMemberID: child.ID}, "r.pdf", "application/pdf", bytes.NewReader(pdf))
	wantKind(t, err, apperr.KindNotFound, "someone else's family member")

	mine, err := s.Add(ctx, pat, records.Meta{Title: "CBC  report", Kind: "lab_report", RecordDate: "2026-11-01"}, "cbc.pdf", "application/pdf", bytes.NewReader(pdf))
	if err != nil {
		t.Fatal(err)
	}
	if mine.Title != "CBC report" || mine.PersonName != "Karim" || mine.SizeBytes != int64(len(pdf)) {
		t.Fatalf("record = %+v", mine)
	}
	kids, err := s.Add(ctx, pat, records.Meta{Title: "Vaccination card", Kind: "vaccination", FamilyMemberID: child.ID}, "card.pdf", "application/pdf", bytes.NewReader(pdf))
	if err != nil || kids.PersonName != "Rahim" {
		t.Fatalf("child's record = %+v, %v", kids, err)
	}
	if rs, _ := s.List(ctx, pat, "self"); len(rs) != 1 || rs[0].ID != mine.ID {
		t.Fatalf("own records = %+v", rs)
	}
	if rs, _ := s.List(ctx, pat, child.ID); len(rs) != 1 || rs[0].ID != kids.ID {
		t.Fatalf("child's records = %+v", rs)
	}
	if rs, _ := s.List(ctx, stranger, ""); len(rs) != 0 {
		t.Fatalf("a stranger sees %d records", len(rs))
	}

	open := func(who, what string) error {
		t.Helper()
		_, rc, err := s.Open(ctx, who, mine.ID)
		if err == nil {
			b, _ := io.ReadAll(rc)
			rc.Close()
			if !bytes.Equal(b, pdf) {
				t.Fatalf("%s: file content differs", what)
			}
		}
		return err
	}
	if err := open(pat, "owner"); err != nil {
		t.Fatal(err)
	}
	wantKind(t, open(doc, "doctor before sharing"), apperr.KindNotFound, "doctor before sharing")

	// Share with the appointment: its doctor can read it, another doctor can't.
	_, err = s.ShareWithAppointment(ctx, stranger, mine.ID, appt.ID)
	wantKind(t, err, apperr.KindNotFound, "share someone else's record")
	r, err := s.ShareWithAppointment(ctx, pat, mine.ID, appt.ID)
	if err != nil || len(r.SharedWith) != 1 || r.SharedWith[0].DoctorName != "Dr One" {
		t.Fatalf("shared = %+v, %v", r, err)
	}
	if _, err := s.ShareWithAppointment(ctx, pat, mine.ID, appt.ID); err != nil {
		t.Fatalf("sharing twice: %v", err)
	}
	if err := open(doc, "doctor after sharing"); err != nil {
		t.Fatal(err)
	}
	wantKind(t, open(otherDoc, "other doctor"), apperr.KindNotFound, "other doctor")
	list, err := s.ForAppointment(ctx, doc, appt.ID)
	if err != nil || len(list) != 1 || list[0].SharedWith != nil {
		t.Fatalf("doctor's list = %+v, %v", list, err)
	}
	_, err = s.ForAppointment(ctx, otherDoc, appt.ID)
	wantKind(t, err, apperr.KindNotFound, "other doctor's list")
	if a, _ := as.ListForDoctor(ctx, doc, appt.Date); len(a) != 1 || a[0].SharedRecordCount != 1 {
		t.Fatalf("appointment record count = %+v", a)
	}

	// Cancelling the appointment ends the doctor's access; so does unsharing.
	if _, err := as.Cancel(ctx, pat, appt.ID); err != nil {
		t.Fatal(err)
	}
	wantKind(t, open(doc, "doctor after cancel"), apperr.KindNotFound, "doctor after cancel")
	_, err = s.ShareWithAppointment(ctx, pat, kids.ID, appt.ID)
	wantKind(t, err, apperr.KindConflict, "share with a cancelled appointment")
	r, err = s.Unshare(ctx, pat, mine.ID, appt.ID)
	if err != nil || len(r.SharedWith) != 0 {
		t.Fatalf("unshared = %+v, %v", r, err)
	}

	// Send a copy into an active consultation chat; it outlives the record.
	var consultationID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO consultations (patient_id, doctor_id, subject, description, fee_tokens, commission_bps, status)
		VALUES ($1, $2, 'Fever for days', 'High fever since Saturday with body ache.', 2, 5000, 'active') RETURNING id`,
		pat, doc).Scan(&consultationID); err != nil {
		t.Fatal(err)
	}
	_, err = s.SendToConsultation(ctx, stranger, mine.ID, consultationID)
	wantKind(t, err, apperr.KindNotFound, "send into someone else's consultation")
	if _, err := s.SendToConsultation(ctx, pat, mine.ID, consultationID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, pat, mine.ID); err != nil {
		t.Fatal(err)
	}
	wantKind(t, open(pat, "deleted record"), apperr.KindNotFound, "deleted record")
	var key string
	if err := pool.QueryRow(ctx, `SELECT storage_key FROM attachments WHERE consultation_id = $1`, consultationID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	rc, err := files.Open(ctx, key)
	if err != nil {
		t.Fatalf("chat copy gone after deleting the record: %v", err)
	}
	rc.Close()
	wantKind(t, s.Delete(ctx, pat, mine.ID), apperr.KindNotFound, "delete twice")
}
