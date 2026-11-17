// Package hospital lets hospitals publish a profile, bring verified doctors on
// staff, run chambers for them on the central e-ticketing system, and manage the
// bookings made there.
package hospital

import (
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/platform/apperr"
)

var dhaka = time.FixedZone("Asia/Dhaka", 6*60*60)

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"

	RoleOwner = "owner"
	RoleStaff = "staff"

	// Doctor association states.
	DoctorInvited  = "invited"
	DoctorActive   = "active"
	DoctorDeclined = "declined"
	DoctorRemoved  = "removed"
	DoctorLeft     = "left"
)

type Hospital struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	RegistrationNo     string     `json:"registration_no"`
	Description        *string    `json:"description"`
	Address            string     `json:"address"`
	District           string     `json:"district"`
	Area               *string    `json:"area"`
	Phone              string     `json:"phone"`
	Email              *string    `json:"email"`
	Website            *string    `json:"website"`
	Departments        []string   `json:"departments"`
	Facilities         []string   `json:"facilities"`
	VisitingHours      *string    `json:"visiting_hours"`
	Latitude           *float64   `json:"latitude"`
	Longitude          *float64   `json:"longitude"`
	MapURL             *string    `json:"map_url"`
	LogoURL            *string    `json:"logo_url"`
	VerificationStatus string     `json:"verification_status"`
	VerificationNote   *string    `json:"verification_note,omitempty"`
	DoctorCount        int        `json:"doctor_count"`
	CreatedAt          time.Time  `json:"created_at"`
	VerifiedAt         *time.Time `json:"verified_at,omitempty"`

	MyRole string `json:"my_role,omitempty"` // for hospital staff viewing their own hospitals
}

type Photo struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// StaffDoctor is a doctor's place at a hospital.
type StaffDoctor struct {
	DoctorID        string           `json:"doctor_id"`
	Name            string           `json:"name"`
	Specialization  *string          `json:"specialization"`
	Degrees         *string          `json:"degrees"`
	ExperienceYears *int             `json:"experience_years"`
	BMDCRegNo       *string          `json:"bmdc_reg_no"`
	Department      *string          `json:"department"`
	Designation     *string          `json:"designation"`
	Status          string           `json:"status"`
	Chambers        []doctor.Chamber `json:"chambers,omitempty"`
}

// PublicHospital is the hospital page patients see.
type PublicHospital struct {
	Hospital
	Photos  []Photo       `json:"photos"`
	Doctors []StaffDoctor `json:"doctors"`
}

