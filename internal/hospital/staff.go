package hospital

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
)

// ---------------------------------------------------------------- doctors on staff

const staffSelect = `
SELECT u.id, u.name, d.specialization, d.degrees, d.experience_years, d.bmdc_reg_no,
       hd.department, hd.designation, hd.status
FROM hospital_doctors hd
JOIN doctor_profiles d ON d.user_id = hd.doctor_id
JOIN users u           ON u.id = hd.doctor_id`

func scanStaff(r pgx.CollectableRow) (StaffDoctor, error) {
	var sd StaffDoctor
	err := r.Scan(&sd.DoctorID, &sd.Name, &sd.Specialization, &sd.Degrees, &sd.ExperienceYears, &sd.BMDCRegNo,
		&sd.Department, &sd.Designation, &sd.Status)
	return sd, err
}

// staff lists a hospital's doctors. publicOnly keeps active, verified doctors.
func (s *Store) staff(ctx context.Context, hospitalID string, publicOnly bool) ([]StaffDoctor, error) {
	rows, err := s.db.Query(ctx, staffSelect+`
		WHERE hd.hospital_id = $1
		  AND (NOT $2 OR (hd.status = 'active' AND d.verification_status = 'approved' AND u.status = 'active'))
		ORDER BY hd.department NULLS LAST, u.name`, hospitalID, publicOnly)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanStaff)
}

