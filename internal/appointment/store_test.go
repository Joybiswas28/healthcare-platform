package appointment

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/user"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Tuesday 17 Nov 2026, 09:00 in Dhaka.
var fixedNow = time.Date(2026, 11, 17, 9, 0, 0, 0, Dhaka)

const today = "2026-11-17"

type fixture struct {
	pool       *pgxpool.Pool
	store      *Store
	doctorID   string
	scheduleID string
}

func setup(t *testing.T, maxPatients int) fixture {
	t.Helper()
	ctx := context.Background()
	pool := dbtest.New(t)

	doctorID := dbtest.CreateUser(t, pool, "Dr Karim", "01711111111", user.RoleDoctor)
	ds := doctor.NewStore(pool)
	must(t, func() error {
		_, err := ds.UpdateProfile(ctx, doctorID, doctor.ProfileInput{
			BMDCRegNo: "A-12345", Specialization: "Medicine", Degrees: "MBBS, FCPS", ExperienceYears: 8,
		})
		return err
	})
	must(t, func() error {
		_, err := ds.Verify(ctx, doctorID, doctor.VerifyInput{Status: doctor.StatusApproved})
		return err
	})
	ch, err := ds.CreateChamber(ctx, doctorID, doctor.ChamberInput{
		Name: "Green Life Chamber", Address: "House 16, Road 2, Dhanmondi", District: "Dhaka", FeePaisa: 80000,
	})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ds.AddSchedule(ctx, doctor.DoctorScope(doctorID), ch.ID, doctor.ScheduleInput{
		Weekday: int(fixedNow.Weekday()), StartTime: "17:00", EndTime: "21:00", MaxPatients: maxPatients,
	})
	if err != nil {
		t.Fatal(err)
	}

	st := NewStore(pool)
	st.now = func() time.Time { return fixedNow }
	return fixture{pool: pool, store: st, doctorID: doctorID, scheduleID: sc.ID}
}

func (f fixture) patient(t *testing.T, i int) string {
	t.Helper()
	return dbtest.CreateUser(t, f.pool, fmt.Sprintf("Patient %d", i), fmt.Sprintf("0181%07d", i), user.RolePatient)
}

func must(t *testing.T, fn func() error) {
	t.Helper()
	if err := fn(); err != nil {
		t.Fatal(err)
	}
}

func wantKind(t *testing.T, err error, k apperr.Kind, what string) {
	t.Helper()
	if !apperr.Is(err, k) {
		t.Fatalf("%s: got error %v, want kind %d", what, err, k)
	}
}

func TestBookAssignsSequentialSerials(t *testing.T) {
	f := setup(t, 10)
	ctx := context.Background()

	a1, err := f.store.Book(ctx, f.patient(t, 1), f.scheduleID, today)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := f.store.Book(ctx, f.patient(t, 2), f.scheduleID, today)
	if err != nil {
		t.Fatal(err)
	}

	if a1.SerialNo != 1 || a2.SerialNo != 2 {
		t.Fatalf("serials = %d, %d; want 1, 2", a1.SerialNo, a2.SerialNo)
	}
	if a1.Status != StatusConfirmed || a1.FeePaisa != 80000 || a1.Date != today || a1.StartTime != "17:00" {
		t.Fatalf("unexpected appointment: %+v", a1)
	}
	if len(a1.TicketCode) != 8 || a1.TicketCode == a2.TicketCode {
		t.Fatalf("bad ticket codes %q, %q", a1.TicketCode, a2.TicketCode)
	}

	av, err := f.store.Availability(ctx, f.scheduleID, today)
	if err != nil {
		t.Fatal(err)
	}
	if av.Booked != 2 || av.Remaining != 8 {
		t.Fatalf("availability = %+v", av)
	}
}

func TestBookRules(t *testing.T) {
	f := setup(t, 2)
	ctx := context.Background()
	p1, p2, p3 := f.patient(t, 1), f.patient(t, 2), f.patient(t, 3)

	first, err := f.store.Book(ctx, p1, f.scheduleID, today)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.store.Book(ctx, p1, f.scheduleID, today)
	wantKind(t, err, apperr.KindConflict, "same patient twice")

	_, err = f.store.Book(ctx, p2, f.scheduleID, "2026-11-18")
	wantKind(t, err, apperr.KindValidation, "wrong weekday")

	_, err = f.store.Book(ctx, p2, f.scheduleID, "2026-11-10")
	wantKind(t, err, apperr.KindValidation, "past date")

	_, err = f.store.Book(ctx, p2, f.scheduleID, "2026-12-29")
	wantKind(t, err, apperr.KindValidation, "beyond booking window")

	_, err = f.store.Book(ctx, p2, f.scheduleID, "17-11-2026")
	wantKind(t, err, apperr.KindValidation, "bad date format")

	_, err = f.store.Book(ctx, f.doctorID, f.scheduleID, today)
	wantKind(t, err, apperr.KindValidation, "doctor booking themselves")

	if _, err := f.store.Book(ctx, p2, f.scheduleID, today); err != nil {
		t.Fatal(err)
	}
	_, err = f.store.Book(ctx, p3, f.scheduleID, today)
	wantKind(t, err, apperr.KindConflict, "fully booked")

	// Cancelling frees a place, but the serial is not reused.
	if _, err := f.store.Cancel(ctx, p1, first.ID); err != nil {
		t.Fatal(err)
	}
	a3, err := f.store.Book(ctx, p3, f.scheduleID, today)
	if err != nil {
		t.Fatal(err)
	}
	if a3.SerialNo != 3 {
		t.Fatalf("serial after cancellation = %d, want 3", a3.SerialNo)
	}

	// The session is full again, so the patient who cancelled cannot get back in.
	_, err = f.store.Book(ctx, p1, f.scheduleID, today)
	wantKind(t, err, apperr.KindConflict, "rebook into full session")

	// Once today's session is over, it can no longer be booked.
	f.store.now = func() time.Time { return fixedNow.Add(12*time.Hour + 30*time.Minute) } // 21:30
	_, err = f.store.Book(ctx, f.patient(t, 4), f.scheduleID, today)
	wantKind(t, err, apperr.KindValidation, "session ended")
}

