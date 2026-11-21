package pharmacy

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/family"
	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
	"healthcare-platform/internal/prescription"
)

// Upload is a prescription file sent with a request.
type Upload struct {
	ContentType string
	Body        io.Reader
}

// UploadTypes are the accepted prescription file types (detected from content).
var UploadTypes = map[string]bool{"application/pdf": true, "image/jpeg": true, "image/png": true}

// servesSQL is true when pharmacy p can serve request r: verified, taking orders,
// delivering if needed, and r is within its radius.
const servesSQL = `(p.verification_status = 'approved' AND p.accepting_orders
	AND (r.fulfilment = 'pickup' OR p.offers_delivery)
	AND distance_km(p.latitude::float8, p.longitude::float8, r.latitude::float8, r.longitude::float8) <= p.delivery_radius_km::float8)`

const requestColumns = `
r.id, r.prescription_id, r.attachment_key IS NOT NULL, r.items, r.note, r.fulfilment,
CASE WHEN r.status = 'open' AND r.expires_at <= now() THEN 'expired' ELSE r.status END,
(SELECT count(*) FROM pharmacy_quotes q WHERE q.request_id = r.id AND q.status IN ('pending', 'accepted')),
r.expires_at, r.created_at, r.address, r.contact_phone, (SELECT o.id FROM medicine_orders o WHERE o.request_id = r.id),
r.family_member_id, (SELECT fm.name FROM family_members fm WHERE fm.id = r.family_member_id),
(SELECT fm.gender FROM family_members fm WHERE fm.id = r.family_member_id),
(SELECT date_part('year', age(fm.birth_date))::int FROM family_members fm WHERE fm.id = r.family_member_id)`

