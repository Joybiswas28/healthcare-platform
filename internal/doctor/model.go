// Package doctor manages doctor profiles, chambers, weekly schedules,
// public doctor search and admin verification.
package doctor

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"healthcare-platform/internal/platform/apperr"
)

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

type Profile struct {
	UserID             string  `json:"user_id"`
	Name               string  `json:"name"`
	BMDCRegNo          *string `json:"bmdc_reg_no"`
	Specialization     *string `json:"specialization"`
	Degrees            *string `json:"degrees"`
	ExperienceYears    *int    `json:"experience_years"`
	Bio                *string `json:"bio"`
	VerificationStatus string  `json:"verification_status"`
	VerificationNote   *string `json:"verification_note,omitempty"`

	ConsultationEnabled   bool `json:"consultation_enabled"`
	ConsultationFeeTokens int  `json:"consultation_fee_tokens"`
	CommissionBps         *int `json:"commission_bps,omitempty"` // shown to the doctor and admins only

	Rating      *float64 `json:"rating"` // average of patients' reviews; null before the first
	ReviewCount int      `json:"review_count"`
}

type ConsultationSettingsInput struct {
	Enabled   bool `json:"enabled"`
	FeeTokens int  `json:"fee_tokens"`
}

type CommissionInput struct {
	CommissionBps int `json:"commission_bps"`
}

type Chamber struct {
	ID             string  `json:"id"`
	DoctorID       string  `json:"doctor_id"`
	Name           string  `json:"name"`
	Address        string  `json:"address"`
	District       string  `json:"district"`
	Area           *string `json:"area"`
	Phone          *string `json:"phone"`
	FeePaisa       int64   `json:"fee_paisa"`
	BookingEnabled bool    `json:"booking_enabled"`
	// Set for chambers run by a hospital; those are managed by the hospital.
	HospitalID   *string    `json:"hospital_id"`
	HospitalName *string    `json:"hospital_name"`
	Room         *string    `json:"room"`
	Schedules    []Schedule `json:"schedules"`
}

// Schedule is a weekly recurring session. Weekday: 0 = Sunday ... 6 = Saturday.
type Schedule struct {
	ID          string `json:"id"`
	ChamberID   string `json:"chamber_id"`
	Weekday     int    `json:"weekday"`
	StartTime   string `json:"start_time"` // "HH:MM", Asia/Dhaka
	EndTime     string `json:"end_time"`
	MaxPatients int    `json:"max_patients"`
}

// Summary is a doctor as shown in search results.
type Summary struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Specialization  *string  `json:"specialization"`
	Degrees         *string  `json:"degrees"`
	ExperienceYears *int     `json:"experience_years"`
	Districts       []string `json:"districts"`
	MinFeePaisa     *int64   `json:"min_fee_paisa"`

	ConsultationEnabled   bool `json:"consultation_enabled"`
	ConsultationFeeTokens int  `json:"consultation_fee_tokens"`

	Rating      *float64 `json:"rating"`
	ReviewCount int      `json:"review_count"`
}

type PublicDoctor struct {
	Profile
	Chambers []Chamber `json:"chambers"`
	Reviews  []Review  `json:"reviews"` // the latest ones
}

type AdminDoctor struct {
	Profile
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
}

// ---- inputs ----

type ProfileInput struct {
	BMDCRegNo       string `json:"bmdc_reg_no"`
	Specialization  string `json:"specialization"`
	Degrees         string `json:"degrees"`
	ExperienceYears int    `json:"experience_years"`
	Bio             string `json:"bio"`
}

// BMDC numbers look like "12345", "A-12345" (MBBS) or "D-1234" (dental).
var bmdcPattern = regexp.MustCompile(`^[A-Z]{0,2}-?[0-9]{3,8}$`)

func (in *ProfileInput) normalize() error {
	in.BMDCRegNo = strings.ToUpper(strings.TrimSpace(in.BMDCRegNo))
	in.Specialization = strings.TrimSpace(in.Specialization)
	in.Degrees = strings.TrimSpace(in.Degrees)
	in.Bio = strings.TrimSpace(in.Bio)

	switch {
	case !bmdcPattern.MatchString(in.BMDCRegNo):
		return apperr.Validation("bmdc_reg_no must look like 12345 or A-12345")
	case !lenBetween(in.Specialization, 2, 100):
		return apperr.Validation("specialization must be 2-100 characters")
	case !lenBetween(in.Degrees, 2, 200):
		return apperr.Validation("degrees must be 2-200 characters")
	case in.ExperienceYears < 0 || in.ExperienceYears > 70:
		return apperr.Validation("experience_years must be between 0 and 70")
	case !lenBetween(in.Bio, 0, 2000):
		return apperr.Validation("bio must be at most 2000 characters")
	}
	return nil
}

type ChamberInput struct {
	Name           string `json:"name"`
	Address        string `json:"address"`
	District       string `json:"district"`
	Area           string `json:"area"`
	Phone          string `json:"phone"`
	FeePaisa       int64  `json:"fee_paisa"`
	BookingEnabled *bool  `json:"booking_enabled"`
}

const maxFeePaisa = 100_000 * 100 // BDT 100,000

func (in *ChamberInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.TrimSpace(in.Address)
	in.District = strings.TrimSpace(in.District)
	in.Area = strings.TrimSpace(in.Area)
	in.Phone = strings.TrimSpace(in.Phone)
	if in.BookingEnabled == nil {
		t := true
		in.BookingEnabled = &t
	}

	switch {
	case !lenBetween(in.Name, 2, 120):
		return apperr.Validation("name must be 2-120 characters")
	case !lenBetween(in.Address, 5, 300):
		return apperr.Validation("address must be 5-300 characters")
	case !lenBetween(in.District, 2, 60):
		return apperr.Validation("district must be 2-60 characters")
	case !lenBetween(in.Area, 0, 100):
		return apperr.Validation("area must be at most 100 characters")
	case !lenBetween(in.Phone, 0, 30):
		return apperr.Validation("phone must be at most 30 characters")
	case in.FeePaisa < 0 || in.FeePaisa > maxFeePaisa:
		return apperr.Validation("fee_paisa must be between 0 and %d", maxFeePaisa)
	}
	return nil
}

type ScheduleInput struct {
	Weekday     int    `json:"weekday"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	MaxPatients int    `json:"max_patients"`
}

func (in *ScheduleInput) normalize() error {
	if in.Weekday < 0 || in.Weekday > 6 {
		return apperr.Validation("weekday must be 0 (Sunday) to 6 (Saturday)")
	}
	start, err := time.Parse("15:04", strings.TrimSpace(in.StartTime))
	if err != nil {
		return apperr.Validation("start_time must be HH:MM (24-hour)")
	}
	end, err := time.Parse("15:04", strings.TrimSpace(in.EndTime))
	if err != nil {
		return apperr.Validation("end_time must be HH:MM (24-hour)")
	}
	if !end.After(start) {
		return apperr.Validation("end_time must be after start_time")
	}
	if in.MaxPatients < 1 || in.MaxPatients > 500 {
		return apperr.Validation("max_patients must be between 1 and 500")
	}
	in.StartTime, in.EndTime = start.Format("15:04"), end.Format("15:04")
	return nil
}

type VerifyInput struct {
	Status string `json:"status"` // "approved" | "rejected"
	Note   string `json:"note"`
}

// ---- helpers ----

func lenBetween(s string, lo, hi int) bool {
	n := utf8.RuneCountInString(s)
	return n >= lo && n <= hi
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// containsPattern builds an ILIKE pattern that matches s literally anywhere.
func containsPattern(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s) + "%"
}
