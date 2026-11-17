package pharmacy

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
	"healthcare-platform/internal/platform/storage"
)

type Store struct {
	db            *pgxpool.Pool
	files         storage.Storage
	commissionBps int // platform's share of the medicine total on each delivered order
	now           func() time.Time
}

func NewStore(db *pgxpool.Pool, files storage.Storage, commissionBps int) *Store {
	return &Store{db: db, files: files, commissionBps: commissionBps, now: time.Now}
}

var errPharmacyNotFound = apperr.NotFound("pharmacy not found")

// pharmacyColumns reads pharmacy p. $1/$2, when bound, are a point to measure distance from.
const pharmacyColumns = `
p.id, p.name, p.drug_license_no, p.address, p.district, p.area, p.phone,
p.latitude::float8, p.longitude::float8, p.opening_hours, p.offers_delivery, p.delivery_radius_km::float8,
p.accepting_orders, p.verification_status, p.verification_note,
(SELECT avg(rating)::float8 FROM pharmacy_reviews r WHERE r.pharmacy_id = p.id),
(SELECT count(*) FROM pharmacy_reviews r WHERE r.pharmacy_id = p.id),
p.created_at, p.verified_at`

func scanPharmacy(row pgx.Row, extra ...any) (Pharmacy, error) {
	var p Pharmacy
	dest := []any{&p.ID, &p.Name, &p.DrugLicenseNo, &p.Address, &p.District, &p.Area, &p.Phone,
		&p.Latitude, &p.Longitude, &p.OpeningHours, &p.OffersDelivery, &p.DeliveryRadiusKm,
		&p.AcceptingOrders, &p.VerificationStatus, &p.VerificationNote, &p.Rating, &p.ReviewCount,
		&p.CreatedAt, &p.VerifiedAt}
	err := row.Scan(append(dest, extra...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pharmacy{}, errPharmacyNotFound
	}
	return p, err
}

func collectPharmacies(rows pgx.Rows, err error, withDistance bool) ([]Pharmacy, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Pharmacy, error) {
		if !withDistance {
			return scanPharmacy(r)
		}
		var d float64
		p, err := scanPharmacy(r, &d)
		p.DistanceKm = &d
		return p, err
	})
}

// IsOwner reports whether userID owns the pharmacy (NotFound otherwise).
func (s *Store) IsOwner(ctx context.Context, userID, pharmacyID string) error {
	var ok bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pharmacies WHERE id = $1 AND owner_id = $2)`,
		pharmacyID, userID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return errPharmacyNotFound
	}
	return nil
}

func (s *Store) Create(ctx context.Context, ownerID string, in PharmacyInput) (Pharmacy, error) {
	if err := in.normalize(); err != nil {
		return Pharmacy{}, err
	}
	var id string
	err := s.db.QueryRow(ctx, `
		INSERT INTO pharmacies (owner_id, name, drug_license_no, address, district, area, phone, latitude, longitude,
		                        opening_hours, offers_delivery, delivery_radius_km, accepting_orders)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id`,
		ownerID, in.Name, in.DrugLicenseNo, in.Address, in.District, nullIfEmpty(in.Area), in.Phone,
		*in.Latitude, *in.Longitude, nullIfEmpty(in.OpeningHours), *in.OffersDelivery, in.DeliveryRadiusKm,
		*in.AcceptingOrders).Scan(&id)
	if database.IsUniqueViolation(err) {
		return Pharmacy{}, apperr.Conflict("a pharmacy with this drug licence number already exists")
	}
	if err != nil {
		return Pharmacy{}, err
	}
	return s.Get(ctx, id)
}

func (s *Store) Get(ctx context.Context, pharmacyID string) (Pharmacy, error) {
	return scanPharmacy(s.db.QueryRow(ctx, `SELECT `+pharmacyColumns+` FROM pharmacies p WHERE p.id = $1`, pharmacyID))
}

func (s *Store) Mine(ctx context.Context, ownerID string) ([]Pharmacy, error) {
	rows, err := s.db.Query(ctx, `SELECT `+pharmacyColumns+` FROM pharmacies p WHERE p.owner_id = $1 ORDER BY p.name`, ownerID)
	return collectPharmacies(rows, err, false)
}

// Update edits the profile. A verified pharmacy cannot change its licence number;
// a rejected one goes back to pending when it resubmits.
func (s *Store) Update(ctx context.Context, pharmacyID string, in PharmacyInput) (Pharmacy, error) {
	if err := in.normalize(); err != nil {
		return Pharmacy{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Pharmacy{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var licence, status string
	if err := tx.QueryRow(ctx, `SELECT drug_license_no, verification_status FROM pharmacies WHERE id = $1 FOR UPDATE`,
		pharmacyID).Scan(&licence, &status); err != nil {
		return Pharmacy{}, err
	}
	if status == StatusApproved && licence != in.DrugLicenseNo {
		return Pharmacy{}, apperr.Validation("a verified pharmacy cannot change its drug licence number; contact support")
	}
	if status == StatusRejected {
		status = StatusPending
	}
	_, err = tx.Exec(ctx, `
		UPDATE pharmacies SET name = $2, drug_license_no = $3, address = $4, district = $5, area = $6, phone = $7,
		       latitude = $8, longitude = $9, opening_hours = $10, offers_delivery = $11, delivery_radius_km = $12,
		       accepting_orders = $13, verification_status = $14, updated_at = now()
		WHERE id = $1`,
		pharmacyID, in.Name, in.DrugLicenseNo, in.Address, in.District, nullIfEmpty(in.Area), in.Phone,
		*in.Latitude, *in.Longitude, nullIfEmpty(in.OpeningHours), *in.OffersDelivery, in.DeliveryRadiusKm,
		*in.AcceptingOrders, status)
	if database.IsUniqueViolation(err) {
		return Pharmacy{}, apperr.Conflict("a pharmacy with this drug licence number already exists")
	}
	if err != nil {
		return Pharmacy{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Pharmacy{}, err
	}
	return s.Get(ctx, pharmacyID)
}

// Nearby lists verified pharmacies within maxKm of a point, nearest first.
func (s *Store) Nearby(ctx context.Context, lat, lng, maxKm float64, limit, offset int) ([]Pharmacy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT * FROM (
			SELECT `+pharmacyColumns+`, distance_km($1, $2, p.latitude::float8, p.longitude::float8) AS d
			FROM pharmacies p WHERE p.verification_status = 'approved'
		) x WHERE d <= $3 ORDER BY d LIMIT $4 OFFSET $5`, lat, lng, maxKm, limit, offset)
	return collectPharmacies(rows, err, true)
}