func scanRequest(row pgx.Row, extra ...any) (Request, error) {
	var rq Request
	var items []byte
	var address *string
	var phone string
	dest := []any{&rq.ID, &rq.PrescriptionID, &rq.HasAttachment, &items, &rq.Note, &rq.Fulfilment, &rq.Status,
		&rq.QuoteCount, &rq.ExpiresAt, &rq.CreatedAt, &address, &phone, &rq.OrderID,
		&rq.FamilyMemberID, &rq.ForName, &rq.PatientGender, &rq.PatientAge}
	err := row.Scan(append(dest, extra...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, apperr.NotFound("request not found")
	}
	if err != nil {
		return Request{}, err
	}
	rq.Address, rq.ContactPhone = address, phone // callers strip these for pharmacies
	return rq, json.Unmarshal(items, &rq.Items)
}

func forPharmacy(rq Request) Request {
	rq.Address, rq.ContactPhone, rq.OrderID = nil, "", nil
	rq.FamilyMemberID, rq.ForName = nil, nil
	return rq
}

const quoteSelect = `
SELECT q.id, q.request_id, q.pharmacy_id, p.name, p.area,
       (SELECT avg(rating)::float8 FROM pharmacy_reviews rv WHERE rv.pharmacy_id = p.id),
       (SELECT count(*) FROM pharmacy_reviews rv WHERE rv.pharmacy_id = p.id),
       distance_km(p.latitude::float8, p.longitude::float8, r.latitude::float8, r.longitude::float8),
       q.items, q.medicines_total_paisa, q.delivery_charge_paisa, q.eta_minutes, q.note, q.status, q.created_at
FROM pharmacy_quotes q
JOIN pharmacies p        ON p.id = q.pharmacy_id
JOIN medicine_requests r ON r.id = q.request_id`

func scanQuote(row pgx.Row) (Quote, error) {
	var q Quote
	var items []byte
	err := row.Scan(&q.ID, &q.RequestID, &q.PharmacyID, &q.PharmacyName, &q.PharmacyArea, &q.PharmacyRating,
		&q.PharmacyReviews, &q.DistanceKm, &items, &q.MedicinesTotalPaisa, &q.DeliveryChargePaisa,
		&q.EtaMinutes, &q.Note, &q.Status, &q.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, apperr.NotFound("quote not found")
	}
	if err != nil {
		return Quote{}, err
	}
	if err := json.Unmarshal(items, &q.Items); err != nil {
		return Quote{}, err
	}
	for _, it := range q.Items {
		if it.Available {
			q.AvailableCount++
		}
	}
	q.TotalPaisa = q.MedicinesTotalPaisa + q.DeliveryChargePaisa
	return q, nil
}

func (s *Store) rxSummary(ctx context.Context, q database.Querier, id string) (*RxSummary, error) {
	var rx RxSummary
	var meds []byte
	err := q.QueryRow(ctx, `SELECT code, status, doctor_name, doctor_bmdc, issued_at, medicines FROM prescriptions WHERE id = $1`,
		id).Scan(&rx.Code, &rx.Status, &rx.DoctorName, &rx.DoctorBMDC, &rx.IssuedAt, &meds)
	if err != nil {
		return nil, err
	}
	return &rx, json.Unmarshal(meds, &rx.Medicines)
}

// ---------------------------------------------------------------- patient: requests

// CreateRequest posts a medicine request to the pharmacies around the patient.
// It fails if no verified pharmacy serves that location yet.
func (s *Store) CreateRequest(ctx context.Context, patientID string, in RequestInput, upload *Upload) (Request, error) {
	if err := in.normalize(upload != nil); err != nil {
		return Request{}, err
	}

	var rxID *string
	if in.PrescriptionID != "" {
		if _, err := uuid.Parse(in.PrescriptionID); err != nil {
			return Request{}, apperr.Validation("invalid prescription_id")
		}
		var status string
		var meds []byte
		err := s.db.QueryRow(ctx, `SELECT status, medicines FROM prescriptions WHERE id = $1 AND patient_id = $2`,
			in.PrescriptionID, patientID).Scan(&status, &meds)
		if errors.Is(err, pgx.ErrNoRows) {
			return Request{}, apperr.NotFound("prescription not found")
		}
		if err != nil {
			return Request{}, err
		}
		if status != prescription.StatusIssued {
			return Request{}, apperr.Conflict("this prescription has been revoked by the doctor")
		}
		if len(in.Items) == 0 {
			var ms []prescription.Medicine
			if err := json.Unmarshal(meds, &ms); err != nil {
				return Request{}, err
			}
			for _, m := range ms {
				in.Items = append(in.Items, Item{Name: m.Name, Strength: m.Strength, Quantity: m.Dosage + ", " + m.Duration})
			}
		}
		rxID = &in.PrescriptionID
	}
	in.FamilyMemberID = strings.TrimSpace(in.FamilyMemberID)
	if in.FamilyMemberID != "" {
		if _, err := uuid.Parse(in.FamilyMemberID); err != nil {
			return Request{}, apperr.Validation("invalid family_member_id")
		}
	}
	if err := family.Check(ctx, s.db, patientID, in.FamilyMemberID); err != nil {
		return Request{}, err
	}
	if upload != nil && !UploadTypes[upload.ContentType] {
		return Request{}, apperr.Validation("the prescription must be a PDF, JPEG or PNG file")
	}

	if in.ContactPhone == "" {
		if err := s.db.QueryRow(ctx, `SELECT phone FROM users WHERE id = $1`, patientID).Scan(&in.ContactPhone); err != nil {
			return Request{}, err
		}
	}

	var open int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM medicine_requests
		WHERE patient_id = $1 AND status = 'open' AND expires_at > now()`, patientID).Scan(&open); err != nil {
		return Request{}, err
	}
	if open >= MaxOpenRequests {
		return Request{}, apperr.Conflict("you can have at most %d open requests; cancel one first", MaxOpenRequests)
	}

	// Who will see it? Same rule as servesSQL, against the not-yet-saved request.
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT p.owner_id FROM pharmacies p
		WHERE p.verification_status = 'approved' AND p.accepting_orders AND ($3 = 'pickup' OR p.offers_delivery)
		  AND distance_km(p.latitude::float8, p.longitude::float8, $1, $2) <= p.delivery_radius_km::float8`,
		*in.Latitude, *in.Longitude, in.Fulfilment)
	if err != nil {
		return Request{}, err
	}
	owners, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return Request{}, err
	}
	if len(owners) == 0 {
		return Request{}, apperr.Conflict("no registered pharmacy serves your location yet")
	}

	var key, ctype *string
	if upload != nil {
		k := uuid.NewString()
		size, err := s.files.Put(ctx, k, upload.Body)
		if err != nil {
			return Request{}, err
		}
		if size == 0 {
			_ = s.files.Delete(ctx, k)
			return Request{}, apperr.Validation("the prescription file is empty")
		}
		key, ctype = &k, &upload.ContentType
	}

	id, err := s.insertRequest(ctx, patientID, in, rxID, key, ctype, owners)
	if err != nil {
		if key != nil {
			_ = s.files.Delete(ctx, *key)
		}
		return Request{}, err
	}
	return s.RequestForPatient(ctx, patientID, id)
}

