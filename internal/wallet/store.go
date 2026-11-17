package wallet

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/platform/apperr"
)

type Package struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	PricePaisa   int64  `json:"price_paisa"`
	Tokens       int    `json:"tokens"`
	ValidityDays int    `json:"validity_days"`
}

type Payment struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	PackageID    string     `json:"package_id"`
	AmountPaisa  int64      `json:"amount_paisa"`
	Tokens       int        `json:"tokens"`
	ValidityDays int        `json:"validity_days"`
	Provider     string     `json:"provider"`
	Status       string     `json:"status"`
	CheckoutURL  string     `json:"checkout_url,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	PaidAt       *time.Time `json:"paid_at"`
}

type Lot struct {
	ID              string    `json:"id"`
	TokensTotal     int       `json:"tokens_total"`
	TokensRemaining int       `json:"tokens_remaining"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type Wallet struct {
	TokensAvailable int   `json:"tokens_available"`
	TokenValuePaisa int64 `json:"token_value_paisa"`
	Lots            []Lot `json:"lots"`
}

type Entry struct {
	Kind        string    `json:"kind"`
	ReferenceID *string   `json:"reference_id"`
	AmountPaisa int64     `json:"amount_paisa"`
	CreatedAt   time.Time `json:"created_at"`
}

type Earnings struct {
	BalancePaisa           int64   `json:"balance_paisa"`            // available to withdraw
	PendingWithdrawalPaisa int64   `json:"pending_withdrawal_paisa"` // requested, not yet paid
	Entries                []Entry `json:"entries"`
}

type Store struct {
	db  *pgxpool.Pool
	now func() time.Time
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db, now: time.Now} }

// ---------------------------------------------------------------- packages & payments

func (s *Store) Packages(ctx context.Context) ([]Package, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, price_paisa, tokens, validity_days FROM token_packages
		WHERE active ORDER BY price_paisa`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Package])
}

const paymentColumns = `id, user_id, package_id, amount_paisa, tokens, validity_days, provider, status, created_at, paid_at`

func scanPayment(row pgx.Row) (Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.UserID, &p.PackageID, &p.AmountPaisa, &p.Tokens, &p.ValidityDays,
		&p.Provider, &p.Status, &p.CreatedAt, &p.PaidAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, apperr.NotFound("payment not found")
	}
	return p, err
}

// StartPurchase creates a pending payment for a token package. Tokens are only
// credited once the payment provider confirms it (CompletePayment).
func (s *Store) StartPurchase(ctx context.Context, userID, packageID, provider string) (Payment, error) {
	p, err := scanPayment(s.db.QueryRow(ctx, `
		INSERT INTO payments (user_id, package_id, amount_paisa, tokens, validity_days, provider)
		SELECT $1, id, price_paisa, tokens, validity_days, $3 FROM token_packages
		WHERE id = $2 AND active
		RETURNING `+paymentColumns, userID, packageID, provider))
	if apperr.Is(err, apperr.KindNotFound) {
		return Payment{}, apperr.NotFound("token package not found")
	}
	return p, err
}

func (s *Store) GetPayment(ctx context.Context, userID, paymentID string) (Payment, error) {
	return scanPayment(s.db.QueryRow(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE id = $1 AND user_id = $2`, paymentID, userID))
}

