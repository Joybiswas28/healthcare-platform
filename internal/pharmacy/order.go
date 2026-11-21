package pharmacy

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
)

const orderSelect = `
SELECT o.id, o.request_id, r.prescription_id, o.patient_id, pu.name, r.contact_phone, r.address,
       r.latitude::float8, r.longitude::float8, o.pharmacy_id, p.name, p.phone, p.address,
       o.items, o.medicines_total_paisa, o.delivery_charge_paisa, o.total_paisa, o.payment_method,
       o.fulfilment, o.delivery_mode, o.rider_id, ru.name, ru.phone,
       rd.latitude::float8, rd.longitude::float8, rd.location_at,
       o.status, o.cancelled_by, o.cancel_reason, o.commission_paisa, o.delivery_code,
       o.delivery_code_attempts, o.commission_bps,
       EXISTS (SELECT 1 FROM pharmacy_reviews rv WHERE rv.order_id = o.id), o.created_at, o.delivered_at,
       (SELECT fm.name FROM family_members fm WHERE fm.id = r.family_member_id),
       (SELECT fm.gender FROM family_members fm WHERE fm.id = r.family_member_id),
       (SELECT date_part('year', age(fm.birth_date))::int FROM family_members fm WHERE fm.id = r.family_member_id)
FROM medicine_orders o
JOIN medicine_requests r ON r.id = o.request_id
JOIN users pu            ON pu.id = o.patient_id
JOIN pharmacies p        ON p.id = o.pharmacy_id
LEFT JOIN users ru       ON ru.id = o.rider_id
LEFT JOIN riders rd      ON rd.user_id = o.rider_id`

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	var items []byte
	err := row.Scan(&o.ID, &o.RequestID, &o.PrescriptionID, &o.PatientID, &o.PatientName, &o.ContactPhone, &o.Address,
		&o.Latitude, &o.Longitude, &o.PharmacyID, &o.PharmacyName, &o.PharmacyPhone, &o.PharmacyAddress,
		&items, &o.MedicinesTotalPaisa, &o.DeliveryChargePaisa, &o.TotalPaisa, &o.PaymentMethod,
		&o.Fulfilment, &o.DeliveryMode, &o.RiderID, &o.RiderName, &o.RiderPhone,
		&o.riderLat, &o.riderLng, &o.riderAt,
		&o.Status, &o.CancelledBy, &o.CancelReason, &o.CommissionPaisa, &o.DeliveryCode,
		&o.codeAttempts, &o.commissionBps, &o.Reviewed, &o.CreatedAt, &o.DeliveredAt,
		&o.ForName, &o.PatientGender, &o.PatientAge)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, apperr.NotFound("order not found")
	}
	if err != nil {
		return Order{}, err
	}
	return o, json.Unmarshal(items, &o.Items)
}

type viewer int

const (
	asPatient viewer = iota
	asPharmacy
	asRider
)

// shape removes what a viewer must not see and adds live tracking.
func (s *Store) shape(o Order, v viewer) Order {
	if v != asPatient {
		o.DeliveryCode = "" // only the patient can confirm a handover
	}
	if v != asPharmacy {
		o.CommissionPaisa = nil
	}
	if v == asRider {
		// The rider hands the order to the account holder; who it is for is not theirs to know.
		o.ForName, o.PatientAge, o.PatientGender = nil, nil, nil
	}
	if (o.Status == OrderRiderAssigned || o.Status == OrderOutForDelivery) && o.riderAt != nil &&
		s.now().Sub(*o.riderAt) < RiderLocationFresh {
		o.RiderLocation = &Location{Latitude: *o.riderLat, Longitude: *o.riderLng, At: *o.riderAt}
	}
	return o
}

func (s *Store) withEvents(ctx context.Context, o Order) (Order, error) {
	rows, err := s.db.Query(ctx, `SELECT status, note, created_at FROM order_events WHERE order_id = $1 ORDER BY id`, o.ID)
	if err != nil {
		return Order{}, err
	}
	o.Events, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Event])
	return o, err
}

