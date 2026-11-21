// Package pharmacy connects patients with nearby registered pharmacies and riders.
//
// Flow: a patient posts a medicine request (a platform prescription or an uploaded
// one) → nearby pharmacies quote → the patient accepts one quote, which becomes an
// order → the pharmacy prepares it → it is picked up by the patient, delivered by the
// pharmacy, or delivered by a platform rider → handover is confirmed with a code
// only the patient has.
//
// Pharmacies may mark a prescribed medicine unavailable but never substitute a
// different one. Payment is cash on delivery for now.
package pharmacy

import (
	"strings"
	"time"
	"unicode/utf8"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/prescription"
)

var dhaka = time.FixedZone("Asia/Dhaka", 6*60*60)

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"

	FulfilDelivery = "delivery"
	FulfilPickup   = "pickup"

	RequestOpen      = "open"
	RequestOrdered   = "ordered"
	RequestCancelled = "cancelled"
	RequestExpired   = "expired" // shown for open requests past expires_at

	QuotePending   = "pending"
	QuoteAccepted  = "accepted"
	QuoteDeclined  = "declined"
	QuoteWithdrawn = "withdrawn"

	OrderConfirmed      = "confirmed"
	OrderPreparing      = "preparing"
	OrderReady          = "ready"
	OrderRiderAssigned  = "rider_assigned"
	OrderOutForDelivery = "out_for_delivery"
	OrderDelivered      = "delivered"
	OrderCancelled      = "cancelled"

	ModePharmacy = "pharmacy"
	ModeRider    = "rider"

	RequestTTL         = 24 * time.Hour
	MaxOpenRequests    = 3
	RiderJobRadiusKm   = 5.0              // riders see jobs at pharmacies this close
	RiderLocationFresh = 30 * time.Minute // older rider locations are ignored
	MaxCodeAttempts    = 5
)

type Pharmacy struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	DrugLicenseNo      string     `json:"drug_license_no"`
	Address            string     `json:"address"`
	District           string     `json:"district"`
	Area               *string    `json:"area"`
	Phone              string     `json:"phone"`
	Latitude           float64    `json:"latitude"`
	Longitude          float64    `json:"longitude"`
	OpeningHours       *string    `json:"opening_hours"`
	OffersDelivery     bool       `json:"offers_delivery"`
	DeliveryRadiusKm   float64    `json:"delivery_radius_km"`
	AcceptingOrders    bool       `json:"accepting_orders"`
	VerificationStatus string     `json:"verification_status"`
	VerificationNote   *string    `json:"verification_note,omitempty"`
	Rating             *float64   `json:"rating"`
	ReviewCount        int        `json:"review_count"`
	DistanceKm         *float64   `json:"distance_km,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	VerifiedAt         *time.Time `json:"verified_at,omitempty"`
}

type Review struct {
	Rating    int       `json:"rating"`
	Comment   *string   `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

// Item is a medicine the patient asks for.
type Item struct {
	Name     string `json:"name"`
	Strength string `json:"strength"`
	Quantity string `json:"quantity"` // e.g. "10 tablets", "1 bottle", or the dosage and duration
}

// QuotedItem is a pharmacy's answer for one item.
type QuotedItem struct {
	Name       string `json:"name"`
	Strength   string `json:"strength"`
	Quantity   string `json:"quantity"`
	Available  bool   `json:"available"`
	PricePaisa int64  `json:"price_paisa"` // for the whole quantity; 0 when unavailable
}

// RxSummary is what a pharmacy sees of a platform prescription.
type RxSummary struct {
	Code       string                  `json:"code"`
	Status     string                  `json:"status"`
	DoctorName string                  `json:"doctor_name"`
	DoctorBMDC string                  `json:"doctor_bmdc"`
	IssuedAt   time.Time               `json:"issued_at"`
	Medicines  []prescription.Medicine `json:"medicines"`
}

type Request struct {
	ID             string     `json:"id"`
	PrescriptionID *string    `json:"prescription_id,omitempty"`
	Prescription   *RxSummary `json:"prescription,omitempty"`
	HasAttachment  bool       `json:"has_attachment"`
	Items          []Item     `json:"items"`
	Note           *string    `json:"note"`
	Fulfilment     string     `json:"fulfilment"`
	Status         string     `json:"status"`
	QuoteCount     int        `json:"quote_count"`
	ExpiresAt      time.Time  `json:"expires_at"`
	CreatedAt      time.Time  `json:"created_at"`

	// For a family member: their age and gender help the pharmacy check doses.
	PatientAge    *int    `json:"patient_age,omitempty"`
	PatientGender *string `json:"patient_gender,omitempty"`

	// Patient view only.
	FamilyMemberID *string `json:"family_member_id,omitempty"`
	ForName        *string `json:"for_name,omitempty"` // the family member it is for
	Address        *string `json:"address,omitempty"`
	ContactPhone   string  `json:"contact_phone,omitempty"`
	Quotes         []Quote `json:"quotes,omitempty"`
	OrderID        *string `json:"order_id,omitempty"`
	// Pharmacy view only: how far the patient is, never where exactly.
	DistanceKm *float64 `json:"distance_km,omitempty"`
	MyQuote    *Quote   `json:"my_quote,omitempty"`
}