// Public returns a verified pharmacy with its latest reviews.
func (s *Store) Public(ctx context.Context, pharmacyID string) (Pharmacy, []Review, error) {
	p, err := scanPharmacy(s.db.QueryRow(ctx, `SELECT `+pharmacyColumns+` FROM pharmacies p
		WHERE p.id = $1 AND p.verification_status = 'approved'`, pharmacyID))
	if err != nil {
		return Pharmacy{}, nil, err
	}
	p.VerificationNote = nil
	rows, err := s.db.Query(ctx, `SELECT rating, comment, created_at FROM pharmacy_reviews
		WHERE pharmacy_id = $1 ORDER BY created_at DESC LIMIT 20`, pharmacyID)
	if err != nil {
		return Pharmacy{}, nil, err
	}
	reviews, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Review])
	return p, reviews, err
}

// ---------------------------------------------------------------- admin

func (s *Store) AdminList(ctx context.Context, status string, limit, offset int) ([]Pharmacy, error) {
	rows, err := s.db.Query(ctx, `SELECT `+pharmacyColumns+` FROM pharmacies p
		WHERE ($1 = '' OR p.verification_status = $1) ORDER BY p.created_at, p.id LIMIT $2 OFFSET $3`,
		status, limit, offset)
	return collectPharmacies(rows, err, false)
}

func (s *Store) Verify(ctx context.Context, pharmacyID string, in VerifyInput) (Pharmacy, error) {
	if err := in.normalize("pharmacy"); err != nil {
		return Pharmacy{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Pharmacy{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var owner, name string
	err = tx.QueryRow(ctx, `
		UPDATE pharmacies SET verification_status = $2, verification_note = $3,
		       verified_at = CASE WHEN $2 = 'approved' THEN now() END, updated_at = now()
		WHERE id = $1 RETURNING owner_id, name`, pharmacyID, in.Status, nullIfEmpty(in.Note)).Scan(&owner, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pharmacy{}, errPharmacyNotFound
	}
	if err != nil {
		return Pharmacy{}, err
	}
	msg := notify.Msg{UserID: owner, Kind: notify.KindPharmacyVerification, SMS: true,
		Title: "Pharmacy verified", Body: name + " is verified and will now receive medicine requests from nearby patients."}
	if in.Status == StatusRejected {
		msg.Title = "Pharmacy verification unsuccessful"
		msg.Body = name + " could not be verified: " + in.Note + ". Please update the details and resubmit."
	}
	if err := notify.Enqueue(ctx, tx, msg); err != nil {
		return Pharmacy{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Pharmacy{}, err
	}
	return s.Get(ctx, pharmacyID)
}
