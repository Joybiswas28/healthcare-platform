package wallet

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database"
)

const (
	AcctPayoutsPending = "payouts_pending" // system: requested withdrawals not yet paid

	TxWithdrawalRequest  = "withdrawal_request"
	TxWithdrawalPaid     = "withdrawal_paid"
	TxWithdrawalRejected = "withdrawal_rejected"

	// MinWithdrawalPaisa is the smallest amount a doctor may withdraw (BDT 500).
	MinWithdrawalPaisa int64 = 500 * 100

	WithdrawalRequested = "requested"
	WithdrawalPaid      = "paid"
	WithdrawalRejected  = "rejected"
)

type Withdrawal struct {
	ID             string     `json:"id"`
	DoctorID       string     `json:"doctor_id"`
	DoctorName     string     `json:"doctor_name"`
	AmountPaisa    int64      `json:"amount_paisa"`
	Method         string     `json:"method"`
	AccountDetails string     `json:"account_details"`
	Status         string     `json:"status"`
	AdminNote      *string    `json:"admin_note"`
	PayoutRef      *string    `json:"payout_ref"`
	CreatedAt      time.Time  `json:"created_at"`
	ProcessedAt    *time.Time `json:"processed_at"`
}

type WithdrawalInput struct {
	AmountPaisa    int64  `json:"amount_paisa"`
	Method         string `json:"method"`          // bkash | nagad | bank
	AccountDetails string `json:"account_details"` // mobile number, or bank name/branch/account
}

type ProcessInput struct {
	Status    string `json:"status"`     // paid | rejected
	PayoutRef string `json:"payout_ref"` // required when paid, e.g. the bKash transaction id
	Note      string `json:"note"`       // required when rejected
}

func (in *WithdrawalInput) normalize() error {
	in.Method = strings.ToLower(strings.TrimSpace(in.Method))
	in.AccountDetails = strings.TrimSpace(in.AccountDetails)
	if in.AmountPaisa < MinWithdrawalPaisa {
		return apperr.Validation("the minimum withdrawal is %d paisa (BDT %d)", MinWithdrawalPaisa, MinWithdrawalPaisa/100)
	}
	switch in.Method {
	case "bkash", "nagad":
		phone, err := auth.NormalizePhone(in.AccountDetails)
		if err != nil {
			return apperr.Validation("account_details must be a valid %s mobile number", in.Method)
		}
		in.AccountDetails = phone
	case "bank":
		if n := utf8.RuneCountInString(in.AccountDetails); n < 10 || n > 300 {
			return apperr.Validation("for bank transfers, account_details must include bank, branch, account name and number (10-300 characters)")
		}
	default:
		return apperr.Validation("method must be bkash, nagad or bank")
	}
	return nil
}

const withdrawalSelect = `
SELECT w.id, w.doctor_id, u.name, w.amount_paisa, w.method, w.account_details, w.status,
       w.admin_note, w.payout_ref, w.created_at, w.processed_at
FROM withdrawals w JOIN users u ON u.id = w.doctor_id`

func scanWithdrawal(row pgx.Row) (Withdrawal, error) {
	var w Withdrawal
	err := row.Scan(&w.ID, &w.DoctorID, &w.DoctorName, &w.AmountPaisa, &w.Method, &w.AccountDetails,
		&w.Status, &w.AdminNote, &w.PayoutRef, &w.CreatedAt, &w.ProcessedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Withdrawal{}, apperr.NotFound("withdrawal not found")
	}
	return w, err
}

func (s *Store) getWithdrawal(ctx context.Context, q database.Querier, id string) (Withdrawal, error) {
	return scanWithdrawal(q.QueryRow(ctx, withdrawalSelect+` WHERE w.id = $1`, id))
}