func (s *Store) insertRequest(ctx context.Context, patientID string, in RequestInput, rxID, key, ctype *string, owners []string) (string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	items, _ := json.Marshal(in.Items)
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO medicine_requests (patient_id, prescription_id, attachment_key, attachment_type, items, note,
		                               fulfilment, address, latitude, longitude, contact_phone, expires_at, family_member_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id`,
		patientID, rxID, key, ctype, items, nullIfEmpty(in.Note), in.Fulfilment, nullIfEmpty(in.Address),
		*in.Latitude, *in.Longitude, in.ContactPhone, s.now().Add(RequestTTL), nullIfEmpty(in.FamilyMemberID)).Scan(&id); err != nil {
		return "", err
	}
	what := "pickup"
	if in.Fulfilment == FulfilDelivery {
		what = "home delivery"
	}
	for _, owner := range owners {
		if err := notify.Enqueue(ctx, tx, notify.Msg{UserID: owner, Kind: notify.KindMedicineRequest,
			Title: "New medicine request nearby",
			Body:  fmt.Sprintf("A patient nearby needs medicines (%s). Send a quote within 24 hours.", what)}); err != nil {
			return "", err
		}
	}
	return id, tx.Commit(ctx)
}

func (s *Store) MyRequests(ctx context.Context, patientID string, limit, offset int) ([]Request, error) {
	rows, err := s.db.Query(ctx, `SELECT `+requestColumns+` FROM medicine_requests r
		WHERE r.patient_id = $1 ORDER BY r.created_at DESC LIMIT $2 OFFSET $3`, patientID, limit, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Request, error) { return scanRequest(r) })
}

// RequestForPatient returns the patient's request with every quote, cheapest first.
func (s *Store) RequestForPatient(ctx context.Context, patientID, id string) (Request, error) {
	rq, err := scanRequest(s.db.QueryRow(ctx, `SELECT `+requestColumns+` FROM medicine_requests r
		WHERE r.id = $1 AND r.patient_id = $2`, id, patientID))
	if err != nil {
		return Request{}, err
	}
	if rq.PrescriptionID != nil {
		if rq.Prescription, err = s.rxSummary(ctx, s.db, *rq.PrescriptionID); err != nil {
			return Request{}, err
		}
	}
	rows, err := s.db.Query(ctx, quoteSelect+`
		WHERE q.request_id = $1 AND q.status <> 'withdrawn'
		ORDER BY (q.status = 'accepted') DESC, q.medicines_total_paisa + q.delivery_charge_paisa, q.created_at`, id)
	if err != nil {
		return Request{}, err
	}
	rq.Quotes, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Quote, error) { return scanQuote(r) })
	return rq, err
}

func (s *Store) CancelRequest(ctx context.Context, patientID, id string) (Request, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Request{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE medicine_requests SET status = 'cancelled'
		WHERE id = $1 AND patient_id = $2 AND status = 'open'`, id, patientID)
	if err != nil {
		return Request{}, err
	}
	if tag.RowsAffected() == 0 {
		rq, err := s.RequestForPatient(ctx, patientID, id)
		if err != nil {
			return Request{}, err
		}
		return Request{}, apperr.Conflict("this request is already %s", rq.Status)
	}
	if _, err := tx.Exec(ctx, `UPDATE pharmacy_quotes SET status = 'declined', updated_at = now()
		WHERE request_id = $1 AND status = 'pending'`, id); err != nil {
		return Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	return s.RequestForPatient(ctx, patientID, id)
}

// ---------------------------------------------------------------- pharmacy: feed and quotes

// Feed lists open requests this pharmacy can serve, nearest first, without any
// patient identity or exact address.
func (s *Store) Feed(ctx context.Context, pharmacyID string, limit, offset int) ([]Request, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+requestColumns+`,
		       distance_km(p.latitude::float8, p.longitude::float8, r.latitude::float8, r.longitude::float8) AS d
		FROM medicine_requests r JOIN pharmacies p ON p.id = $1
		WHERE r.status = 'open' AND r.expires_at > now() AND `+servesSQL+`
		ORDER BY d, r.created_at LIMIT $2 OFFSET $3`, pharmacyID, limit, offset)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Request, error) {
		var d float64
		rq, err := scanRequest(r, &d)
		rq.DistanceKm = &d
		return forPharmacy(rq), err
	})
	if err != nil {
		return nil, err
	}
	for i := range list {
		if q, err := scanQuote(s.db.QueryRow(ctx, quoteSelect+` WHERE q.request_id = $1 AND q.pharmacy_id = $2`,
			list[i].ID, pharmacyID)); err == nil {
			list[i].MyQuote = &q
		}
	}
	return list, nil
}

