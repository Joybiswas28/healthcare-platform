package prescription

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/user"
)

// Tuesday 17 Nov 2026, 11:00 in Dhaka.
var fixedNow = time.Date(2026, 11, 17, 11, 0, 0, 0, Dhaka)

type fixture struct {
	pool                  *pgxpool.Pool
	store                 *Store
	doctorID, patientID   string
	consultationID        string
	chamberID, scheduleID string
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	pool := dbtest.New(t)
	f := fixture{pool: pool, store: NewStore(pool)}
	f.store.now = func() time.Time { return fixedNow }

	f.doctorID = dbtest.CreateUser(t, pool, "Dr Ayesha Siddiqua", "01711111111", user.RoleDoctor)
	f.patientID = dbtest.CreateUser(t, pool, "Karim Uddin", "01811111111", user.RolePatient)
	ds := doctor.NewStore(pool)
	if _, err := ds.UpdateProfile(ctx, f.doctorID, doctor.ProfileInput{
		BMDCRegNo: "A-54321", Specialization: "Medicine", Degrees: "MBBS, FCPS (Medicine)", ExperienceYears: 7,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Verify(ctx, f.doctorID, doctor.VerifyInput{Status: doctor.StatusApproved}); err != nil {
		t.Fatal(err)
	}

	f.consultationID = f.consultation(t, f.doctorID, "active")
	f.chamberID = exec(t, pool, `INSERT INTO chambers (doctor_id, name, address, district, fee_paisa)
		VALUES ($1, 'Chamber', 'Road 2, Dhanmondi', 'Dhaka', 50000) RETURNING id`, f.doctorID)
	f.scheduleID = exec(t, pool, `INSERT INTO chamber_schedules (chamber_id, weekday, start_time, end_time, max_patients)
		VALUES ($1, 2, '17:00', '21:00', 20) RETURNING id`, f.chamberID)
	return f
}

func (f fixture) consultation(t *testing.T, doctorID, status string) string {
	t.Helper()
	return exec(t, f.pool, `INSERT INTO consultations (patient_id, doctor_id, subject, description, fee_tokens, commission_bps, status)
		VALUES ($1, $2, 'Fever', 'Fever for three days', 1, 5000, $3) RETURNING id`, f.patientID, doctorID, status)
}

func (f fixture) appointment(t *testing.T, date, status string, serial int) string {
	t.Helper()
	return exec(t, f.pool, `INSERT INTO appointments
		(ticket_code, schedule_id, chamber_id, doctor_id, patient_id, appointment_date, serial_no, fee_paisa, status)
		VALUES (md5(random()::text), $1, $2, $3, $4, $5::date, $6, 50000, $7) RETURNING id`,
		f.scheduleID, f.chamberID, f.doctorID, f.patientID, date, serial, status)
}

func validInput(consultationID string) IssueInput {
	return IssueInput{
		ConsultationID:  consultationID,
		PatientAge:      "34 years",
		PatientGender:   "Male",
		ChiefComplaints: "Fever 3 days, body ache",
		Diagnosis:       "Viral fever",
		Medicines: []Medicine{
			{Name: "Napa", Strength: "500 mg", Form: "Tab.", Dosage: "1+1+1", Duration: "5 days", Instructions: "after meal"},
			{Name: "Fexo", Strength: "120 mg", Form: "Tab.", Dosage: "0+0+1", Duration: "7 days"},
		},
		Investigations: []string{"CBC", "Dengue NS1"},
		Advice:         "প্রচুর পানি পান করুন এবং বিশ্রাম নিন।",
		FollowUpDate:   "2026-11-24",
	}
}

func wantKind(t *testing.T, err error, k apperr.Kind, what string) {
	t.Helper()
	if !apperr.Is(err, k) {
		t.Fatalf("%s: got error %v, want kind %d", what, err, k)
	}
}

var codePattern = regexp.MustCompile(`^[` + codeAlphabet + `]{10}$`)

func TestIssueAndRead(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	rx, err := f.store.Issue(ctx, f.doctorID, validInput(f.consultationID))
	if err != nil {
		t.Fatal(err)
	}
	if !codePattern.MatchString(rx.Code) || rx.Status != StatusIssued {
		t.Fatalf("code/status = %q/%s", rx.Code, rx.Status)
	}
	if rx.DoctorBMDC != "A-54321" || rx.PatientName != "Karim Uddin" || *rx.PatientGender != "male" {
		t.Fatalf("snapshot = %+v", rx)
	}
	if len(rx.Medicines) != 2 || rx.Medicines[0].Dosage != "1+1+1" || len(rx.Investigations) != 2 || *rx.FollowUpDate != "2026-11-24" {
		t.Fatalf("content = %+v", rx)
	}

	// The patient is notified (and texted).
	var sms string
	if err := f.pool.QueryRow(ctx, `SELECT sms_status FROM notifications WHERE user_id = $1 AND kind = 'prescription_issued'`,
		f.patientID).Scan(&sms); err != nil || sms != "pending" {
		t.Fatalf("notification: %q, %v", sms, err)
	}

	// Later profile changes don't alter an issued prescription.
	if _, err := f.pool.Exec(ctx, `UPDATE users SET name = 'Someone Else' WHERE id = $1`, f.patientID); err != nil {
		t.Fatal(err)
	}
	got, err := f.store.Get(ctx, f.patientID, rx.ID)
	if err != nil || got.PatientName != "Karim Uddin" {
		t.Fatalf("patient reads: %+v, %v", got, err)
	}

	outsider := dbtest.CreateUser(t, f.pool, "Outsider", "01922222222", user.RolePatient)
	_, err = f.store.Get(ctx, outsider, rx.ID)
	wantKind(t, err, apperr.KindNotFound, "outsider reads prescription")

	mine, _ := f.store.List(ctx, f.patientID, false, "", "", 20, 0)
	issued, _ := f.store.List(ctx, f.doctorID, true, f.consultationID, "", 20, 0)
	none, _ := f.store.List(ctx, outsider, false, "", "", 20, 0)
	if len(mine) != 1 || len(issued) != 1 || len(none) != 0 {
		t.Fatalf("lists: patient=%d doctor=%d outsider=%d", len(mine), len(issued), len(none))
	}

	v, err := f.store.Verify(ctx, strings.ToLower(rx.Code))
	if err != nil || v.PatientName != "K**** U****" || v.Status != StatusIssued || len(v.Medicines) != 2 {
		t.Fatalf("verify: %+v, %v", v, err)
	}

	_, err = f.store.Revoke(ctx, f.doctorID, rx.ID, "no")
	wantKind(t, err, apperr.KindValidation, "revoke reason too short")
	_, err = f.store.Revoke(ctx, outsider, rx.ID, "wrong patient selected")
	wantKind(t, err, apperr.KindNotFound, "outsider revokes")
	if rx, err = f.store.Revoke(ctx, f.doctorID, rx.ID, "wrong dose, reissued"); err != nil || rx.Status != StatusRevoked {
		t.Fatalf("revoke: %+v, %v", rx, err)
	}
	_, err = f.store.Revoke(ctx, f.doctorID, rx.ID, "again and again")
	wantKind(t, err, apperr.KindConflict, "revoke twice")
	if v, _ = f.store.Verify(ctx, rx.Code); v.Status != StatusRevoked {
		t.Fatalf("verify after revoke: %+v", v)
	}
	_, err = f.store.Verify(ctx, "AAAAAAAAAA")
	wantKind(t, err, apperr.KindNotFound, "unknown code")
}

func TestIssueRules(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	bad := func(mutate func(*IssueInput)) error {
		in := validInput(f.consultationID)
		mutate(&in)
		_, err := f.store.Issue(ctx, f.doctorID, in)
		return err
	}
	wantKind(t, bad(func(in *IssueInput) { in.AppointmentID = f.consultationID }), apperr.KindValidation, "both ids")
	wantKind(t, bad(func(in *IssueInput) { in.ConsultationID = "" }), apperr.KindValidation, "no id")
	wantKind(t, bad(func(in *IssueInput) { in.ChiefComplaints, in.Diagnosis = "", "" }), apperr.KindValidation, "no complaints or diagnosis")
	wantKind(t, bad(func(in *IssueInput) { in.Medicines[1].Dosage = "" }), apperr.KindValidation, "medicine without dosage")
	wantKind(t, bad(func(in *IssueInput) { in.FollowUpDate = "2026-11-10" }), apperr.KindValidation, "follow-up in the past")
	wantKind(t, bad(func(in *IssueInput) { in.PatientGender = "x" }), apperr.KindValidation, "bad gender")

	other := dbtest.CreateUser(t, f.pool, "Dr Other", "01733333333", user.RoleDoctor)
	_, err := f.store.Issue(ctx, other, validInput(f.consultationID))
	wantKind(t, err, apperr.KindNotFound, "another doctor's consultation")

	// An unverified doctor can't issue, even for their own consultation.
	_, err = f.store.Issue(ctx, other, validInput(f.consultation(t, other, "active")))
	wantKind(t, err, apperr.KindForbidden, "unverified doctor")

	// A second patient, since one patient may hold only one open consultation per doctor.
	p2 := dbtest.CreateUser(t, f.pool, "Second Patient", "01844444444", user.RolePatient)
	requested := exec(t, f.pool, `INSERT INTO consultations (patient_id, doctor_id, subject, description, fee_tokens, commission_bps, status)
		VALUES ($1, $2, 'Cough', 'Dry cough for a week', 1, 5000, 'requested') RETURNING id`, p2, f.doctorID)
	_, err = f.store.Issue(ctx, f.doctorID, validInput(requested))
	wantKind(t, err, apperr.KindConflict, "consultation not started")

	appt := func(date, status string, serial int) IssueInput {
		in := validInput("")
		in.AppointmentID = f.appointment(t, date, status, serial)
		return in
	}
	if _, err := f.store.Issue(ctx, f.doctorID, appt("2026-11-17", "confirmed", 1)); err != nil {
		t.Fatalf("today's appointment: %v", err)
	}
	_, err = f.store.Issue(ctx, f.doctorID, appt("2026-11-24", "confirmed", 1))
	wantKind(t, err, apperr.KindConflict, "future appointment")
	_, err = f.store.Issue(ctx, f.doctorID, appt("2026-11-10", "cancelled", 1))
	wantKind(t, err, apperr.KindConflict, "cancelled appointment")
}

func TestRender(t *testing.T) {
	f := setup(t)
	in := validInput(f.consultationID)
	in.Advice = `Drink water <script>alert("x")</script> প্রচুর পানি পান করুন`
	rx, err := f.store.Issue(context.Background(), f.doctorID, in)
	if err != nil {
		t.Fatal(err)
	}
	html, err := RenderString(rx, "http://localhost:8080/api/v1/prescriptions/verify/"+rx.Code)
	if err != nil {
		t.Fatal(err)
	}
	// html/template writes "+" as "&#43;", which browsers display as "+".
	for _, want := range []string{"Dr Ayesha Siddiqua", "A-54321", rx.Code, "Napa", "1&#43;1&#43;1", "Dengue NS1",
		"প্রচুর পানি পান করুন", "&lt;script&gt;", "data:image/png;base64,"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}
	if strings.Contains(html, "<script>alert") {
		t.Error("advice was not escaped")
	}
	if strings.Contains(html, ">REVOKED<") {
		t.Error("issued prescription shows REVOKED")
	}
	rx.Status = StatusRevoked
	if html, _ = RenderString(rx, "x"); !strings.Contains(html, ">REVOKED<") {
		t.Error("revoked prescription has no REVOKED mark")
	}
}
