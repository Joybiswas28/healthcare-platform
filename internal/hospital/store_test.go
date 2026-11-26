package hospital

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/appointment"
	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/platform/storage"
	"healthcare-platform/internal/user"
)

type fixture struct {
	pool         *pgxpool.Pool
	store        *Store
	doctors      *doctor.Store
	appointments *appointment.Store
	owner        string
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
	ds := doctor.NewStore(pool)
	return fixture{
		pool: pool, store: NewStore(pool, ds, files), doctors: ds, appointments: appointment.NewStore(pool),
		owner: dbtest.CreateUser(t, pool, "Hospital Owner", phone(), user.RoleHospitalAdmin),
	}
}

func input(name, regNo string) HospitalInput {
	lat, lng := 23.7461, 90.3742
	return HospitalInput{
		Name: name, RegistrationNo: regNo, Address: "House 1, Road 4, Dhanmondi", District: "Dhaka",
		Area: "Dhanmondi", Phone: "09606213233", Departments: []string{"Cardiology", "Medicine", " cardiology "},
		Facilities: []string{"ICU", "Pharmacy", "Ambulance"}, Latitude: &lat, Longitude: &lng,
	}
}

func (f fixture) approvedHospital(t *testing.T, name, regNo string) string {
	t.Helper()
	ctx := context.Background()
	h, err := f.store.Create(ctx, f.owner, input(name, regNo))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Verify(ctx, h.ID, VerifyInput{Status: StatusApproved}); err != nil {
		t.Fatal(err)
	}
	return h.ID
}