// RequestForPharmacy shows a request to a pharmacy that can serve it (while open)
// or has quoted on it.
func (s *Store) RequestForPharmacy(ctx context.Context, pharmacyID, id string) (Request, error) {
	var d float64
	rq, err := scanRequest(s.db.QueryRow(ctx, `
		SELECT `+requestColumns+`,
		       distance_km(p.latitude::float8, p.longitude::float8, r.latitude::float8, r.longitude::float8)
		FROM medicine_requests r JOIN pharmacies p ON p.id = $2
		WHERE r.id = $1 AND ((r.status = 'open' AND r.expires_at > now() AND `+servesSQL+`)
		      OR EXISTS (SELECT 1 FROM pharmacy_quotes q WHERE q.request_id = r.id AND q.pharmacy_id = p.id))`,
		id, pharmacyID), &d)
	if err != nil {
		return Request{}, err
	}
	rq.DistanceKm = &d
	rq = forPharmacy(rq)
	if rq.PrescriptionID != nil {
		if rq.Prescription, err = s.rxSummary(ctx, s.db, *rq.PrescriptionID); err != nil {
			return Request{}, err
		}
	}
	if q, err := scanQuote(s.db.QueryRow(ctx, quoteSelect+` WHERE q.request_id = $1 AND q.pharmacy_id = $2`, id, pharmacyID)); err == nil {
		rq.MyQuote = &q
	}
	return rq, nil
}

// OpenAttachment returns an uploaded prescription to its patient, or to a pharmacy
// allowed to see the request.
func (s *Store) OpenAttachment(ctx context.Context, patientID, pharmacyID, requestID string) (string, io.ReadCloser, error) {
	if pharmacyID != "" {
		if _, err := s.RequestForPharmacy(ctx, pharmacyID, requestID); err != nil {
			return "", nil, err
		}
	}
	var key, ctype *string
	err := s.db.QueryRow(ctx, `SELECT attachment_key, attachment_type FROM medicine_requests
		WHERE id = $1 AND ($2 = '' OR patient_id::text = $2)`, requestID, patientID).Scan(&key, &ctype)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && key == nil) {
		return "", nil, apperr.NotFound("no uploaded prescription for this request")
	}
	if err != nil {
		return "", nil, err
	}
	rc, err := s.files.Open(ctx, *key)
	return *ctype, rc, err
}

