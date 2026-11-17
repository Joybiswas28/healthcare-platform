package doctor

import (
	"context"
	"testing"

	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/user"
)

func validProfile(bmdc string) ProfileInput {
	return ProfileInput{BMDCRegNo: bmdc, Specialization: "Cardiology", Degrees: "MBBS, MD", ExperienceYears: 10}
}

func validChamber(district string) ChamberInput {
	return ChamberInput{Name: "City Chamber", Address: "12 Station Road", District: district, FeePaisa: 100000}
}

func wantKind(t *testing.T, err error, k apperr.Kind, what string) {
	t.Helper()
	if !apperr.Is(err, k) {
		t.Fatalf("%s: got error %v, want kind %d", what, err, k)
	}
}

func TestProfileAndVerificationFlow(t *testing.T) {
	pool := dbtest.New(t)
	s := NewStore(pool)
	ctx := context.Background()
	d1 := dbtest.CreateUser(t, pool, "Dr One", "01711111111", user.RoleDoctor)
	d2 := dbtest.CreateUser(t, pool, "Dr Two", "01722222222", user.RoleDoctor)

	_, err := s.UpdateProfile(ctx, d1, validProfile("not a number"))
	wantKind(t, err, apperr.KindValidation, "bad BMDC")

	_, err = s.Verify(ctx, d1, VerifyInput{Status: StatusApproved})
	wantKind(t, err, apperr.KindValidation, "approve incomplete profile")

	p, err := s.UpdateProfile(ctx, d1, validProfile("a-12345"))
	if err != nil {
		t.Fatal(err)
	}
	if *p.BMDCRegNo != "A-12345" || p.VerificationStatus != StatusPending {
		t.Fatalf("profile = %+v", p)
	}

	_, err = s.UpdateProfile(ctx, d2, validProfile("A-12345"))
	wantKind(t, err, apperr.KindConflict, "duplicate BMDC")

	_, err = s.Verify(ctx, d1, VerifyInput{Status: StatusRejected})
	wantKind(t, err, apperr.KindValidation, "reject without note")

	p, err = s.Verify(ctx, d1, VerifyInput{Status: StatusRejected, Note: "certificate unreadable"})
	if err != nil || p.VerificationStatus != StatusRejected {
		t.Fatalf("reject: %+v, %v", p, err)
	}

	// Resubmitting after rejection puts the doctor back in the queue.
	p, err = s.UpdateProfile(ctx, d1, validProfile("A-12345"))
	if err != nil || p.VerificationStatus != StatusPending {
		t.Fatalf("resubmit: %+v, %v", p, err)
	}

	if _, err := s.Verify(ctx, d1, VerifyInput{Status: StatusApproved}); err != nil {
		t.Fatal(err)
	}
	_, err = s.UpdateProfile(ctx, d1, validProfile("A-99999"))
	wantKind(t, err, apperr.KindValidation, "approved doctor changes BMDC")

	pending, err := s.ListForAdmin(ctx, StatusPending, 20, 0)
	if err != nil || len(pending) != 1 || pending[0].UserID != d2 {
		t.Fatalf("pending list: %+v, %v", pending, err)
	}
}