func (f fixture) verifiedDoctor(t *testing.T, name, bmdc string) string {
	t.Helper()
	ctx := context.Background()
	id := dbtest.CreateUser(t, f.pool, name, phone(), user.RoleDoctor)
	if _, err := f.doctors.UpdateProfile(ctx, id, doctor.ProfileInput{BMDCRegNo: bmdc, Specialization: "Cardiology", Degrees: "MBBS, MD"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.doctors.Verify(ctx, id, doctor.VerifyInput{Status: doctor.StatusApproved}); err != nil {
		t.Fatal(err)
	}
	return id
}

func wantKind(t *testing.T, err error, k apperr.Kind, what string) {
	t.Helper()
	if !apperr.Is(err, k) {
		t.Fatalf("%s: got error %v, want kind %d", what, err, k)
	}
}

func TestHospitalLifecycle(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	h, err := f.store.Create(ctx, f.owner, input("Green Life Hospital", "dghs-1234"))
	if err != nil {
		t.Fatal(err)
	}
	if h.VerificationStatus != StatusPending || h.MyRole != RoleOwner || h.RegistrationNo != "DGHS-1234" ||
		len(h.Departments) != 2 || h.MapURL == nil {
		t.Fatalf("created = %+v", h)
	}
	_, err = f.store.Create(ctx, f.owner, input("Copy Hospital", "DGHS-1234"))
	wantKind(t, err, apperr.KindConflict, "duplicate registration number")
	bad := input("Far Away", "X-1")
	lat := 51.5
	bad.Latitude = &lat
	_, err = f.store.Create(ctx, f.owner, bad)
	wantKind(t, err, apperr.KindValidation, "coordinates outside Bangladesh")

	// Hidden until verified.
	if list, _ := f.store.Search(ctx, SearchParams{Limit: 20}); len(list) != 0 {
		t.Fatalf("pending hospital listed: %d", len(list))
	}
	_, err = f.store.Public(ctx, h.ID)
	wantKind(t, err, apperr.KindNotFound, "pending hospital page")
	_, err = f.store.Invite(ctx, h.ID, f.owner, InviteInput{BMDCRegNo: "A-1"})
	wantKind(t, err, apperr.KindConflict, "invite before verification")

	outsider := dbtest.CreateUser(t, f.pool, "Other Admin", phone(), user.RoleHospitalAdmin)
	_, err = f.store.Role(ctx, outsider, h.ID)
	wantKind(t, err, apperr.KindNotFound, "non-member role")

	_, err = f.store.Verify(ctx, h.ID, VerifyInput{Status: StatusRejected})
	wantKind(t, err, apperr.KindValidation, "reject without note")
	if h, err = f.store.Verify(ctx, h.ID, VerifyInput{Status: StatusApproved}); err != nil || h.VerificationStatus != StatusApproved {
		t.Fatalf("approve: %+v, %v", h, err)
	}
	var notified int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND kind = 'hospital_verification'`, f.owner).Scan(&notified)
	if notified != 1 {
		t.Fatalf("owner notifications = %d", notified)
	}

	for _, p := range []SearchParams{{Department: "cardio"}, {Location: "dhanmondi"}, {Facility: "icu"}, {Name: "green"}} {
		p.Limit = 20
		if list, _ := f.store.Search(ctx, p); len(list) != 1 {
			t.Errorf("search %+v found %d", p, len(list))
		}
	}
	if list, _ := f.store.Search(ctx, SearchParams{Department: "neurology", Limit: 20}); len(list) != 0 {
		t.Error("department filter matched a missing department")
	}

	upd := input("Green Life Medical College Hospital", "DGHS-9999")
	_, err = f.store.Update(ctx, f.owner, h.ID, upd)
	wantKind(t, err, apperr.KindValidation, "verified hospital changes registration number")
	upd.RegistrationNo = "DGHS-1234"
	if h, err = f.store.Update(ctx, f.owner, h.ID, upd); err != nil || h.Name != upd.Name || h.VerificationStatus != StatusApproved {
		t.Fatalf("update: %+v, %v", h, err)
	}

	// Staff members.
	dbtest.CreateUser(t, f.pool, "Patient", "01899999999", user.RolePatient)
	_, err = f.store.AddMember(ctx, h.ID, "01899999999")
	wantKind(t, err, apperr.KindValidation, "patient account as staff")
	staffPhone := phone()
	staff := dbtest.CreateUser(t, f.pool, "Receptionist", staffPhone, user.RoleHospitalAdmin)
	if m, err := f.store.AddMember(ctx, h.ID, staffPhone); err != nil || m.Role != RoleStaff {
		t.Fatalf("add staff: %+v, %v", m, err)
	}
	if role, _ := f.store.Role(ctx, staff, h.ID); role != RoleStaff {
		t.Fatalf("staff role = %q", role)
	}
	wantKind(t, f.store.RemoveMember(ctx, h.ID, f.owner), apperr.KindNotFound, "remove owner")
	if err := f.store.RemoveMember(ctx, h.ID, staff); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorsChambersAndBookings(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	hid := f.approvedHospital(t, "Square Hospital", "DGHS-1")
	otherHospital := f.approvedHospital(t, "Popular Hospital", "DGHS-2")
	doc := f.verifiedDoctor(t, "Dr Rahman", "A-11111")
	otherDoc := f.verifiedDoctor(t, "Dr Karim", "A-22222")
	patient := dbtest.CreateUser(t, f.pool, "Nusrat", "01811111111", user.RolePatient)

	// Next week's same weekday: always bookable regardless of the time the test runs.
	day := time.Now().In(appointment.Dhaka).AddDate(0, 0, 7)
	date, weekday := day.Format(time.DateOnly), int(day.Weekday())

	_, err := f.store.Invite(ctx, hid, f.owner, InviteInput{BMDCRegNo: "A-99999"})
	wantKind(t, err, apperr.KindNotFound, "unknown BMDC")
	sd, err := f.store.Invite(ctx, hid, f.owner, InviteInput{BMDCRegNo: " a-11111 ", Department: "Cardiology", Designation: "Consultant"})
	if err != nil || sd.Status != DoctorInvited || *sd.Designation != "Consultant" {
		t.Fatalf("invite: %+v, %v", sd, err)
	}
	_, err = f.store.Invite(ctx, hid, f.owner, InviteInput{BMDCRegNo: "A-11111"})
	wantKind(t, err, apperr.KindConflict, "invite twice")
	_, err = f.store.CreateChamber(ctx, hid, doc, ChamberInput{FeePaisa: 100000})
	wantKind(t, err, apperr.KindConflict, "chamber before acceptance")

	if inv, _ := f.store.Invitations(ctx, doc); len(inv) != 1 || inv[0].HospitalName != "Square Hospital" {
		t.Fatalf("invitations = %+v", inv)
	}
	if err := f.store.Respond(ctx, doc, hid, true); err != nil {
		t.Fatal(err)
	}
	wantKind(t, f.store.Respond(ctx, doc, hid, true), apperr.KindNotFound, "accept twice")

	ch, err := f.store.CreateChamber(ctx, hid, doc, ChamberInput{FeePaisa: 120000, Room: "Room 305"})
	if err != nil || ch.HospitalID == nil || *ch.HospitalName != "Square Hospital" || *ch.Room != "Room 305" || ch.Address != "House 1, Road 4, Dhanmondi" {
		t.Fatalf("chamber: %+v, %v", ch, err)
	}

	scope := doctor.HospitalScope(hid)
	evening, err := f.doctors.AddSchedule(ctx, scope, ch.ID, doctor.ScheduleInput{Weekday: weekday, StartTime: "17:00", EndTime: "20:00", MaxPatients: 20})
	if err != nil {
		t.Fatal(err)
	}
	morning, err := f.doctors.AddSchedule(ctx, scope, ch.ID, doctor.ScheduleInput{Weekday: weekday, StartTime: "09:00", EndTime: "11:00", MaxPatients: 10})
	if err != nil {
		t.Fatal(err)
	}

	// The doctor's private chamber cannot clash with hospital hours, and the doctor
	// cannot edit the hospital's chamber or schedule. Another hospital can't either.
	private, err := f.doctors.CreateChamber(ctx, doc, doctor.ChamberInput{Name: "Own Chamber", Address: "Mirpur 10, Dhaka", District: "Dhaka", FeePaisa: 80000})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.doctors.AddSchedule(ctx, doctor.DoctorScope(doc), private.ID, doctor.ScheduleInput{Weekday: weekday, StartTime: "19:00", EndTime: "22:00", MaxPatients: 5})
	wantKind(t, err, apperr.KindConflict, "private session overlapping hospital session")
	_, err = f.doctors.UpdateChamber(ctx, doc, ch.ID, doctor.ChamberInput{Name: "Hijack", Address: "Somewhere 123", District: "Dhaka"})
	wantKind(t, err, apperr.KindNotFound, "doctor edits hospital chamber")
	wantKind(t, f.doctors.RemoveSchedule(ctx, doctor.DoctorScope(doc), evening.ID), apperr.KindNotFound, "doctor removes hospital schedule")
	_, err = f.doctors.AddSchedule(ctx, doctor.HospitalScope(otherHospital), ch.ID, doctor.ScheduleInput{Weekday: 1, StartTime: "08:00", EndTime: "09:00", MaxPatients: 5})
	wantKind(t, err, apperr.KindNotFound, "other hospital adds schedule")

	// Public pages.
	page, err := f.store.Public(ctx, hid)
	if err != nil || len(page.Doctors) != 1 || len(page.Doctors[0].Chambers) != 1 || len(page.Doctors[0].Chambers[0].Schedules) != 2 {
		t.Fatalf("hospital page: %+v, %v", page, err)
	}
	pd, _ := f.doctors.GetPublic(ctx, doc)
	if len(pd.Chambers) != 2 {
		t.Fatalf("doctor page should list own and hospital chambers, got %d", len(pd.Chambers))
	}

	// Booking through the central e-ticketing system.
	a, err := f.appointments.Book(ctx, patient, evening.ID, date)
	if err != nil || a.HospitalName == nil || *a.HospitalName != "Square Hospital" || a.FeePaisa != 120000 {
		t.Fatalf("book: %+v, %v", a, err)
	}
	if list, _ := f.appointments.ListForHospital(ctx, hid, "", date); len(list) != 1 {
		t.Fatalf("hospital list = %d", len(list))
	}
	if list, _ := f.appointments.ListForHospital(ctx, otherHospital, "", date); len(list) != 0 {
		t.Fatal("other hospital sees the booking")
	}

	// The hospital and the chamber's doctor can both run the live serial status;
	// another hospital can't.
	for _, who := range []appointment.QueueActor{{HospitalID: hid}, {DoctorID: doc}} {
		if q, err := f.appointments.QueueToday(ctx, who, evening.ID); err != nil || q.Started || q.MinutesPerPatient != 9 {
			t.Fatalf("queue for %+v: %+v, %v", who, q, err)
		}
	}
	_, err = f.appointments.QueueToday(ctx, appointment.QueueActor{HospitalID: otherHospital}, evening.ID)
	wantKind(t, err, apperr.KindNotFound, "other hospital's queue")

	// Reschedule to the morning session.
	_, err = f.appointments.Reschedule(ctx, otherHospital, a.ID, morning.ID, date)
	wantKind(t, err, apperr.KindNotFound, "other hospital reschedules")
	moved, err := f.appointments.Reschedule(ctx, hid, a.ID, morning.ID, date)
	if err != nil || moved.ScheduleID != morning.ID || moved.SerialNo != 1 || moved.TicketCode == a.TicketCode {
		t.Fatalf("reschedule: %+v, %v", moved, err)
	}
	old, _ := f.appointments.GetForHospital(ctx, hid, a.ID)
	if old.Status != "rescheduled" || old.RescheduledTo == nil || *old.RescheduledTo != moved.ID {
		t.Fatalf("old appointment = %+v", old)
	}
	_, err = f.appointments.Reschedule(ctx, hid, a.ID, evening.ID, date)
	wantKind(t, err, apperr.KindConflict, "reschedule an already rescheduled booking")

	// A schedule of another doctor is not a valid target.
	if _, err := f.store.Invite(ctx, hid, f.owner, InviteInput{BMDCRegNo: "A-22222"}); err != nil {
		t.Fatal(err)
	}
	f.store.Respond(ctx, otherDoc, hid, true)
	och, _ := f.store.CreateChamber(ctx, hid, otherDoc, ChamberInput{FeePaisa: 50000})
	osc, _ := f.doctors.AddSchedule(ctx, scope, och.ID, doctor.ScheduleInput{Weekday: weekday, StartTime: "12:00", EndTime: "13:00", MaxPatients: 5})
	_, err = f.appointments.Reschedule(ctx, hid, moved.ID, osc.ID, date)
	wantKind(t, err, apperr.KindValidation, "reschedule to another doctor")

	_, err = f.appointments.CancelByHospital(ctx, hid, moved.ID, "no")
	wantKind(t, err, apperr.KindValidation, "cancel reason too short")
	cancelled, err := f.appointments.CancelByHospital(ctx, hid, moved.ID, "Doctor is attending a conference")
	if err != nil || cancelled.Status != "cancelled" || *cancelled.CancelledBy != "hospital" {
		t.Fatalf("cancel: %+v, %v", cancelled, err)
	}
	var sms int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1
		AND kind IN ('appointment_rescheduled', 'appointment_cancelled') AND sms_status = 'pending'`, patient).Scan(&sms)
	if sms != 2 {
		t.Fatalf("patient SMS for reschedule+cancel = %d", sms)
	}

	// Removing the doctor stops bookings but reports the upcoming ones.
	if _, err := f.appointments.Book(ctx, patient, evening.ID, date); err != nil {
		t.Fatal(err)
	}
	upcoming, err := f.store.RemoveStaff(ctx, hid, doc)
	if err != nil || upcoming != 1 {
		t.Fatalf("remove: upcoming=%d, %v", upcoming, err)
	}
	p2 := dbtest.CreateUser(t, f.pool, "Second", "01822222222", user.RolePatient)
	_, err = f.appointments.Book(ctx, p2, evening.ID, date)
	wantKind(t, err, apperr.KindConflict, "booking after doctor removed")
	if page, _ = f.store.Public(ctx, hid); len(page.Doctors) != 1 || page.Doctors[0].DoctorID != otherDoc {
		t.Fatalf("hospital page after removal: %+v", page.Doctors)
	}
	if pd, _ = f.doctors.GetPublic(ctx, doc); len(pd.Chambers) != 1 {
		t.Fatalf("doctor page after removal lists %d chambers", len(pd.Chambers))
	}

	// If the hospital loses its verification, its chambers stop taking bookings.
	if _, err := f.store.Verify(ctx, hid, VerifyInput{Status: StatusRejected, Note: "licence expired"}); err != nil {
		t.Fatal(err)
	}
	_, err = f.appointments.Book(ctx, p2, osc.ID, date)
	wantKind(t, err, apperr.KindConflict, "booking at unverified hospital")

	// A removed doctor can be invited again.
	f.store.Verify(ctx, hid, VerifyInput{Status: StatusApproved})
	if _, err := f.store.Invite(ctx, hid, f.owner, InviteInput{BMDCRegNo: "A-11111"}); err != nil {
		t.Fatalf("re-invite: %v", err)
	}
	if n, err := f.store.Leave(ctx, otherDoc, hid); err != nil || n != 0 {
		t.Fatalf("leave: %d, %v", n, err)
	}
}

func TestPhotos(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	hid := f.approvedHospital(t, "Ibn Sina", "DGHS-7")
	png := "\x89PNG\r\n\x1a\nfake image bytes"

	first, err := f.store.AddPhoto(ctx, hid, "image/png", strings.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.AddPhoto(ctx, hid, "image/gif", strings.NewReader("GIF89a"))
	wantKind(t, err, apperr.KindValidation, "gif upload")
	for range MaxPhotos - 1 {
		if _, err := f.store.AddPhoto(ctx, hid, "image/jpeg", strings.NewReader("\xff\xd8\xff jpeg")); err != nil {
			t.Fatal(err)
		}
	}
	_, err = f.store.AddPhoto(ctx, hid, "image/png", strings.NewReader(png))
	wantKind(t, err, apperr.KindConflict, "too many photos")

	page, _ := f.store.Public(ctx, hid)
	if len(page.Photos) != MaxPhotos || page.LogoURL == nil || !strings.HasSuffix(*page.LogoURL, first.ID) {
		t.Fatalf("photos=%d logo=%v", len(page.Photos), page.LogoURL)
	}
	ct, size, rc, err := f.store.OpenPhoto(ctx, hid, first.ID)
	if err != nil || ct != "image/png" || size != int64(len(png)) {
		t.Fatalf("open: %s %d %v", ct, size, err)
	}
	rc.Close()

	if err := f.store.SetLogo(ctx, hid, page.Photos[3].ID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DeletePhoto(ctx, hid, page.Photos[3].ID); err != nil {
		t.Fatal(err)
	}
	if page, _ = f.store.Public(ctx, hid); page.LogoURL != nil || len(page.Photos) != MaxPhotos-1 {
		t.Fatalf("after deleting logo: logo=%v photos=%d", page.LogoURL, len(page.Photos))
	}
	other := f.approvedHospital(t, "Labaid", "DGHS-8")
	wantKind(t, f.store.DeletePhoto(ctx, other, first.ID), apperr.KindNotFound, "delete another hospital's photo")
}