type Quote struct {
	ID                  string       `json:"id"`
	RequestID           string       `json:"request_id"`
	PharmacyID          string       `json:"pharmacy_id"`
	PharmacyName        string       `json:"pharmacy_name"`
	PharmacyArea        *string      `json:"pharmacy_area"`
	PharmacyRating      *float64     `json:"pharmacy_rating"`
	PharmacyReviews     int          `json:"pharmacy_review_count"`
	DistanceKm          float64      `json:"distance_km"`
	Items               []QuotedItem `json:"items"`
	AvailableCount      int          `json:"available_count"`
	MedicinesTotalPaisa int64        `json:"medicines_total_paisa"`
	DeliveryChargePaisa int64        `json:"delivery_charge_paisa"`
	TotalPaisa          int64        `json:"total_paisa"`
	EtaMinutes          int          `json:"eta_minutes"`
	Note                *string      `json:"note"`
	Status              string       `json:"status"`
	CreatedAt           time.Time    `json:"created_at"`
}

type Location struct {
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	At        time.Time `json:"at"`
}

type Event struct {
	Status    string    `json:"status"`
	Note      *string   `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

type Order struct {
	ID                  string       `json:"id"`
	RequestID           string       `json:"request_id"`
	PrescriptionID      *string      `json:"prescription_id"`
	PatientID           string       `json:"patient_id"`
	PatientName         string       `json:"patient_name"`       // the account holder, who receives the order
	ForName             *string      `json:"for_name,omitempty"` // the family member the medicines are for
	PatientAge          *int         `json:"patient_age,omitempty"`
	PatientGender       *string      `json:"patient_gender,omitempty"`
	ContactPhone        string       `json:"contact_phone"`
	Address             *string      `json:"address"`
	Latitude            float64      `json:"latitude"`
	Longitude           float64      `json:"longitude"`
	PharmacyID          string       `json:"pharmacy_id"`
	PharmacyName        string       `json:"pharmacy_name"`
	PharmacyPhone       string       `json:"pharmacy_phone"`
	PharmacyAddress     string       `json:"pharmacy_address"`
	Items               []QuotedItem `json:"items"`
	MedicinesTotalPaisa int64        `json:"medicines_total_paisa"`
	DeliveryChargePaisa int64        `json:"delivery_charge_paisa"`
	TotalPaisa          int64        `json:"total_paisa"`
	PaymentMethod       string       `json:"payment_method"`
	Fulfilment          string       `json:"fulfilment"`
	DeliveryMode        *string      `json:"delivery_mode"`
	RiderID             *string      `json:"rider_id"`
	RiderName           *string      `json:"rider_name"`
	RiderPhone          *string      `json:"rider_phone"`
	RiderLocation       *Location    `json:"rider_location,omitempty"`
	Status              string       `json:"status"`
	CancelledBy         *string      `json:"cancelled_by"`
	CancelReason        *string      `json:"cancel_reason"`
	CommissionPaisa     *int64       `json:"commission_paisa,omitempty"` // pharmacy view
	DeliveryCode        string       `json:"delivery_code,omitempty"`    // patient view only
	Reviewed            bool         `json:"reviewed"`
	Events              []Event      `json:"events,omitempty"`
	CreatedAt           time.Time    `json:"created_at"`
	DeliveredAt         *time.Time   `json:"delivered_at"`

	commissionBps int
	codeAttempts  int
	riderLat      *float64
	riderLng      *float64
	riderAt       *time.Time
}

type Rider struct {
	UserID             string    `json:"user_id"`
	Name               string    `json:"name"`
	Phone              string    `json:"phone"`
	NIDNumber          string    `json:"nid_number"`
	Vehicle            string    `json:"vehicle"`
	VerificationStatus string    `json:"verification_status"`
	VerificationNote   *string   `json:"verification_note,omitempty"`
	Available          bool      `json:"available"`
	Location           *Location `json:"location"`
	CreatedAt          time.Time `json:"created_at"`
}

// Job is a delivery waiting for a rider. The patient's details appear only after
// a rider takes the job.
type Job struct {
	OrderID            string  `json:"order_id"`
	PharmacyName       string  `json:"pharmacy_name"`
	PharmacyAddress    string  `json:"pharmacy_address"`
	PharmacyLatitude   float64 `json:"pharmacy_latitude"`
	PharmacyLongitude  float64 `json:"pharmacy_longitude"`
	DistanceToPharmacy float64 `json:"distance_to_pharmacy_km"`
	DeliveryDistanceKm float64 `json:"delivery_distance_km"`
	EarningPaisa       int64   `json:"earning_paisa"`
	CashToCollectPaisa int64   `json:"cash_to_collect_paisa"`
}

// ---------------------------------------------------------------- inputs

type PharmacyInput struct {
	Name             string   `json:"name"`
	DrugLicenseNo    string   `json:"drug_license_no"`
	Address          string   `json:"address"`
	District         string   `json:"district"`
	Area             string   `json:"area"`
	Phone            string   `json:"phone"`
	Latitude         *float64 `json:"latitude"`
	Longitude        *float64 `json:"longitude"`
	OpeningHours     string   `json:"opening_hours"`
	OffersDelivery   *bool    `json:"offers_delivery"`
	DeliveryRadiusKm float64  `json:"delivery_radius_km"`
	AcceptingOrders  *bool    `json:"accepting_orders"`
}

func lenBetween(s string, lo, hi int) bool {
	n := utf8.RuneCountInString(s)
	return n >= lo && n <= hi
}

func checkLocation(lat, lng *float64) error {
	if lat == nil || lng == nil {
		return apperr.Validation("latitude and longitude are required")
	}
	if *lat < 20.3 || *lat > 26.8 || *lng < 87.9 || *lng > 92.8 {
		return apperr.Validation("latitude/longitude must be inside Bangladesh")
	}
	return nil
}

func boolOr(b *bool, def bool) *bool {
	if b == nil {
		return &def
	}
	return b
}

func (in *PharmacyInput) normalize() error {
	t := strings.TrimSpace
	in.Name, in.DrugLicenseNo, in.Address = t(in.Name), strings.ToUpper(t(in.DrugLicenseNo)), t(in.Address)
	in.District, in.Area, in.Phone, in.OpeningHours = t(in.District), t(in.Area), t(in.Phone), t(in.OpeningHours)
	in.OffersDelivery, in.AcceptingOrders = boolOr(in.OffersDelivery, true), boolOr(in.AcceptingOrders, true)
	if in.DeliveryRadiusKm == 0 {
		in.DeliveryRadiusKm = 5
	}
	switch {
	case !lenBetween(in.Name, 3, 120):
		return apperr.Validation("name must be 3-120 characters")
	case !lenBetween(in.DrugLicenseNo, 3, 50):
		return apperr.Validation("drug_license_no (DGDA licence) must be 3-50 characters")
	case !lenBetween(in.Address, 5, 300):
		return apperr.Validation("address must be 5-300 characters")
	case !lenBetween(in.District, 2, 60):
		return apperr.Validation("district must be 2-60 characters")
	case !lenBetween(in.Area, 0, 100):
		return apperr.Validation("area must be at most 100 characters")
	case !lenBetween(in.Phone, 5, 30):
		return apperr.Validation("phone must be 5-30 characters")
	case !lenBetween(in.OpeningHours, 0, 200):
		return apperr.Validation("opening_hours must be at most 200 characters")
	case in.DeliveryRadiusKm < 0.5 || in.DeliveryRadiusKm > 20:
		return apperr.Validation("delivery_radius_km must be between 0.5 and 20")
	}
	return checkLocation(in.Latitude, in.Longitude)
}

type RequestInput struct {
	PrescriptionID string   `json:"prescription_id"` // a prescription issued on the platform, or
	Items          []Item   `json:"items"`           // medicines to ask for (needed with an uploaded prescription)
	Note           string   `json:"note"`
	Fulfilment     string   `json:"fulfilment"` // delivery | pickup
	Address        string   `json:"address"`    // required for delivery
	Latitude       *float64 `json:"latitude"`
	Longitude      *float64 `json:"longitude"`
	ContactPhone   string   `json:"contact_phone"`    // defaults to the account's phone
	FamilyMemberID string   `json:"family_member_id"` // empty: for the account holder
}

func cleanItems(items []Item) ([]Item, error) {
	if len(items) > 50 {
		return nil, apperr.Validation("at most 50 items")
	}
	out := make([]Item, 0, len(items))
	for i, it := range items {
		it.Name, it.Strength, it.Quantity = strings.TrimSpace(it.Name), strings.TrimSpace(it.Strength), strings.TrimSpace(it.Quantity)
		if !lenBetween(it.Name, 2, 100) || !lenBetween(it.Strength, 0, 50) || !lenBetween(it.Quantity, 0, 100) {
			return nil, apperr.Validation("item %d: name 2-100, strength up to 50 and quantity up to 100 characters", i+1)
		}
		out = append(out, it)
	}
	return out, nil
}

func (in *RequestInput) normalize(hasUpload bool) error {
	in.PrescriptionID, in.Note, in.Address = strings.TrimSpace(in.PrescriptionID), strings.TrimSpace(in.Note), strings.TrimSpace(in.Address)
	in.Fulfilment = strings.ToLower(strings.TrimSpace(in.Fulfilment))
	if (in.PrescriptionID == "") == !hasUpload {
		return apperr.Validation("give either prescription_id (a platform prescription) or upload a prescription file, not both")
	}
	var err error
	if in.Items, err = cleanItems(in.Items); err != nil {
		return err
	}
	if hasUpload && len(in.Items) == 0 {
		in.Items = []Item{} // the pharmacy reads the uploaded prescription
	}
	switch in.Fulfilment {
	case FulfilDelivery:
		if !lenBetween(in.Address, 5, 300) {
			return apperr.Validation("address (5-300 characters) is required for delivery")
		}
	case FulfilPickup:
		in.Address = ""
	default:
		return apperr.Validation("fulfilment must be delivery or pickup")
	}
	if !lenBetween(in.Note, 0, 500) {
		return apperr.Validation("note must be at most 500 characters")
	}
	if in.ContactPhone != "" {
		p, err := auth.NormalizePhone(in.ContactPhone)
		if err != nil {
			return apperr.Validation("contact_phone must be a Bangladeshi mobile number")
		}
		in.ContactPhone = p
	}
	return checkLocation(in.Latitude, in.Longitude)
}

type QuoteInput struct {
	Items               []QuotedItem `json:"items"`
	DeliveryChargePaisa int64        `json:"delivery_charge_paisa"`
	EtaMinutes          int          `json:"eta_minutes"`
	Note                string       `json:"note"`
}

const maxPricePaisa = 1_000_000 * 100 // BDT 10 lakh

func (in *QuoteInput) normalize(fulfilment string) (total int64, err error) {
	in.Note = strings.TrimSpace(in.Note)
	if len(in.Items) == 0 || len(in.Items) > 50 {
		return 0, apperr.Validation("a quote needs 1-50 items")
	}
	available := 0
	for i := range in.Items {
		it := &in.Items[i]
		it.Name, it.Strength, it.Quantity = strings.TrimSpace(it.Name), strings.TrimSpace(it.Strength), strings.TrimSpace(it.Quantity)
		switch {
		case !lenBetween(it.Name, 2, 100) || !lenBetween(it.Strength, 0, 50) || !lenBetween(it.Quantity, 0, 100):
			return 0, apperr.Validation("item %d: name 2-100, strength up to 50 and quantity up to 100 characters", i+1)
		case !it.Available && it.PricePaisa != 0:
			return 0, apperr.Validation("item %d: an unavailable item must have price 0", i+1)
		case it.Available && (it.PricePaisa <= 0 || it.PricePaisa > maxPricePaisa):
			return 0, apperr.Validation("item %d: price_paisa must be positive", i+1)
		}
		if it.Available {
			available++
			total += it.PricePaisa
		}
	}
	if available == 0 {
		return 0, apperr.Validation("at least one item must be available")
	}
	if fulfilment == FulfilPickup && in.DeliveryChargePaisa != 0 {
		return 0, apperr.Validation("pickup orders have no delivery charge")
	}
	if in.DeliveryChargePaisa < 0 || in.DeliveryChargePaisa > 100_000 {
		return 0, apperr.Validation("delivery_charge_paisa must be between 0 and 100000")
	}
	if in.EtaMinutes < 5 || in.EtaMinutes > 1440 {
		return 0, apperr.Validation("eta_minutes must be between 5 and 1440")
	}
	if !lenBetween(in.Note, 0, 500) {
		return 0, apperr.Validation("note must be at most 500 characters")
	}
	return total, nil
}

type RiderInput struct {
	NIDNumber string `json:"nid_number"`
	Vehicle   string `json:"vehicle"` // bicycle | motorcycle | car | on_foot
}

type AvailabilityInput struct {
	Available bool     `json:"available"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

type VerifyInput struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (in *VerifyInput) normalize(what string) error {
	in.Note = strings.TrimSpace(in.Note)
	switch in.Status {
	case StatusApproved:
		return nil
	case StatusRejected:
		if in.Note == "" {
			return apperr.Validation("note is required when rejecting a %s", what)
		}
		return nil
	}
	return apperr.Validation("status must be 'approved' or 'rejected'")
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