func TestSchedules(t *testing.T) {
	pool := dbtest.New(t)
	s := NewStore(pool)
	ctx := context.Background()
	d1 := dbtest.CreateUser(t, pool, "Dr One", "01711111111", user.RoleDoctor)
	d2 := dbtest.CreateUser(t, pool, "Dr Two", "01722222222", user.RoleDoctor)

	c1, err := s.CreateChamber(ctx, d1, validChamber("Dhaka"))
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.CreateChamber(ctx, d1, validChamber("Gazipur"))
	if err != nil {
		t.Fatal(err)
	}

	evening := ScheduleInput{Weekday: 0, StartTime: "17:00", EndTime: "21:00", MaxPatients: 20}
	sc, err := s.AddSchedule(ctx, DoctorScope(d1), c1.ID, evening)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.AddSchedule(ctx, DoctorScope(d1), c2.ID, ScheduleInput{Weekday: 0, StartTime: "20:00", EndTime: "22:00", MaxPatients: 5})
	wantKind(t, err, apperr.KindConflict, "overlap across chambers")

	if _, err := s.AddSchedule(ctx, DoctorScope(d1), c2.ID, ScheduleInput{Weekday: 0, StartTime: "21:00", EndTime: "22:00", MaxPatients: 5}); err != nil {
		t.Fatalf("back-to-back session should be allowed: %v", err)
	}

	_, err = s.AddSchedule(ctx, DoctorScope(d1), c1.ID, ScheduleInput{Weekday: 1, StartTime: "21:00", EndTime: "20:00", MaxPatients: 5})
	wantKind(t, err, apperr.KindValidation, "end before start")

	_, err = s.AddSchedule(ctx, DoctorScope(d2), c1.ID, ScheduleInput{Weekday: 3, StartTime: "09:00", EndTime: "10:00", MaxPatients: 5})
	wantKind(t, err, apperr.KindNotFound, "schedule on someone else's chamber")

	wantKind(t, s.RemoveSchedule(ctx, DoctorScope(d2), sc.ID), apperr.KindNotFound, "remove someone else's schedule")
	if err := s.RemoveSchedule(ctx, DoctorScope(d1), sc.ID); err != nil {
		t.Fatal(err)
	}
	// The slot is free again once removed.
	if _, err := s.AddSchedule(ctx, DoctorScope(d1), c2.ID, evening); err != nil {
		t.Fatalf("re-add after removal: %v", err)
	}
}

func TestSearchAndPublicProfile(t *testing.T) {
	pool := dbtest.New(t)
	s := NewStore(pool)
	ctx := context.Background()

	approved := dbtest.CreateUser(t, pool, "Dr Rahman", "01711111111", user.RoleDoctor)
	pending := dbtest.CreateUser(t, pool, "Dr Rahim", "01722222222", user.RoleDoctor)
	for i, id := range []string{approved, pending} {
		if _, err := s.UpdateProfile(ctx, id, validProfile([]string{"A-1111", "A-2222"}[i])); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Verify(ctx, approved, VerifyInput{Status: StatusApproved}); err != nil {
		t.Fatal(err)
	}

	open, err := s.CreateChamber(ctx, approved, validChamber("Sylhet"))
	if err != nil {
		t.Fatal(err)
	}
	closed := validChamber("Khulna")
	off := false
	closed.BookingEnabled = &off
	if _, err := s.CreateChamber(ctx, approved, closed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddSchedule(ctx, DoctorScope(approved), open.ID, ScheduleInput{Weekday: 5, StartTime: "15:00", EndTime: "18:00", MaxPatients: 10}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		params SearchParams
		want   int
	}{
		{"all approved", SearchParams{}, 1},
		{"by name", SearchParams{Name: "rah"}, 1},
		{"by specialization", SearchParams{Specialization: "cardio"}, 1},
		{"by location", SearchParams{Location: "sylhet"}, 1},
		{"disabled chamber location", SearchParams{Location: "khulna"}, 0},
		{"wildcard is literal", SearchParams{Name: "%"}, 0},
	}
	for _, c := range cases {
		c.params.Limit = 20
		got, err := s.Search(ctx, c.params)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != c.want {
			t.Errorf("%s: got %d results, want %d", c.name, len(got), c.want)
		}
	}

	res, _ := s.Search(ctx, SearchParams{Limit: 20})
	if len(res) == 1 && (*res[0].MinFeePaisa != 100000 || len(res[0].Districts) != 1) {
		t.Errorf("summary = %+v", res[0])
	}

	pub, err := s.GetPublic(ctx, approved)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub.Chambers) != 1 || len(pub.Chambers[0].Schedules) != 1 {
		t.Fatalf("public profile chambers = %+v", pub.Chambers)
	}

	_, err = s.GetPublic(ctx, pending)
	wantKind(t, err, apperr.KindNotFound, "pending doctor is not public")
}