func (s *Store) one(ctx context.Context, v viewer, where string, args ...any) (Order, error) {
	o, err := scanOrder(s.db.QueryRow(ctx, orderSelect+` WHERE `+where, args...))
	if err != nil {
		return Order{}, err
	}
	return s.withEvents(ctx, s.shape(o, v))
}

func (s *Store) many(ctx context.Context, v viewer, tail string, args ...any) ([]Order, error) {
	rows, err := s.db.Query(ctx, orderSelect+` `+tail, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Order, error) {
		o, err := scanOrder(r)
		return s.shape(o, v), err
	})
}

func (s *Store) OrderForPatient(ctx context.Context, patientID, id string) (Order, error) {
	return s.one(ctx, asPatient, `o.id = $1 AND o.patient_id = $2`, id, patientID)
}

func (s *Store) OrderForPharmacy(ctx context.Context, pharmacyID, id string) (Order, error) {
	return s.one(ctx, asPharmacy, `o.id = $1 AND o.pharmacy_id = $2`, id, pharmacyID)
}

func (s *Store) PatientOrders(ctx context.Context, patientID string, limit, offset int) ([]Order, error) {
	return s.many(ctx, asPatient, `WHERE o.patient_id = $1 ORDER BY o.created_at DESC LIMIT $2 OFFSET $3`, patientID, limit, offset)
}

// PharmacyOrders lists orders; status "active" means not yet delivered or cancelled.
func (s *Store) PharmacyOrders(ctx context.Context, pharmacyID, status string, limit, offset int) ([]Order, error) {
	return s.many(ctx, asPharmacy, `
		WHERE o.pharmacy_id = $1 AND ($2 = '' OR o.status = $2 OR ($2 = 'active' AND o.status NOT IN ('delivered', 'cancelled')))
		ORDER BY o.created_at DESC LIMIT $3 OFFSET $4`, pharmacyID, status, limit, offset)
}

func addEvent(ctx context.Context, q database.Querier, orderID, status, note, actorID string) error {
	_, err := q.Exec(ctx, `INSERT INTO order_events (order_id, status, note, actor_id) VALUES ($1, $2, $3, $4)`,
		orderID, status, nullIfEmpty(note), actorID)
	return err
}

// lock loads an order for update inside tx.
func lock(ctx context.Context, tx pgx.Tx, id string) (Order, error) {
	return scanOrder(tx.QueryRow(ctx, orderSelect+` WHERE o.id = $1 FOR UPDATE OF o`, id))
}

func oneOf(status string, allowed ...string) bool {
	for _, a := range allowed {
		if status == a {
			return true
		}
	}
	return false
}