// RequestWithdrawal reserves part of a doctor's earnings for payout.
// A doctor may have only one pending request at a time.
func (s *Store) RequestWithdrawal(ctx context.Context, doctorID string, in WithdrawalInput) (Withdrawal, error) {
	if err := in.normalize(); err != nil {
		return Withdrawal{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Withdrawal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO withdrawals (doctor_id, amount_paisa, method, account_details)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		doctorID, in.AmountPaisa, in.Method, in.AccountDetails).Scan(&id)
	if database.UniqueViolationOn(err, "withdrawals_one_pending") {
		return Withdrawal{}, apperr.Conflict("you already have a pending withdrawal request")
	}
	if err != nil {
		return Withdrawal{}, err
	}

	earnings, err := userAccount(ctx, tx, doctorID, AcctDoctorEarnings)
	if err != nil {
		return Withdrawal{}, err
	}
	pending, err := systemAccount(ctx, tx, AcctPayoutsPending)
	if err != nil {
		return Withdrawal{}, err
	}
	err = post(ctx, tx, TxWithdrawalRequest, &id, entry{earnings, -in.AmountPaisa}, entry{pending, in.AmountPaisa})
	if errors.Is(err, errInsufficientBalance) {
		return Withdrawal{}, apperr.Conflict("the amount is more than your available earnings")
	}
	if err != nil {
		return Withdrawal{}, err
	}
	w, err := s.getWithdrawal(ctx, tx, id)
	if err != nil {
		return Withdrawal{}, err
	}
	return w, tx.Commit(ctx)
}

// ListWithdrawals lists withdrawals for one doctor, or for everyone when doctorID is "".
func (s *Store) ListWithdrawals(ctx context.Context, doctorID, status string, limit, offset int) ([]Withdrawal, error) {
	rows, err := s.db.Query(ctx, withdrawalSelect+`
		WHERE ($1 = '' OR w.doctor_id::text = $1) AND ($2 = '' OR w.status = $2)
		ORDER BY w.created_at DESC, w.id
		LIMIT $3 OFFSET $4`, doctorID, status, limit, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Withdrawal, error) { return scanWithdrawal(r) })
}

// ProcessWithdrawal is the admin marking a request as paid (after sending the money
// outside the platform) or rejecting it (the money returns to the doctor's earnings).
func (s *Store) ProcessWithdrawal(ctx context.Context, id string, in ProcessInput) (Withdrawal, error) {
	in.PayoutRef = strings.TrimSpace(in.PayoutRef)
	in.Note = strings.TrimSpace(in.Note)
	switch in.Status {
	case WithdrawalPaid:
		if in.PayoutRef == "" {
			return Withdrawal{}, apperr.Validation("payout_ref is required when marking a withdrawal as paid")
		}
	case WithdrawalRejected:
		if in.Note == "" {
			return Withdrawal{}, apperr.Validation("note is required when rejecting a withdrawal")
		}
	default:
		return Withdrawal{}, apperr.Validation("status must be paid or rejected")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Withdrawal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	w, err := scanWithdrawal(tx.QueryRow(ctx, withdrawalSelect+` WHERE w.id = $1 FOR UPDATE OF w`, id))
	if err != nil {
		return Withdrawal{}, err
	}
	if w.Status != WithdrawalRequested {
		return Withdrawal{}, apperr.Conflict("this withdrawal is already %s", w.Status)
	}

	pending, err := systemAccount(ctx, tx, AcctPayoutsPending)
	if err != nil {
		return Withdrawal{}, err
	}
	var counter string
	var kind string
	if in.Status == WithdrawalPaid {
		kind = TxWithdrawalPaid
		counter, err = systemAccount(ctx, tx, AcctGateway)
	} else {
		kind = TxWithdrawalRejected
		counter, err = userAccount(ctx, tx, w.DoctorID, AcctDoctorEarnings)
	}
	if err != nil {
		return Withdrawal{}, err
	}
	if err := post(ctx, tx, kind, &w.ID, entry{pending, -w.AmountPaisa}, entry{counter, w.AmountPaisa}); err != nil {
		return Withdrawal{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE withdrawals SET status = $2, payout_ref = $3, admin_note = $4, processed_at = $5 WHERE id = $1`,
		id, in.Status, nullIfEmpty(in.PayoutRef), nullIfEmpty(in.Note), s.now()); err != nil {
		return Withdrawal{}, err
	}
	w, err = s.getWithdrawal(ctx, tx, id)
	if err != nil {
		return Withdrawal{}, err
	}
	return w, tx.Commit(ctx)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