// Invitation is a hospital's invitation as seen by the doctor.
type Invitation struct {
	HospitalID   string    `json:"hospital_id"`
	HospitalName string    `json:"hospital_name"`
	District     string    `json:"district"`
	Department   *string   `json:"department"`
	Designation  *string   `json:"designation"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

type Member struct {
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// ---------------------------------------------------------------- inputs

type HospitalInput struct {
	Name           string   `json:"name"`
	RegistrationNo string   `json:"registration_no"`
	Description    string   `json:"description"`
	Address        string   `json:"address"`
	District       string   `json:"district"`
	Area           string   `json:"area"`
	Phone          string   `json:"phone"`
	Email          string   `json:"email"`
	Website        string   `json:"website"`
	Departments    []string `json:"departments"`
	Facilities     []string `json:"facilities"`
	VisitingHours  string   `json:"visiting_hours"`
	Latitude       *float64 `json:"latitude"`
	Longitude      *float64 `json:"longitude"`
}

func lenBetween(s string, lo, hi int) bool {
	n := utf8.RuneCountInString(s)
	return n >= lo && n <= hi
}

func cleanList(items []string, field string) ([]string, error) {
	if len(items) > 50 {
		return nil, apperr.Validation("%s: at most 50 items", field)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" || seen[strings.ToLower(it)] {
			continue
		}
		if !lenBetween(it, 2, 60) {
			return nil, apperr.Validation("%s: each item must be 2-60 characters", field)
		}
		seen[strings.ToLower(it)] = true
		out = append(out, it)
	}
	return out, nil
}

func (in *HospitalInput) normalize() error {
	t := strings.TrimSpace
	in.Name, in.RegistrationNo, in.Description = t(in.Name), strings.ToUpper(t(in.RegistrationNo)), t(in.Description)
	in.Address, in.District, in.Area, in.Phone = t(in.Address), t(in.District), t(in.Area), t(in.Phone)
	in.Email, in.Website, in.VisitingHours = t(in.Email), t(in.Website), t(in.VisitingHours)

	switch {
	case !lenBetween(in.Name, 3, 150):
		return apperr.Validation("name must be 3-150 characters")
	case !lenBetween(in.RegistrationNo, 3, 50):
		return apperr.Validation("registration_no (DGHS licence number) must be 3-50 characters")
	case !lenBetween(in.Description, 0, 3000):
		return apperr.Validation("description must be at most 3000 characters")
	case !lenBetween(in.Address, 5, 300):
		return apperr.Validation("address must be 5-300 characters")
	case !lenBetween(in.District, 2, 60):
		return apperr.Validation("district must be 2-60 characters")
	case !lenBetween(in.Area, 0, 100):
		return apperr.Validation("area must be at most 100 characters")
	case !lenBetween(in.Phone, 5, 30):
		return apperr.Validation("phone must be 5-30 characters")
	case !lenBetween(in.VisitingHours, 0, 300):
		return apperr.Validation("visiting_hours must be at most 300 characters")
	}
	if in.Email != "" {
		if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email {
			return apperr.Validation("email is not valid")
		}
	}
	if in.Website != "" {
		u, err := url.Parse(in.Website)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(in.Website) > 200 {
			return apperr.Validation("website must be an http(s) URL")
		}
	}
	if (in.Latitude == nil) != (in.Longitude == nil) {
		return apperr.Validation("give both latitude and longitude, or neither")
	}
	// Roughly the bounding box of Bangladesh.
	if in.Latitude != nil && (*in.Latitude < 20.3 || *in.Latitude > 26.8 || *in.Longitude < 87.9 || *in.Longitude > 92.8) {
		return apperr.Validation("latitude/longitude must be inside Bangladesh")
	}
	var err error
	if in.Departments, err = cleanList(in.Departments, "departments"); err != nil {
		return err
	}
	if in.Facilities, err = cleanList(in.Facilities, "facilities"); err != nil {
		return err
	}
	return nil
}

type InviteInput struct {
	BMDCRegNo   string `json:"bmdc_reg_no"` // the doctor to invite
	Department  string `json:"department"`
	Designation string `json:"designation"` // e.g. "Consultant", "Associate Professor"
}

type StaffInput struct {
	Department  string `json:"department"`
	Designation string `json:"designation"`
}

func (in *StaffInput) normalize() error {
	in.Department, in.Designation = strings.TrimSpace(in.Department), strings.TrimSpace(in.Designation)
	if !lenBetween(in.Department, 0, 60) || !lenBetween(in.Designation, 0, 80) {
		return apperr.Validation("department must be at most 60 and designation at most 80 characters")
	}
	return nil
}

type ChamberInput struct {
	FeePaisa       int64  `json:"fee_paisa"`
	Room           string `json:"room"` // e.g. "Room 305, 3rd floor"
	BookingEnabled *bool  `json:"booking_enabled"`
}

func (in *ChamberInput) normalize() error {
	in.Room = strings.TrimSpace(in.Room)
	if in.FeePaisa < 0 || in.FeePaisa > 100_000*100 {
		return apperr.Validation("fee_paisa must be between 0 and 10000000")
	}
	if !lenBetween(in.Room, 0, 100) {
		return apperr.Validation("room must be at most 100 characters")
	}
	if in.BookingEnabled == nil {
		t := true
		in.BookingEnabled = &t
	}
	return nil
}

type VerifyInput struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