func TestConcurrentBookingNeverOverbooks(t *testing.T) {
	const capacity, patients = 5, 20
	f := setup(t, capacity)
	ctx := context.Background()

	ids := make([]string, patients)
	for i := range ids {
		ids[i] = f.patient(t, i)
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		serials   []int
		conflicts int
	)
	for _, pid := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := f.store.Book(ctx, pid, f.scheduleID, today)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				serials = append(serials, a.SerialNo)
			case apperr.Is(err, apperr.KindConflict):
				conflicts++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	sort.Ints(serials)
	if fmt.Sprint(serials) != "[1 2 3 4 5]" || conflicts != patients-capacity {
		t.Fatalf("serials = %v, conflicts = %d", serials, conflicts)
	}
}

func TestUnapprovedDoctorCannotBeBooked(t *testing.T) {
	f := setup(t, 10)
	ctx := context.Background()

	ds := doctor.NewStore(f.pool)
	pending := dbtest.CreateUser(t, f.pool, "Dr Pending", "01722222222", user.RoleDoctor)
	ch, err := ds.CreateChamber(ctx, pending, doctor.ChamberInput{
		Name: "New Chamber", Address: "Agrabad, Chattogram", District: "Chattogram", FeePaisa: 50000,
	})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ds.AddSchedule(ctx, doctor.DoctorScope(pending), ch.ID, doctor.ScheduleInput{
		Weekday: int(fixedNow.Weekday()), StartTime: "10:00", EndTime: "12:00", MaxPatients: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.store.Book(ctx, f.patient(t, 1), sc.ID, today)
	wantKind(t, err, apperr.KindConflict, "pending doctor")

	_, err = f.store.Availability(ctx, sc.ID, today)
	wantKind(t, err, apperr.KindNotFound, "availability of pending doctor")
}

func TestCancelAndOutcome(t *testing.T) {
	f := setup(t, 10)
	ctx := context.Background()
	p1, p2 := f.patient(t, 1), f.patient(t, 2)

	a, err := f.store.Book(ctx, p1, f.scheduleID, today)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.store.Cancel(ctx, p2, a.ID)
	wantKind(t, err, apperr.KindNotFound, "cancel someone else's appointment")

	other := dbtest.CreateUser(t, f.pool, "Dr Other", "01733333333", user.RoleDoctor)
	_, err = f.store.SetOutcome(ctx, other, a.ID, StatusCompleted)
	wantKind(t, err, apperr.KindNotFound, "other doctor marks outcome")

	done, err := f.store.SetOutcome(ctx, f.doctorID, a.ID, StatusCompleted)
	if err != nil || done.Status != StatusCompleted {
		t.Fatalf("complete: %+v, %v", done, err)
	}

	_, err = f.store.Cancel(ctx, p1, a.ID)
	wantKind(t, err, apperr.KindConflict, "cancel completed appointment")

	future, err := f.store.Book(ctx, p2, f.scheduleID, "2026-11-24")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.SetOutcome(ctx, f.doctorID, future.ID, StatusNoShow)
	wantKind(t, err, apperr.KindConflict, "no-show for a future appointment")

	got, err := f.store.FindByTicket(ctx, f.doctorID, " "+lower(future.TicketCode)+" ")
	if err != nil || got.ID != future.ID {
		t.Fatalf("FindByTicket: %+v, %v", got, err)
	}
	_, err = f.store.FindByTicket(ctx, other, future.TicketCode)
	wantKind(t, err, apperr.KindNotFound, "ticket of another doctor")

	day, err := f.store.ListForDoctor(ctx, f.doctorID, today)
	if err != nil || len(day) != 1 || day[0].PatientName != "Patient 1" {
		t.Fatalf("ListForDoctor: %+v, %v", day, err)
	}
	mine, err := f.store.ListForPatient(ctx, p2, 20, 0)
	if err != nil || len(mine) != 1 || mine[0].DoctorName != "Dr Karim" {
		t.Fatalf("ListForPatient: %+v, %v", mine, err)
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