// Staff lists all of a hospital's doctor associations (any status) with their hospital chambers.
func (s *Store) Staff(ctx context.Context, hospitalID string) ([]StaffDoctor, error) {
	list, err := s.staff(ctx, hospitalID, false)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Chambers, err = s.doctors.ListHospitalChambers(ctx, hospitalID, list[i].DoctorID, false); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func (s *Store) staffMember(ctx context.Context, q database.Querier, hospitalID, doctorID string) (StaffDoctor, error) {
	row := q.QueryRow(ctx, staffSelect+` WHERE hd.hospital_id = $1 AND hd.doctor_id = $2`, hospitalID, doctorID)
	var sd StaffDoctor
	err := row.Scan(&sd.DoctorID, &sd.Name, &sd.Specialization, &sd.Degrees, &sd.ExperienceYears, &sd.BMDCRegNo,
		&sd.Department, &sd.Designation, &sd.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return StaffDoctor{}, apperr.NotFound("this doctor is not associated with the hospital")
	}
	return sd, err
}

// Invite asks a verified doctor (found by BMDC number) to join the hospital.
// A doctor who declined, left or was removed can be invited again.
func (s *Store) Invite(ctx context.Context, hospitalID, inviterID string, in InviteInput) (StaffDoctor, error) {
	bmdc := strings.ToUpper(strings.TrimSpace(in.BMDCRegNo))
	staffIn := StaffInput{Department: in.Department, Designation: in.Designation}
	if err := staffIn.normalize(); err != nil {
		return StaffDoctor{}, err
	}
	if bmdc == "" {
		return StaffDoctor{}, apperr.Validation("bmdc_reg_no is required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return StaffDoctor{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var hospitalName, hospitalStatus string
	if err := tx.QueryRow(ctx, `SELECT name, verification_status FROM hospitals WHERE id = $1`,
		hospitalID).Scan(&hospitalName, &hospitalStatus); err != nil {
		return StaffDoctor{}, err
	}
	if hospitalStatus != StatusApproved {
		return StaffDoctor{}, apperr.Conflict("the hospital must be verified before it can invite doctors")
	}

	var doctorID string
	err = tx.QueryRow(ctx, `
		SELECT d.user_id FROM doctor_profiles d JOIN users u ON u.id = d.user_id
		WHERE d.bmdc_reg_no = $1 AND d.verification_status = 'approved' AND u.status = 'active'`, bmdc).Scan(&doctorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return StaffDoctor{}, apperr.NotFound("no verified doctor with this BMDC number is registered on the platform")
	}
	if err != nil {
		return StaffDoctor{}, err
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO hospital_doctors (hospital_id, doctor_id, department, designation, invited_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (hospital_id, doctor_id) DO UPDATE
		    SET status = 'invited', department = EXCLUDED.department, designation = EXCLUDED.designation,
		        invited_by = EXCLUDED.invited_by, updated_at = now()
		    WHERE hospital_doctors.status IN ('declined', 'removed', 'left')`,
		hospitalID, doctorID, nullIfEmpty(staffIn.Department), nullIfEmpty(staffIn.Designation), inviterID)
	if err != nil {
		return StaffDoctor{}, err
	}
	if tag.RowsAffected() == 0 {
		return StaffDoctor{}, apperr.Conflict("this doctor is already invited or on staff")
	}
	if err := notify.Enqueue(ctx, tx, notify.Msg{
		UserID: doctorID,
		Kind:   notify.KindHospitalInvitation,
		Title:  "Hospital invitation",
		Body:   hospitalName + " invited you to join its doctors. Open the app to accept or decline.",
		SMS:    true,
	}); err != nil {
		return StaffDoctor{}, err
	}
	sd, err := s.staffMember(ctx, tx, hospitalID, doctorID)
	if err != nil {
		return StaffDoctor{}, err
	}
	return sd, tx.Commit(ctx)
}

// UpdateStaff sets the hospital-specific details of a doctor (not their profile).
func (s *Store) UpdateStaff(ctx context.Context, hospitalID, doctorID string, in StaffInput) (StaffDoctor, error) {
	if err := in.normalize(); err != nil {
		return StaffDoctor{}, err
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE hospital_doctors SET department = $3, designation = $4, updated_at = now()
		WHERE hospital_id = $1 AND doctor_id = $2 AND status IN ('invited', 'active')`,
		hospitalID, doctorID, nullIfEmpty(in.Department), nullIfEmpty(in.Designation))
	if err != nil {
		return StaffDoctor{}, err
	}
	if tag.RowsAffected() == 0 {
		return StaffDoctor{}, apperr.NotFound("this doctor is not invited or on staff")
	}
	return s.staffMember(ctx, s.db, hospitalID, doctorID)
}

// endAssociation moves an invited/active association to a final status and turns
// off booking at the doctor's chambers in this hospital. Existing bookings are kept
// so the hospital can cancel or reschedule them.
func (s *Store) endAssociation(ctx context.Context, hospitalID, doctorID string, from []string, to string) (int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE hospital_doctors SET status = $3, updated_at = now()
		WHERE hospital_id = $1 AND doctor_id = $2 AND status = ANY($4)`, hospitalID, doctorID, to, from)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 0, apperr.NotFound("no matching association with this hospital")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE chambers SET booking_enabled = false, updated_at = now()
		WHERE hospital_id = $1 AND doctor_id = $2`, hospitalID, doctorID); err != nil {
		return 0, err
	}
	var upcoming int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM appointments a JOIN chambers c ON c.id = a.chamber_id
		WHERE c.hospital_id = $1 AND a.doctor_id = $2 AND a.status = 'confirmed' AND a.appointment_date >= $3::date`,
		hospitalID, doctorID, s.now().In(dhaka).Format("2006-01-02")).Scan(&upcoming); err != nil {
		return 0, err
	}

	var hospitalName, doctorName string
	if err := tx.QueryRow(ctx, `SELECT h.name, u.name FROM hospitals h, users u WHERE h.id = $1 AND u.id = $2`,
		hospitalID, doctorID).Scan(&hospitalName, &doctorName); err != nil {
		return 0, err
	}
	switch to {
	case DoctorRemoved:
		err = notify.Enqueue(ctx, tx, notify.Msg{UserID: doctorID, Kind: notify.KindHospitalStaffChange,
			Title: "Removed from hospital", Body: hospitalName + " has removed you from its doctors."})
	case DoctorLeft:
		err = s.notifyMembers(ctx, tx, hospitalID, notify.KindHospitalStaffChange, "Doctor left",
			doctorName+" has left "+hospitalName+". Review their upcoming bookings.", false)
	case DoctorDeclined:
		err = s.notifyMembers(ctx, tx, hospitalID, notify.KindHospitalStaffChange, "Invitation declined",
			doctorName+" declined the invitation to join "+hospitalName+".", false)
	}
	if err != nil {
		return 0, err
	}
	return upcoming, tx.Commit(ctx)
}

// RemoveStaff removes a doctor (or withdraws an invitation). It returns how many
// upcoming bookings the hospital still needs to cancel or reschedule.
func (s *Store) RemoveStaff(ctx context.Context, hospitalID, doctorID string) (int, error) {
	return s.endAssociation(ctx, hospitalID, doctorID, []string{DoctorInvited, DoctorActive}, DoctorRemoved)
}

// ---------------------------------------------------------------- doctor side

func (s *Store) Invitations(ctx context.Context, doctorID string) ([]Invitation, error) {
	rows, err := s.db.Query(ctx, `
		SELECT h.id, h.name, h.district, hd.department, hd.designation, hd.status, hd.created_at
		FROM hospital_doctors hd JOIN hospitals h ON h.id = hd.hospital_id
		WHERE hd.doctor_id = $1 AND hd.status IN ('invited', 'active')
		ORDER BY hd.status DESC, h.name`, doctorID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Invitation])
}

// Respond is the doctor accepting or declining an invitation.
func (s *Store) Respond(ctx context.Context, doctorID, hospitalID string, accept bool) error {
	if !accept {
		_, err := s.endAssociation(ctx, hospitalID, doctorID, []string{DoctorInvited}, DoctorDeclined)
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var doctorName, hospitalName string
	err = tx.QueryRow(ctx, `
		UPDATE hospital_doctors hd SET status = 'active', updated_at = now()
		FROM users u, hospitals h
		WHERE hd.hospital_id = $1 AND hd.doctor_id = $2 AND hd.status = 'invited'
		  AND u.id = hd.doctor_id AND h.id = hd.hospital_id
		RETURNING u.name, h.name`, hospitalID, doctorID).Scan(&doctorName, &hospitalName)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound("no pending invitation from this hospital")
	}
	if err != nil {
		return err
	}
	if err := s.notifyMembers(ctx, tx, hospitalID, notify.KindHospitalStaffChange, "Invitation accepted",
		doctorName+" joined "+hospitalName+". You can now set up their chamber and schedule.", false); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Leave is the doctor leaving a hospital.
func (s *Store) Leave(ctx context.Context, doctorID, hospitalID string) (int, error) {
	return s.endAssociation(ctx, hospitalID, doctorID, []string{DoctorActive}, DoctorLeft)
}

// ---------------------------------------------------------------- chambers

// CreateChamber opens a chamber at the hospital for a doctor on staff. Its name and
// address come from the hospital.
func (s *Store) CreateChamber(ctx context.Context, hospitalID, doctorID string, in ChamberInput) (doctor.Chamber, error) {
	if err := in.normalize(); err != nil {
		return doctor.Chamber{}, err
	}
	var id string
	err := s.db.QueryRow(ctx, `
		INSERT INTO chambers (doctor_id, hospital_id, name, address, district, area, phone, fee_paisa, room, booking_enabled)
		SELECT hd.doctor_id, h.id, h.name, h.address, h.district, h.area, h.phone, $3, $4, $5
		FROM hospitals h JOIN hospital_doctors hd ON hd.hospital_id = h.id
		WHERE h.id = $1 AND hd.doctor_id = $2 AND hd.status = 'active'
		RETURNING id`, hospitalID, doctorID, in.FeePaisa, nullIfEmpty(in.Room), *in.BookingEnabled).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return doctor.Chamber{}, apperr.Conflict("the doctor must accept the hospital's invitation first")
	}
	if err != nil {
		return doctor.Chamber{}, err
	}
	return s.doctors.ChamberWithSchedules(ctx, id)
}

// UpdateChamber changes the fee, room or booking switch of a hospital chamber.
func (s *Store) UpdateChamber(ctx context.Context, hospitalID, chamberID string, in ChamberInput) (doctor.Chamber, error) {
	if err := in.normalize(); err != nil {
		return doctor.Chamber{}, err
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE chambers SET fee_paisa = $3, room = $4, booking_enabled = $5, updated_at = now()
		WHERE id = $1 AND hospital_id = $2`, chamberID, hospitalID, in.FeePaisa, nullIfEmpty(in.Room), *in.BookingEnabled)
	if err != nil {
		return doctor.Chamber{}, err
	}
	if tag.RowsAffected() == 0 {
		return doctor.Chamber{}, apperr.NotFound("chamber not found")
	}
	return s.doctors.ChamberWithSchedules(ctx, chamberID)
}
