package pharmacy

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
)

// Bangladeshi NIDs are 10 or 17 digits (older 13-digit numbers also exist).
var nidPattern = regexp.MustCompile(`^(\d{10}|\d{13}|\d{17})$`)

var vehicles = map[string]bool{"bicycle": true, "motorcycle": true, "car": true, "on_foot": true}

var errNoRiderProfile = apperr.NotFound("rider profile not found; create one first")

const riderSelect = `
SELECT rd.user_id, u.name, u.phone, rd.nid_number, rd.vehicle, rd.verification_status, rd.verification_note,
       rd.available, rd.latitude::float8, rd.longitude::float8, rd.location_at, rd.created_at
FROM riders rd JOIN users u ON u.id = rd.user_id`

func scanRider(row pgx.Row) (Rider, error) {
	var r Rider
	var lat, lng *float64
	var locAt *time.Time
	err := row.Scan(&r.UserID, &r.Name, &r.Phone, &r.NIDNumber, &r.Vehicle, &r.VerificationStatus, &r.VerificationNote,
		&r.Available, &lat, &lng, &locAt, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rider{}, errNoRiderProfile
	}
	if err != nil {
		return Rider{}, err
	}
	if lat != nil && locAt != nil {
		r.Location = &Location{Latitude: *lat, Longitude: *lng, At: *locAt}
	}
	return r, nil
}

func (s *Store) Rider(ctx context.Context, userID string) (Rider, error) {
	return scanRider(s.db.QueryRow(ctx, riderSelect+` WHERE rd.user_id = $1`, userID))
}

// RegisterRider creates (or, while not yet verified, updates) a rider profile.
func (s *Store) RegisterRider(ctx context.Context, userID string, in RiderInput) (Rider, error) {
	in.NIDNumber = strings.ReplaceAll(strings.TrimSpace(in.NIDNumber), " ", "")
	in.Vehicle = strings.ToLower(strings.TrimSpace(in.Vehicle))
	if !nidPattern.MatchString(in.NIDNumber) {
		return Rider{}, apperr.Validation("nid_number must be a 10, 13 or 17 digit NID number")
	}
	if !vehicles[in.Vehicle] {
		return Rider{}, apperr.Validation("vehicle must be bicycle, motorcycle, car or on_foot")
	}
	tag, err := s.db.Exec(ctx, `
		INSERT INTO riders (user_id, nid_number, vehicle) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE
		    SET nid_number = EXCLUDED.nid_number, vehicle = EXCLUDED.vehicle,
		        verification_status = 'pending', updated_at = now()
		    WHERE riders.verification_status <> 'approved'`, userID, in.NIDNumber, in.Vehicle)
	if database.IsUniqueViolation(err) {
		return Rider{}, apperr.Conflict("this NID is already registered to another rider")
	}
	if err != nil {
		return Rider{}, err
	}
	if tag.RowsAffected() == 0 {
		return Rider{}, apperr.Conflict("a verified rider profile cannot be changed; contact support")
	}
	return s.Rider(ctx, userID)
}