// CompletePayment marks a payment as paid and credits the tokens. It is idempotent:
// payment providers often deliver the same success callback more than once, and only
// the first one credits tokens.
func (s *Store) CompletePayment(ctx context.Context, paymentID, providerRef string) (Payment, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Payment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	p, err := scanPayment(tx.QueryRow(ctx, `
		UPDATE payments SET status = 'paid', paid_at = $3, provider_ref = $2
		WHERE id = $1 AND status = 'pending'
		RETURNING `+paymentColumns, paymentID, providerRef, s.now()))
	if apperr.Is(err, apperr.KindNotFound) {
		existing, gerr := scanPayment(s.db.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id = $1`, paymentID))
		if gerr != nil {
			return Payment{}, gerr
		}
		if existing.Status == "paid" {
			return existing, nil // duplicate callback
		}
		return Payment{}, apperr.Conflict("payment is %s", existing.Status)
	}
	if err != nil {
		return Payment{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO token_lots (patient_id, payment_id, tokens_total, tokens_remaining, expires_at)
		VALUES ($1, $2, $3, $3, $4)`,
		p.UserID, p.ID, p.Tokens, p.PaidAt.AddDate(0, 0, p.ValidityDays)); err != nil {
		return Payment{}, err
	}

	gateway, err := systemAccount(ctx, tx, AcctGateway)
	if err != nil {
		return Payment{}, err
	}
	revenue, err := systemAccount(ctx, tx, AcctPlatformRevenue)
	if err != nil {
		return Payment{}, err
	}
	patient, err := userAccount(ctx, tx, p.UserID, AcctPatientTokens)
	if err != nil {
		return Payment{}, err
	}
	// Tokens are credited at face value. If a package is sold at a discount (or a
	// premium), the difference is booked against platform revenue.
	faceValue := int64(p.Tokens) * TokenValuePaisa
	if err := post(ctx, tx, TxTokenPurchase, &p.ID,
		entry{gateway, -p.AmountPaisa},
		entry{patient, faceValue},
		entry{revenue, p.AmountPaisa - faceValue},
	); err != nil {
		return Payment{}, err
	}
	return p, tx.Commit(ctx)
}

// ---------------------------------------------------------------- wallet views

func (s *Store) Wallet(ctx context.Context, patientID string) (Wallet, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, tokens_total, tokens_remaining, expires_at FROM token_lots
		WHERE patient_id = $1 AND tokens_remaining > 0 AND expires_at > $2
		ORDER BY expires_at, id`, patientID, s.now())
	if err != nil {
		return Wallet{}, err
	}
	lots, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Lot])
	if err != nil {
		return Wallet{}, err
	}
	w := Wallet{TokenValuePaisa: TokenValuePaisa, Lots: lots}
	for _, l := range lots {
		w.TokensAvailable += l.TokensRemaining
	}
	return w, nil
}

// History lists ledger entries of one of the user's accounts, newest first.
func (s *Store) History(ctx context.Context, ownerID, accountKind string, limit, offset int) ([]Entry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.kind, t.reference_id, e.amount_paisa, e.created_at
		FROM ledger_entries e
		JOIN ledger_accounts a     ON a.id = e.account_id
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE a.owner_id = $1 AND a.kind = $2
		ORDER BY e.id DESC
		LIMIT $3 OFFSET $4`, ownerID, accountKind, limit, offset)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Entry])
}

func (s *Store) Earnings(ctx context.Context, doctorID string, limit, offset int) (Earnings, error) {
	var e Earnings
	err := s.db.QueryRow(ctx, `
		SELECT
		  COALESCE((SELECT balance_paisa FROM ledger_accounts WHERE owner_id = $1 AND kind = $2), 0),
		  COALESCE((SELECT sum(amount_paisa) FROM withdrawals WHERE doctor_id = $1 AND status = 'requested'), 0)::bigint`,
		doctorID, AcctDoctorEarnings).Scan(&e.BalancePaisa, &e.PendingWithdrawalPaisa)
	if err != nil {
		return Earnings{}, err
	}
	e.Entries, err = s.History(ctx, doctorID, AcctDoctorEarnings, limit, offset)
	return e, err
}

// ---------------------------------------------------------------- consultation money flow
// These run inside the caller's transaction so the consultation status change and
// the money movement commit or roll back together.