// SubmitQuote creates or revises this pharmacy's quote on an open request.
func (s *Store) SubmitQuote(ctx context.Context, pharmacyID, requestID string, in QuoteInput) (Quote, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Quote{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var fulfilment, patientID string
	var serves bool
	err = tx.QueryRow(ctx, `
		SELECT r.fulfilment, r.patient_id, `+servesSQL+`
		FROM medicine_requests r JOIN pharmacies p ON p.id = $2
		WHERE r.id = $1 AND r.status = 'open' AND r.expires_at > now()
		FOR SHARE OF r`, requestID, pharmacyID).Scan(&fulfilment, &patientID, &serves)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !serves) {
		return Quote{}, apperr.NotFound("this request is not open to your pharmacy")
	}
	if err != nil {
		return Quote{}, err
	}
	total, err := in.normalize(fulfilment)
	if err != nil {
		return Quote{}, err
	}

	items, _ := json.Marshal(in.Items)
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO pharmacy_quotes (request_id, pharmacy_id, items, medicines_total_paisa, delivery_charge_paisa, eta_minutes, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (request_id, pharmacy_id) DO UPDATE
		    SET items = EXCLUDED.items, medicines_total_paisa = EXCLUDED.medicines_total_paisa,
		        delivery_charge_paisa = EXCLUDED.delivery_charge_paisa, eta_minutes = EXCLUDED.eta_minutes,
		        note = EXCLUDED.note, status = 'pending', updated_at = now()
		    WHERE pharmacy_quotes.status IN ('pending', 'withdrawn')
		RETURNING id`,
		requestID, pharmacyID, items, total, in.DeliveryChargePaisa, in.EtaMinutes, nullIfEmpty(in.Note)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, apperr.Conflict("this quote can no longer be changed")
	}
	if err != nil {
		return Quote{}, err
	}
	q, err := scanQuote(tx.QueryRow(ctx, quoteSelect+` WHERE q.id = $1`, id))
	if err != nil {
		return Quote{}, err
	}
	if err := notify.Enqueue(ctx, tx, notify.Msg{UserID: patientID, Kind: notify.KindMedicineQuote,
		Title: "New quote for your medicines",
		Body: fmt.Sprintf("%s: %d of %d items available, total BDT %s, ready in about %d minutes.",
			q.PharmacyName, q.AvailableCount, len(q.Items), taka(q.TotalPaisa), q.EtaMinutes)}); err != nil {
		return Quote{}, err
	}
	return q, tx.Commit(ctx)
}

func (s *Store) WithdrawQuote(ctx context.Context, pharmacyID, quoteID string) (Quote, error) {
	tag, err := s.db.Exec(ctx, `UPDATE pharmacy_quotes SET status = 'withdrawn', updated_at = now()
		WHERE id = $1 AND pharmacy_id = $2 AND status = 'pending'`, quoteID, pharmacyID)
	if err != nil {
		return Quote{}, err
	}
	q, err := scanQuote(s.db.QueryRow(ctx, quoteSelect+` WHERE q.id = $1 AND q.pharmacy_id = $2`, quoteID, pharmacyID))
	if err != nil {
		return Quote{}, err
	}
	if tag.RowsAffected() == 0 {
		return Quote{}, apperr.Conflict("only pending quotes can be withdrawn (this one is %s)", q.Status)
	}
	return q, nil
}

// AcceptQuote turns the patient's chosen quote into an order. The request row is
// locked, so even simultaneous accepts produce exactly one order.
func (s *Store) AcceptQuote(ctx context.Context, patientID, quoteID string) (Order, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var requestID, pharmacyID, owner, reqStatus, quoteStatus, fulfilment, pharmacyStatus, pharmacyOwner, pharmacyName string
	var expired bool
	var items []byte
	var medicines, delivery int64
	err = tx.QueryRow(ctx, `
		SELECT r.id, r.patient_id, r.status, r.expires_at <= now(), r.fulfilment,
		       q.pharmacy_id, q.status, q.items, q.medicines_total_paisa, q.delivery_charge_paisa,
		       p.verification_status, p.owner_id, p.name
		FROM pharmacy_quotes q
		JOIN medicine_requests r ON r.id = q.request_id
		JOIN pharmacies p        ON p.id = q.pharmacy_id
		WHERE q.id = $1
		FOR UPDATE OF r, q`, quoteID).Scan(&requestID, &owner, &reqStatus, &expired, &fulfilment,
		&pharmacyID, &quoteStatus, &items, &medicines, &delivery, &pharmacyStatus, &pharmacyOwner, &pharmacyName)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner != patientID) {
		return Order{}, apperr.NotFound("quote not found")
	}
	if err != nil {
		return Order{}, err
	}
	switch {
	case reqStatus != RequestOpen:
		return Order{}, apperr.Conflict("this request is already %s", reqStatus)
	case expired:
		return Order{}, apperr.Conflict("this request has expired; post a new one")
	case quoteStatus != QuotePending:
		return Order{}, apperr.Conflict("this quote is %s", quoteStatus)
	case pharmacyStatus != StatusApproved:
		return Order{}, apperr.Conflict("this pharmacy can no longer take orders")
	}

	code, err := deliveryCode()
	if err != nil {
		return Order{}, err
	}
	var orderID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO medicine_orders (request_id, quote_id, patient_id, pharmacy_id, items, medicines_total_paisa,
		                             delivery_charge_paisa, total_paisa, commission_bps, fulfilment, delivery_code)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
		requestID, quoteID, patientID, pharmacyID, items, medicines, delivery, medicines+delivery,
		s.commissionBps, fulfilment, code).Scan(&orderID); err != nil {
		return Order{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE pharmacy_quotes SET status = CASE WHEN id = $2 THEN 'accepted' ELSE 'declined' END,
		updated_at = now() WHERE request_id = $1 AND status = 'pending'`, requestID, quoteID); err != nil {
		return Order{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE medicine_requests SET status = 'ordered' WHERE id = $1`, requestID); err != nil {
		return Order{}, err
	}
	if err := addEvent(ctx, tx, orderID, OrderConfirmed, "", patientID); err != nil {
		return Order{}, err
	}
	if err := notify.Enqueue(ctx, tx, notify.Msg{UserID: pharmacyOwner, Kind: notify.KindMedicineOrder,
		Title: "New order", SMS: true,
		Body: fmt.Sprintf("A patient accepted your quote (BDT %s, %s). Start preparing the order.",
			taka(medicines+delivery), fulfilment)}); err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	return s.OrderForPatient(ctx, patientID, orderID)
}

// deliveryCode is the 6-digit code the patient gives at handover.
func deliveryCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func taka(paisa int64) string {
	if paisa%100 == 0 {
		return fmt.Sprintf("%d", paisa/100)
	}
	return fmt.Sprintf("%d.%02d", paisa/100, paisa%100)
}