// step runs fn on a locked order and commits; fn returns the event note.
func (s *Store) step(ctx context.Context, id string, fn func(tx pgx.Tx, o Order) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	o, err := lock(ctx, tx, id)
	if err != nil {
		return err
	}
	if err := fn(tx, o); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func setStatus(ctx context.Context, tx pgx.Tx, o Order, status, note, actorID string, extra string, args ...any) error {
	sql := `UPDATE medicine_orders SET status = $2, updated_at = now()` + extra + ` WHERE id = $1`
	if _, err := tx.Exec(ctx, sql, append([]any{o.ID, status}, args...)...); err != nil {
		return err
	}
	return addEvent(ctx, tx, o.ID, status, note, actorID)
}

func tellPatient(ctx context.Context, tx pgx.Tx, o Order, title, body string, sms bool) error {
	return notify.Enqueue(ctx, tx, notify.Msg{UserID: o.PatientID, Kind: notify.KindMedicineOrder, Title: title, Body: body, SMS: sms})
}

func (s *Store) pharmacyOwner(ctx context.Context, q database.Querier, pharmacyID string) (string, error) {
	var id string
	err := q.QueryRow(ctx, `SELECT owner_id FROM pharmacies WHERE id = $1`, pharmacyID).Scan(&id)
	return id, err
}

// ---------------------------------------------------------------- pharmacy actions

// Advance moves an order to "preparing" or "ready".
func (s *Store) Advance(ctx context.Context, pharmacyID, actorID, id, to string) (Order, error) {
	err := s.step(ctx, id, func(tx pgx.Tx, o Order) error {
		if o.PharmacyID != pharmacyID {
			return apperr.NotFound("order not found")
		}
		switch to {
		case OrderPreparing:
			if o.Status != OrderConfirmed {
				return apperr.Conflict("only confirmed orders can start preparing (this one is %s)", o.Status)
			}
			return setStatus(ctx, tx, o, to, "", actorID, "")
		case OrderReady:
			if !oneOf(o.Status, OrderConfirmed, OrderPreparing) {
				return apperr.Conflict("the order cannot be marked ready (it is %s)", o.Status)
			}
			if err := setStatus(ctx, tx, o, to, "", actorID, ""); err != nil {
				return err
			}
			if o.Fulfilment == FulfilPickup {
				return tellPatient(ctx, tx, o, "Medicines ready for pickup",
					fmt.Sprintf("Your order at %s is ready. Pay BDT %s and show your delivery code at the counter.", o.PharmacyName, taka(o.TotalPaisa)), true)
			}
			return tellPatient(ctx, tx, o, "Order packed", o.PharmacyName+" has packed your medicines for delivery.", false)
		}
		return apperr.Validation("unknown step")
	})
	if err != nil {
		return Order{}, err
	}
	return s.OrderForPharmacy(ctx, pharmacyID, id)
}

// Dispatch sends a ready delivery order out with the pharmacy's own staff
// (mode "pharmacy") or offers it to platform riders (mode "rider").
func (s *Store) Dispatch(ctx context.Context, pharmacyID, actorID, id, mode string) (Order, error) {
	err := s.step(ctx, id, func(tx pgx.Tx, o Order) error {
		if o.PharmacyID != pharmacyID {
			return apperr.NotFound("order not found")
		}
		if o.Fulfilment != FulfilDelivery {
			return apperr.Conflict("pickup orders are not dispatched")
		}
		if o.Status != OrderReady || o.DeliveryMode != nil {
			return apperr.Conflict("only ready, undispatched orders can be dispatched (this one is %s)", o.Status)
		}
		switch mode {
		case ModePharmacy:
			if err := setStatus(ctx, tx, o, OrderOutForDelivery, "delivered by pharmacy staff", actorID, `, delivery_mode = 'pharmacy'`); err != nil {
				return err
			}
			return tellPatient(ctx, tx, o, "Medicines on the way",
				fmt.Sprintf("%s's delivery person is on the way. Pay BDT %s and give your delivery code on arrival.", o.PharmacyName, taka(o.TotalPaisa)), true)
		case ModeRider:
			if _, err := tx.Exec(ctx, `UPDATE medicine_orders SET delivery_mode = 'rider', updated_at = now() WHERE id = $1`, o.ID); err != nil {
				return err
			}
			if err := addEvent(ctx, tx, o.ID, OrderReady, "waiting for a rider", actorID); err != nil {
				return err
			}
			return s.alertRiders(ctx, tx, o)
		}
		return apperr.Validation("mode must be 'pharmacy' or 'rider'")
	})
	if err != nil {
		return Order{}, err
	}
	return s.OrderForPharmacy(ctx, pharmacyID, id)
}

// alertRiders tells available riders near the pharmacy about a new job.
func (s *Store) alertRiders(ctx context.Context, tx pgx.Tx, o Order) error {
	rows, err := tx.Query(ctx, `
		SELECT rd.user_id FROM riders rd, pharmacies p
		WHERE p.id = $1 AND rd.verification_status = 'approved' AND rd.available AND rd.location_at > $2
		  AND distance_km(rd.latitude::float8, rd.longitude::float8, p.latitude::float8, p.longitude::float8) <= $3`,
		o.PharmacyID, s.now().Add(-RiderLocationFresh), RiderJobRadiusKm)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := notify.Enqueue(ctx, tx, notify.Msg{UserID: id, Kind: notify.KindMedicineOrder,
			Title: "New delivery nearby", Body: fmt.Sprintf("Pickup at %s, earn BDT %s.", o.PharmacyName, taka(o.DeliveryChargePaisa))}); err != nil {
			return err
		}
	}
	return nil
}

