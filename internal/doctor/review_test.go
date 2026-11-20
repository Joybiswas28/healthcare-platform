package doctor

import (
	"context"
	"testing"

	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/user"
)

func TestReviewsAfterAChamberVisit(t *testing.T) {
	pool := dbtest.New(t)
	s := NewStore(pool)
	ctx := context.Background()
	doc := dbtest.CreateUser(t, pool, "Dr One", "01711111111", user.RoleDoctor)
	pat := dbtest.CreateUser(t, pool, "Patient", "01811111111", user.RolePatient)
	other := dbtest.CreateUser(t, pool, "Someone Else", "01822222222", user.RolePatient)

	if _, err := s.UpdateProfile(ctx, doc, validProfile("A-12345")); err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateChamber(ctx, doc, validChamber("Dhaka"))
	if err != nil {
		t.Fatal(err)
	}
	sc, err := s.AddSchedule(ctx, DoctorScope(doc), c.ID, ScheduleInput{Weekday: 1, StartTime: "17:00", EndTime: "21:00", MaxPatients: 10})
	if err != nil {
		t.Fatal(err)
	}
	var apptID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO appointments (ticket_code, schedule_id, chamber_id, doctor_id, patient_id, appointment_date, serial_no, fee_paisa)
		VALUES ('TICKET01', $1, $2, $3, $4, '2026-01-05', 1, 100000) RETURNING id`,
		sc.ID, c.ID, doc, pat).Scan(&apptID); err != nil {
		t.Fatal(err)
	}

	_, err = s.ReviewAppointment(ctx, pat, apptID, ReviewInput{Rating: 4})
	wantKind(t, err, apperr.KindConflict, "review before the visit")

	if _, err := pool.Exec(ctx, `UPDATE appointments SET status = 'completed' WHERE id = $1`, apptID); err != nil {
		t.Fatal(err)
	}
	_, err = s.ReviewAppointment(ctx, pat, apptID, ReviewInput{Rating: 0})
	wantKind(t, err, apperr.KindValidation, "rating 0")
	_, err = s.ReviewAppointment(ctx, other, apptID, ReviewInput{Rating: 5})
	wantKind(t, err, apperr.KindNotFound, "someone else's appointment")

	v, err := s.ReviewAppointment(ctx, pat, apptID, ReviewInput{Rating: 4, Comment: "  Short wait, kind doctor.  "})
	if err != nil {
		t.Fatal(err)
	}
	if v.Source != "chamber" || v.Comment == nil || *v.Comment != "Short wait, kind doctor." {
		t.Fatalf("review = %+v", v)
	}
	_, err = s.ReviewAppointment(ctx, pat, apptID, ReviewInput{Rating: 1})
	wantKind(t, err, apperr.KindConflict, "second review")

	var notes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND kind = 'doctor_review'`, doc).Scan(&notes); err != nil || notes != 1 {
		t.Fatalf("doctor notifications = %d, %v", notes, err)
	}

	p, err := s.GetProfile(ctx, doc)
	if err != nil || p.Rating == nil || *p.Rating != 4 || p.ReviewCount != 1 {
		t.Fatalf("profile rating = %v / %d, %v", p.Rating, p.ReviewCount, err)
	}

	// Only the reviewed doctor can answer, and an empty answer removes it.
	_, err = s.Reply(ctx, other, v.ID, ReplyInput{Reply: "Not mine"})
	wantKind(t, err, apperr.KindNotFound, "reply by another user")
	v, err = s.Reply(ctx, doc, v.ID, ReplyInput{Reply: "Thank you!"})
	if err != nil || v.Reply == nil || v.RepliedAt == nil {
		t.Fatalf("reply = %+v, %v", v, err)
	}
	v, err = s.Reply(ctx, doc, v.ID, ReplyInput{Reply: " "})
	if err != nil || v.Reply != nil || v.RepliedAt != nil {
		t.Fatalf("cleared reply = %+v, %v", v, err)
	}

	mine, err := s.MyReviews(ctx, pat)
	if err != nil || len(mine) != 1 || mine[0].AppointmentID == nil || *mine[0].AppointmentID != apptID {
		t.Fatalf("my reviews = %+v, %v", mine, err)
	}
	if mine, _ := s.MyReviews(ctx, other); len(mine) != 0 {
		t.Fatalf("other patient's reviews = %+v", mine)
	}
}
