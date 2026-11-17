package wallet

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/database/dbtest"
	"healthcare-platform/internal/user"
)

var t0 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

func newStore(t *testing.T) (*Store, *pgxpool.Pool) {
	pool := dbtest.New(t)
	s := NewStore(pool)
	s.now = func() time.Time { return t0 }
	return s, pool
}

func standardPackage(t *testing.T, s *Store) Package {
	t.Helper()
	ps, err := s.Packages(context.Background())
	if err != nil || len(ps) == 0 {
		t.Fatalf("packages: %v, %v", ps, err)
	}
	return ps[0]
}

func buy(t *testing.T, s *Store, patientID string) Payment {
	t.Helper()
	ctx := context.Background()
	p, err := s.StartPurchase(ctx, patientID, standardPackage(t, s).ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.CompletePayment(ctx, p.ID, "ref-"+p.ID)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// consultationFor inserts a bare consultation row so token usage can reference it.
// Its status is irrelevant to the wallet; a closed status avoids the one-open-per-pair rule.
func consultationFor(t *testing.T, pool *pgxpool.Pool, patientID, doctorID string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO consultations (patient_id, doctor_id, subject, description, fee_tokens, commission_bps, status)
		VALUES ($1, $2, 'subject', 'description', 1, 5000, 'completed') RETURNING id`,
		patientID, doctorID).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func inTx(t *testing.T, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func balance(t *testing.T, pool *pgxpool.Pool, ownerID *string, kind string) int64 {
	t.Helper()
	var b int64
	err := pool.QueryRow(context.Background(), `
		SELECT COALESCE((SELECT balance_paisa FROM ledger_accounts
		                 WHERE owner_id IS NOT DISTINCT FROM $1 AND kind = $2), 0)`, ownerID, kind).Scan(&b)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// checkInvariants verifies the ledger is internally consistent.
func checkInvariants(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var total int64
	var mismatched, unbalanced, walletDrift int
	err := pool.QueryRow(ctx, `
		SELECT
		  (SELECT COALESCE(sum(balance_paisa), 0) FROM ledger_accounts),
		  (SELECT count(*) FROM ledger_accounts a
		    WHERE a.balance_paisa <> (SELECT COALESCE(sum(amount_paisa), 0) FROM ledger_entries e WHERE e.account_id = a.id)),
		  (SELECT count(*) FROM (SELECT transaction_id FROM ledger_entries GROUP BY transaction_id HAVING sum(amount_paisa) <> 0) x),
		  (SELECT count(*) FROM ledger_accounts a
		    WHERE a.kind = 'patient_tokens'
		      AND a.balance_paisa <> (SELECT COALESCE(sum(tokens_remaining), 0) FROM token_lots l WHERE l.patient_id = a.owner_id) * $1)`,
		TokenValuePaisa).Scan(&total, &mismatched, &unbalanced, &walletDrift)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || mismatched != 0 || unbalanced != 0 || walletDrift != 0 {
		t.Fatalf("ledger invariants broken: sum=%d, cached-balance mismatches=%d, unbalanced txns=%d, wallet drift=%d",
			total, mismatched, unbalanced, walletDrift)
	}
}

func TestPurchaseIsIdempotent(t *testing.T) {
	s, pool := newStore(t)
	ctx := context.Background()
	patient := dbtest.CreateUser(t, pool, "Patient", "01811111111", user.RolePatient)

	p := buy(t, s, patient)
	again, err := s.CompletePayment(ctx, p.ID, "duplicate-callback")
	if err != nil || again.Status != "paid" {
		t.Fatalf("second callback: %+v, %v", again, err)
	}

	w, err := s.Wallet(ctx, patient)
	if err != nil {
		t.Fatal(err)
	}
	if w.TokensAvailable != 10 || len(w.Lots) != 1 {
		t.Fatalf("wallet = %+v, want 10 tokens in one lot", w)
	}
	if !w.Lots[0].ExpiresAt.Equal(t0.AddDate(0, 0, 365)) {
		t.Fatalf("expiry = %v", w.Lots[0].ExpiresAt)
	}
	if got := balance(t, pool, nil, AcctGateway); got != -100000 {
		t.Fatalf("gateway balance = %d", got)
	}
	_, err = s.StartPurchase(ctx, patient, "00000000-0000-0000-0000-000000000000", "test")
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Fatalf("unknown package: %v", err)
	}
	checkInvariants(t, pool)
}

func TestHoldSpendsEarliestExpiringTokensAndRefundRestoresThem(t *testing.T) {
	s, pool := newStore(t)
	ctx := context.Background()
	patient := dbtest.CreateUser(t, pool, "Patient", "01811111111", user.RolePatient)
	doctor := dbtest.CreateUser(t, pool, "Doctor", "01711111111", user.RoleDoctor)

	buy(t, s, patient) // lot A expires first
	s.now = func() time.Time { return t0.Add(24 * time.Hour) }
	buy(t, s, patient) // lot B

	c1 := consultationFor(t, pool, patient, doctor)
	if err := inTx(t, pool, func(tx pgx.Tx) error { return Hold(ctx, tx, s.now(), patient, c1, 12) }); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Wallet(ctx, patient)
	if w.TokensAvailable != 8 || len(w.Lots) != 1 || w.Lots[0].TokensRemaining != 8 {
		t.Fatalf("after hold: %+v (lot A should be empty, lot B has 8)", w)
	}
	if got := balance(t, pool, nil, AcctEscrow); got != 12*TokenValuePaisa {
		t.Fatalf("escrow = %d", got)
	}

	c2 := consultationFor(t, pool, patient, doctor)
	err := inTx(t, pool, func(tx pgx.Tx) error { return Hold(ctx, tx, s.now(), patient, c2, 9) })
	if !apperr.Is(err, apperr.KindConflict) {
		t.Fatalf("hold beyond balance: %v", err)
	}

	if err := inTx(t, pool, func(tx pgx.Tx) error { return Refund(ctx, tx, c1, patient) }); err != nil {
		t.Fatal(err)
	}
	// A second refund finds nothing left to return.
	if err := inTx(t, pool, func(tx pgx.Tx) error { return Refund(ctx, tx, c1, patient) }); err != nil {
		t.Fatal(err)
	}
	w, _ = s.Wallet(ctx, patient)
	if w.TokensAvailable != 20 || w.Lots[0].TokensRemaining != 10 || w.Lots[1].TokensRemaining != 10 {
		t.Fatalf("after refund: %+v", w)
	}
	if got := balance(t, pool, nil, AcctEscrow); got != 0 {
		t.Fatalf("escrow after refund = %d", got)
	}
	checkInvariants(t, pool)
}

func TestReleaseSplitsFee(t *testing.T) {
	s, pool := newStore(t)
	ctx := context.Background()
	patient := dbtest.CreateUser(t, pool, "Patient", "01811111111", user.RolePatient)
	doctor := dbtest.CreateUser(t, pool, "Doctor", "01711111111", user.RoleDoctor)
	buy(t, s, patient)

	// The specialist example from the spec: 3 tokens, doctor gets 2 and platform 1.
	c := consultationFor(t, pool, patient, doctor)
	err := inTx(t, pool, func(tx pgx.Tx) error {
		if err := Hold(ctx, tx, s.now(), patient, c, 3); err != nil {
			return err
		}
		return Release(ctx, tx, c, doctor, 3, 3333)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := balance(t, pool, &doctor, AcctDoctorEarnings); got != 20001 {
		t.Fatalf("doctor earnings = %d, want 20001", got)
	}
	if got := balance(t, pool, nil, AcctPlatformRevenue); got != 9999 {
		t.Fatalf("platform revenue = %d, want 9999", got)
	}

	e, err := s.Earnings(ctx, doctor, 20, 0)
	if err != nil || e.BalancePaisa != 20001 || len(e.Entries) != 1 || e.Entries[0].Kind != TxConsultationRelease {
		t.Fatalf("earnings view: %+v, %v", e, err)
	}
	checkInvariants(t, pool)
}

func TestExpireTokens(t *testing.T) {
	s, pool := newStore(t)
	ctx := context.Background()
	patient := dbtest.CreateUser(t, pool, "Patient", "01811111111", user.RolePatient)
	doctor := dbtest.CreateUser(t, pool, "Doctor", "01711111111", user.RoleDoctor)
	buy(t, s, patient)

	c := consultationFor(t, pool, patient, doctor)
	if err := inTx(t, pool, func(tx pgx.Tx) error { return Hold(ctx, tx, s.now(), patient, c, 4) }); err != nil {
		t.Fatal(err)
	}

	if n, err := s.ExpireTokens(ctx); err != nil || n != 0 {
		t.Fatalf("nothing should expire yet: %d, %v", n, err)
	}

	s.now = func() time.Time { return t0.AddDate(1, 0, 1) }
	n, err := s.ExpireTokens(ctx)
	if err != nil || n != 6 {
		t.Fatalf("expired %d tokens (%v), want 6", n, err)
	}
	if w, _ := s.Wallet(ctx, patient); w.TokensAvailable != 0 {
		t.Fatalf("wallet after expiry = %+v", w)
	}

	// A refund after expiry puts tokens back on an expired lot; the next sweep takes them.
	if err := inTx(t, pool, func(tx pgx.Tx) error { return Refund(ctx, tx, c, patient) }); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ExpireTokens(ctx); err != nil || n != 4 {
		t.Fatalf("second sweep expired %d (%v), want 4", n, err)
	}
	if got := balance(t, pool, nil, AcctPlatformRevenue); got != 10*TokenValuePaisa {
		t.Fatalf("platform revenue = %d", got)
	}
	checkInvariants(t, pool)
}