// checkCode verifies the patient's delivery code. Wrong codes are counted (and the
// count is committed even though the handover fails); after MaxCodeAttempts the
// order can only be completed with support's help.
func (s *Store) checkCode(ctx context.Context, tx pgx.Tx, o Order, code string) (bool, error) {
	if o.codeAttempts >= MaxCodeAttempts {
		return false, apperr.Conflict("too many wrong codes for this order; contact support")
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(code)), []byte(o.DeliveryCode)) == 1 {
		return true, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE medicine_orders SET delivery_code_attempts = delivery_code_attempts + 1 WHERE id = $1`, o.ID); err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

var errWrongCode = apperr.Validation("the delivery code is incorrect")

func (s *Store) markDelivered(ctx context.Context, tx pgx.Tx, o Order, actorID string) error {
	commission := o.MedicinesTotalPaisa * int64(o.commissionBps) / 10000
	if err := setStatus(ctx, tx, o, OrderDelivered, "", actorID,
		`, delivered_at = $3, commission_paisa = $4`, s.now(), commission); err != nil {
		return err
	}
	return tellPatient(ctx, tx, o, "Order delivered",
		fmt.Sprintf("Your medicines from %s have been handed over. Please rate the pharmacy.", o.PharmacyName), false)
}

// Complete is the pharmacy handing over a pickup order at the counter, or its own
// delivery person handing over a delivery. Either way the patient's code is required.
func (s *Store) Complete(ctx context.Context, pharmacyID, actorID, id, code string) (Order, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	o, err := lock(ctx, tx, id)
	if err != nil {
		return Order{}, err
	}
	if o.PharmacyID != pharmacyID {
		return Order{}, apperr.NotFound("order not found")
	}
	pickup := o.Fulfilment == FulfilPickup && o.Status == OrderReady
	ownDelivery := o.DeliveryMode != nil && *o.DeliveryMode == ModePharmacy && o.Status == OrderOutForDelivery
	if !pickup && !ownDelivery {
		return Order{}, apperr.Conflict("the pharmacy can complete only ready pickup orders or its own deliveries (this one is %s)", o.Status)
	}
	ok, err := s.checkCode(ctx, tx, o, code)
	if err != nil {
		return Order{}, err
	}
	if !ok {
		return Order{}, errWrongCode
	}
	if err := s.markDelivered(ctx, tx, o, actorID); err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	return s.OrderForPharmacy(ctx, pharmacyID, id)
}

func cleanReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(reason); n < 5 || n > 300 {
		return "", apperr.Validation("reason must be 5-300 characters")
	}
	return reason, nil
}

// CancelByPharmacy cancels an order the pharmacy cannot fulfil, before it leaves the shop.
func (s *Store) CancelByPharmacy(ctx context.Context, pharmacyID, actorID, id, reason string) (Order, error) {
	reason, err := cleanReason(reason)
	if err != nil {
		return Order{}, err
	}
	err = s.step(ctx, id, func(tx pgx.Tx, o Order) error {
		if o.PharmacyID != pharmacyID {
			return apperr.NotFound("order not found")
		}
		if !oneOf(o.Status, OrderConfirmed, OrderPreparing, OrderReady) {
			return apperr.Conflict("the order can no longer be cancelled (it is %s)", o.Status)
		}
		if err := setStatus(ctx, tx, o, OrderCancelled, reason, actorID,
			`, cancelled_by = 'pharmacy', cancel_reason = $3`, reason); err != nil {
			return err
		}
		return tellPatient(ctx, tx, o, "Order cancelled",
			fmt.Sprintf("%s cancelled your order: %s. You can post a new request.", o.PharmacyName, reason), true)
	})
	if err != nil {
		return Order{}, err
	}
	return s.OrderForPharmacy(ctx, pharmacyID, id)
}

// ---------------------------------------------------------------- patient actions

// CancelByPatient cancels before the order is ready.
func (s *Store) CancelByPatient(ctx context.Context, patientID, id, reason string) (Order, error) {
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > 300 {
		return Order{}, apperr.Validation("reason must be at most 300 characters")
	}
	err := s.step(ctx, id, func(tx pgx.Tx, o Order) error {
		if o.PatientID != patientID {
			return apperr.NotFound("order not found")
		}
		if !oneOf(o.Status, OrderConfirmed, OrderPreparing) {
			return apperr.Conflict("the order can no longer be cancelled (it is %s); contact the pharmacy", o.Status)
		}
		if err := setStatus(ctx, tx, o, OrderCancelled, reason, patientID,
			`, cancelled_by = 'patient', cancel_reason = $3`, nullIfEmpty(reason)); err != nil {
			return err
		}
		owner, err := s.pharmacyOwner(ctx, tx, o.PharmacyID)
		if err != nil {
			return err
		}
		return notify.Enqueue(ctx, tx, notify.Msg{UserID: owner, Kind: notify.KindMedicineOrder, SMS: true,
			Title: "Order cancelled by patient", Body: "A patient cancelled their order. Please stop preparing it."})
	})
	if err != nil {
		return Order{}, err
	}
	return s.OrderForPatient(ctx, patientID, id)
}

type ReviewInput struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

// Review lets the patient rate the pharmacy once after delivery.
func (s *Store) Review(ctx context.Context, patientID, id string, in ReviewInput) error {
	in.Comment = strings.TrimSpace(in.Comment)
	if in.Rating < 1 || in.Rating > 5 {
		return apperr.Validation("rating must be 1-5")
	}
	if utf8.RuneCountInString(in.Comment) > 1000 {
		return apperr.Validation("comment must be at most 1000 characters")
	}
	tag, err := s.db.Exec(ctx, `
		INSERT INTO pharmacy_reviews (order_id, pharmacy_id, patient_id, rating, comment)
		SELECT o.id, o.pharmacy_id, o.patient_id, $3, $4 FROM medicine_orders o
		WHERE o.id = $1 AND o.patient_id = $2 AND o.status = 'delivered'
		ON CONFLICT (order_id) DO NOTHING`, id, patientID, in.Rating, nullIfEmpty(in.Comment))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		o, err := s.OrderForPatient(ctx, patientID, id)
		if err != nil {
			return err
		}
		if o.Reviewed {
			return apperr.Conflict("you have already reviewed this order")
		}
		return apperr.Conflict("you can review an order once it is delivered")
	}
	return nil
}

// ---------------------------------------------------------------- statement

type Statement struct {
	From                string `json:"from"`
	To                  string `json:"to"`
	DeliveredOrders     int    `json:"delivered_orders"`
	MedicinesTotalPaisa int64  `json:"medicines_total_paisa"`
	DeliveryTotalPaisa  int64  `json:"delivery_total_paisa"`
	CommissionDuePaisa  int64  `json:"commission_due_paisa"`
}

// Statement sums a pharmacy's delivered orders between two dates (inclusive,
// Bangladesh time) and the platform commission due on them.
func (s *Store) Statement(ctx context.Context, pharmacyID, from, to string) (Statement, error) {
	f, err1 := time.ParseInLocation(time.DateOnly, from, dhaka)
	t, err2 := time.ParseInLocation(time.DateOnly, to, dhaka)
	if err1 != nil || err2 != nil || t.Before(f) || t.Sub(f) > 366*24*time.Hour {
		return Statement{}, apperr.Validation("from and to must be dates (YYYY-MM-DD), at most a year apart")
	}
	st := Statement{From: from, To: to}
	err := s.db.QueryRow(ctx, `
		SELECT count(*), COALESCE(sum(medicines_total_paisa), 0)::bigint, COALESCE(sum(delivery_charge_paisa), 0)::bigint,
		       COALESCE(sum(commission_paisa), 0)::bigint
		FROM medicine_orders WHERE pharmacy_id = $1 AND status = 'delivered' AND delivered_at >= $2 AND delivered_at < $3`,
		pharmacyID, f, t.AddDate(0, 0, 1)).Scan(&st.DeliveredOrders, &st.MedicinesTotalPaisa, &st.DeliveryTotalPaisa, &st.CommissionDuePaisa)
	return st, err
}