// SetAvailability turns a verified rider on or off duty. Going on duty needs a location.
func (s *Store) SetAvailability(ctx context.Context, userID string, in AvailabilityInput) (Rider, error) {
	r, err := s.Rider(ctx, userID)
	if err != nil {
		return Rider{}, err
	}
	if in.Available && r.VerificationStatus != StatusApproved {
		return Rider{}, apperr.Forbidden("your rider profile must be verified before you go on duty")
	}
	if in.Latitude != nil || in.Available {
		if err := checkLocation(in.Latitude, in.Longitude); err != nil {
			return Rider{}, err
		}
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE riders SET available = $2,
		       latitude = COALESCE($3, latitude), longitude = COALESCE($4, longitude),
		       location_at = CASE WHEN $3::numeric IS NULL THEN location_at ELSE $5 END, updated_at = now()
		WHERE user_id = $1`, userID, in.Available, in.Latitude, in.Longitude, s.now()); err != nil {
		return Rider{}, err
	}
	return s.Rider(ctx, userID)
}

// UpdateLocation records where the rider is (sent periodically by the rider app).
func (s *Store) UpdateLocation(ctx context.Context, userID string, lat, lng *float64) error {
	if err := checkLocation(lat, lng); err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `UPDATE riders SET latitude = $2, longitude = $3, location_at = $4 WHERE user_id = $1`,
		userID, *lat, *lng, s.now())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNoRiderProfile
	}
	return nil
}

// Jobs lists deliveries waiting for a rider at pharmacies near the rider.
func (s *Store) Jobs(ctx context.Context, userID string) ([]Job, error) {
	r, err := s.Rider(ctx, userID)
	if err != nil {
		return nil, err
	}
	if r.VerificationStatus != StatusApproved || !r.Available || r.Location == nil ||
		s.now().Sub(r.Location.At) > RiderLocationFresh {
		return nil, apperr.Conflict("go on duty with a current location to see jobs")
	}
	rows, err := s.db.Query(ctx, `
		SELECT * FROM (
			SELECT o.id, p.name, p.address, p.latitude::float8, p.longitude::float8,
			       distance_km($1, $2, p.latitude::float8, p.longitude::float8) AS to_pharmacy,
			       distance_km(p.latitude::float8, p.longitude::float8, rq.latitude::float8, rq.longitude::float8),
			       o.delivery_charge_paisa, o.total_paisa
			FROM medicine_orders o
			JOIN pharmacies p         ON p.id = o.pharmacy_id
			JOIN medicine_requests rq ON rq.id = o.request_id
			WHERE o.status = 'ready' AND o.delivery_mode = 'rider' AND o.rider_id IS NULL
		) j WHERE to_pharmacy <= $3 ORDER BY to_pharmacy LIMIT 50`,
		r.Location.Latitude, r.Location.Longitude, RiderJobRadiusKm)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Job])
}

// AcceptJob assigns a waiting delivery to the rider. The order row is locked, so
// only one rider can win it; a rider carries one order at a time.
func (s *Store) AcceptJob(ctx context.Context, userID, orderID string) (Order, error) {
	r, err := s.Rider(ctx, userID)
	if err != nil {
		return Order{}, err
	}
	if r.VerificationStatus != StatusApproved || !r.Available {
		return Order{}, apperr.Forbidden("go on duty to accept jobs")
	}
	err = s.step(ctx, orderID, func(tx pgx.Tx, o Order) error {
		if o.Status != OrderReady || o.DeliveryMode == nil || *o.DeliveryMode != ModeRider || o.RiderID != nil {
			return apperr.Conflict("this job is no longer available")
		}
		err := setStatus(ctx, tx, o, OrderRiderAssigned, r.Name, userID, `, rider_id = $3`, userID)
		if database.UniqueViolationOn(err, "medicine_orders_one_job_per_rider") {
			return apperr.Conflict("finish your current delivery first")
		}
		if err != nil {
			return err
		}
		owner, err := s.pharmacyOwner(ctx, tx, o.PharmacyID)
		if err != nil {
			return err
		}
		if err := notify.Enqueue(ctx, tx, notify.Msg{UserID: owner, Kind: notify.KindMedicineOrder,
			Title: "Rider assigned", Body: fmt.Sprintf("%s (%s) is coming to collect an order.", r.Name, r.Phone)}); err != nil {
			return err
		}
		return tellPatient(ctx, tx, o, "Rider assigned", r.Name+" will pick up your medicines and bring them to you.", false)
	})
	if err != nil {
		return Order{}, err
	}
	return s.OrderForRider(ctx, userID, orderID)
}

func (s *Store) OrderForRider(ctx context.Context, riderID, id string) (Order, error) {
	return s.one(ctx, asRider, `o.id = $1 AND o.rider_id = $2`, id, riderID)
}

// RiderOrders lists the rider's deliveries, current first.
func (s *Store) RiderOrders(ctx context.Context, riderID string, limit, offset int) ([]Order, error) {
	return s.many(ctx, asRider, `WHERE o.rider_id = $1
		ORDER BY (o.status IN ('rider_assigned', 'out_for_delivery')) DESC, o.created_at DESC
		LIMIT $2 OFFSET $3`, riderID, limit, offset)
}

// ReleaseJob gives a job back before pickup (e.g. the rider can't make it).
func (s *Store) ReleaseJob(ctx context.Context, riderID, orderID string) error {
	return s.step(ctx, orderID, func(tx pgx.Tx, o Order) error {
		if o.RiderID == nil || *o.RiderID != riderID {
			return apperr.NotFound("order not found")
		}
		if o.Status != OrderRiderAssigned {
			return apperr.Conflict("only jobs not yet picked up can be released")
		}
		if err := setStatus(ctx, tx, o, OrderReady, "rider released the job", riderID, `, rider_id = NULL`); err != nil {
			return err
		}
		return s.alertRiders(ctx, tx, o)
	})
}

// PickedUp is the rider collecting the order from the pharmacy.
func (s *Store) PickedUp(ctx context.Context, riderID, orderID string) (Order, error) {
	err := s.step(ctx, orderID, func(tx pgx.Tx, o Order) error {
		if o.RiderID == nil || *o.RiderID != riderID {
			return apperr.NotFound("order not found")
		}
		if o.Status != OrderRiderAssigned {
			return apperr.Conflict("the order is %s", o.Status)
		}
		if err := setStatus(ctx, tx, o, OrderOutForDelivery, "", riderID, ""); err != nil {
			return err
		}
		return tellPatient(ctx, tx, o, "Medicines on the way",
			fmt.Sprintf("%s is on the way. Pay BDT %s and give your delivery code on arrival.", *o.RiderName, taka(o.TotalPaisa)), true)
	})
	if err != nil {
		return Order{}, err
	}
	return s.OrderForRider(ctx, riderID, orderID)
}

// Deliver is the rider handing over the order; the patient's code is required.
func (s *Store) Deliver(ctx context.Context, riderID, orderID, code string) (Order, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	o, err := lock(ctx, tx, orderID)
	if err != nil {
		return Order{}, err
	}
	if o.RiderID == nil || *o.RiderID != riderID {
		return Order{}, apperr.NotFound("order not found")
	}
	if o.Status != OrderOutForDelivery {
		return Order{}, apperr.Conflict("pick up the order before delivering it (it is %s)", o.Status)
	}
	ok, err := s.checkCode(ctx, tx, o, code)
	if err != nil {
		return Order{}, err
	}
	if !ok {
		return Order{}, errWrongCode
	}
	if err := s.markDelivered(ctx, tx, o, riderID); err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	return s.OrderForRider(ctx, riderID, orderID)
}

type RiderEarnings struct {
	Deliveries   int   `json:"deliveries"`
	EarningPaisa int64 `json:"earning_paisa"`
}

// Earnings totals the delivery charges of the rider's completed deliveries.
func (s *Store) Earnings(ctx context.Context, riderID string) (RiderEarnings, error) {
	var e RiderEarnings
	err := s.db.QueryRow(ctx, `SELECT count(*), COALESCE(sum(delivery_charge_paisa), 0)::bigint
		FROM medicine_orders WHERE rider_id = $1 AND status = 'delivered'`, riderID).Scan(&e.Deliveries, &e.EarningPaisa)
	return e, err
}

// ---------------------------------------------------------------- admin

func (s *Store) AdminRiders(ctx context.Context, status string, limit, offset int) ([]Rider, error) {
	rows, err := s.db.Query(ctx, riderSelect+` WHERE ($1 = '' OR rd.verification_status = $1)
		ORDER BY rd.created_at LIMIT $2 OFFSET $3`, status, limit, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Rider, error) { return scanRider(r) })
}

func (s *Store) VerifyRider(ctx context.Context, userID string, in VerifyInput) (Rider, error) {
	if err := in.normalize("rider"); err != nil {
		return Rider{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Rider{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE riders SET verification_status = $2, verification_note = $3,
		available = available AND $2 = 'approved', updated_at = now() WHERE user_id = $1`,
		userID, in.Status, nullIfEmpty(in.Note))
	if err != nil {
		return Rider{}, err
	}
	if tag.RowsAffected() == 0 {
		return Rider{}, errNoRiderProfile
	}
	msg := notify.Msg{UserID: userID, Kind: notify.KindRiderVerification, SMS: true,
		Title: "Rider profile verified", Body: "Your rider profile is verified. Go on duty to start receiving deliveries."}
	if in.Status == StatusRejected {
		msg.Title = "Rider verification unsuccessful"
		msg.Body = "Your rider profile could not be verified: " + in.Note
	}
	if err := notify.Enqueue(ctx, tx, msg); err != nil {
		return Rider{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Rider{}, err
	}
	return s.Rider(ctx, userID)
}