// Hold moves tokens from the patient's wallet into escrow, spending the lots that
// expire first.
func Hold(ctx context.Context, tx pgx.Tx, now time.Time, patientID, consultationID string, tokens int) error {
	rows, err := tx.Query(ctx, `
		SELECT id, tokens_remaining FROM token_lots
		WHERE patient_id = $1 AND tokens_remaining > 0 AND expires_at > $2
		ORDER BY expires_at, id
		FOR UPDATE`, patientID, now)
	if err != nil {
		return err
	}
	type lot struct {
		id        string
		remaining int
	}
	lots, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (lot, error) {
		var l lot
		return l, r.Scan(&l.id, &l.remaining)
	})
	if err != nil {
		return err
	}

	available := 0
	for _, l := range lots {
		available += l.remaining
	}
	if available < tokens {
		return apperr.Conflict("not enough tokens: this consultation needs %d, you have %d", tokens, available)
	}

	need := tokens
	for _, l := range lots {
		if need == 0 {
			break
		}
		take := min(need, l.remaining)
		need -= take
		if _, err := tx.Exec(ctx,
			`UPDATE token_lots SET tokens_remaining = tokens_remaining - $2 WHERE id = $1`, l.id, take); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO consultation_token_usage (consultation_id, lot_id, tokens) VALUES ($1, $2, $3)`,
			consultationID, l.id, take); err != nil {
			return err
		}
	}

	patient, err := userAccount(ctx, tx, patientID, AcctPatientTokens)
	if err != nil {
		return err
	}
	escrow, err := systemAccount(ctx, tx, AcctEscrow)
	if err != nil {
		return err
	}
	amount := int64(tokens) * TokenValuePaisa
	return post(ctx, tx, TxConsultationHold, &consultationID, entry{patient, -amount}, entry{escrow, amount})
}

// Split returns how a consultation fee is shared between doctor and platform.
func Split(tokens, commissionBps int) (doctorPaisa, platformPaisa int64) {
	fee := int64(tokens) * TokenValuePaisa
	platformPaisa = fee * int64(commissionBps) / 10000
	return fee - platformPaisa, platformPaisa
}

// Release pays out a completed consultation from escrow to the doctor and platform.
func Release(ctx context.Context, tx pgx.Tx, consultationID, doctorID string, tokens, commissionBps int) error {
	escrow, err := systemAccount(ctx, tx, AcctEscrow)
	if err != nil {
		return err
	}
	revenue, err := systemAccount(ctx, tx, AcctPlatformRevenue)
	if err != nil {
		return err
	}
	doctor, err := userAccount(ctx, tx, doctorID, AcctDoctorEarnings)
	if err != nil {
		return err
	}
	doctorShare, platformShare := Split(tokens, commissionBps)
	return post(ctx, tx, TxConsultationRelease, &consultationID,
		entry{escrow, -(doctorShare + platformShare)},
		entry{doctor, doctorShare},
		entry{revenue, platformShare},
	)
}

// Refund returns held tokens to the exact lots they came from.
func Refund(ctx context.Context, tx pgx.Tx, consultationID, patientID string) error {
	rows, err := tx.Query(ctx, `
		DELETE FROM consultation_token_usage WHERE consultation_id = $1
		RETURNING lot_id, tokens`, consultationID)
	if err != nil {
		return err
	}
	type usage struct {
		lot    string
		tokens int
	}
	usages, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (usage, error) {
		var u usage
		return u, r.Scan(&u.lot, &u.tokens)
	})
	if err != nil {
		return err
	}
	total := 0
	for _, u := range usages {
		total += u.tokens
		if _, err := tx.Exec(ctx,
			`UPDATE token_lots SET tokens_remaining = tokens_remaining + $2 WHERE id = $1`, u.lot, u.tokens); err != nil {
			return err
		}
	}
	if total == 0 {
		return nil
	}

	patient, err := userAccount(ctx, tx, patientID, AcctPatientTokens)
	if err != nil {
		return err
	}
	escrow, err := systemAccount(ctx, tx, AcctEscrow)
	if err != nil {
		return err
	}
	amount := int64(total) * TokenValuePaisa
	return post(ctx, tx, TxConsultationRefund, &consultationID, entry{escrow, -amount}, entry{patient, amount})
}

// ExpireTokens zeroes out expired lots and books their value as platform revenue.
// It returns the number of tokens expired.
func (s *Store) ExpireTokens(ctx context.Context) (int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		UPDATE token_lots l SET tokens_remaining = 0
		FROM (
			SELECT id, tokens_remaining FROM token_lots
			WHERE tokens_remaining > 0 AND expires_at <= $1
			FOR UPDATE SKIP LOCKED
		) old
		WHERE l.id = old.id
		RETURNING l.patient_id, old.tokens_remaining`, s.now())
	if err != nil {
		return 0, err
	}
	perPatient := map[string]int{}
	var patient string
	var n int
	_, err = pgx.ForEachRow(rows, []any{&patient, &n}, func() error {
		perPatient[patient] += n
		return nil
	})
	if err != nil {
		return 0, err
	}

	revenue, err := systemAccount(ctx, tx, AcctPlatformRevenue)
	if err != nil {
		return 0, err
	}
	total := 0
	for pid, tokens := range perPatient {
		acct, err := userAccount(ctx, tx, pid, AcctPatientTokens)
		if err != nil {
			return 0, err
		}
		amount := int64(tokens) * TokenValuePaisa
		if err := post(ctx, tx, TxTokenExpiry, nil, entry{acct, -amount}, entry{revenue, amount}); err != nil {
			return 0, err
		}
		total += tokens
	}
	return total, tx.Commit(ctx)
}
