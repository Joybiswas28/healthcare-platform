// Package wallet manages consultation tokens, payments and the double-entry ledger.
//
// Money is tracked in paisa. One token is worth TokenValuePaisa. Every movement is a
// ledger transaction whose entries sum to zero, and each account keeps a cached
// balance that is updated in the same database transaction.
package wallet

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"healthcare-platform/internal/platform/database"
)

// TokenValuePaisa is the value of one consultation token (BDT 100).
const TokenValuePaisa int64 = 100 * 100

// Ledger account kinds.
const (
	AcctGateway         = "gateway"          // system: cash received from payment providers (negative)
	AcctEscrow          = "escrow"           // system: tokens held for active consultations
	AcctPlatformRevenue = "platform_revenue" // system: commission, expired tokens, discounts
	AcctPatientTokens   = "patient_tokens"   // per patient: value of unspent tokens
	AcctDoctorEarnings  = "doctor_earnings"  // per doctor: earnings not yet withdrawn
)

// Ledger transaction kinds.
const (
	TxTokenPurchase       = "token_purchase"
	TxConsultationHold    = "consultation_hold"
	TxConsultationRelease = "consultation_release"
	TxConsultationRefund  = "consultation_refund"
	TxTokenExpiry         = "token_expiry"
)

var errInsufficientBalance = errors.New("ledger: insufficient balance")

type entry struct {
	account string
	amount  int64
}

func userAccount(ctx context.Context, q database.Querier, ownerID, kind string) (string, error) {
	if _, err := q.Exec(ctx, `
		INSERT INTO ledger_accounts (owner_id, kind) VALUES ($1, $2)
		ON CONFLICT (owner_id, kind) WHERE owner_id IS NOT NULL DO NOTHING`, ownerID, kind); err != nil {
		return "", err
	}
	var id string
	err := q.QueryRow(ctx, `SELECT id FROM ledger_accounts WHERE owner_id = $1 AND kind = $2`, ownerID, kind).Scan(&id)
	return id, err
}

func systemAccount(ctx context.Context, q database.Querier, kind string) (string, error) {
	var id string
	err := q.QueryRow(ctx, `SELECT id FROM ledger_accounts WHERE owner_id IS NULL AND kind = $1`, kind).Scan(&id)
	return id, err
}

// post records a balanced ledger transaction. Balances are updated in account-id
// order so concurrent transactions always lock accounts in the same order and
// cannot deadlock each other.
func post(ctx context.Context, tx pgx.Tx, kind string, ref *string, entries ...entry) error {
	var sum int64
	nonZero := entries[:0:0]
	for _, e := range entries {
		sum += e.amount
		if e.amount != 0 {
			nonZero = append(nonZero, e)
		}
	}
	if sum != 0 {
		return fmt.Errorf("ledger: unbalanced %s transaction (sum %d)", kind, sum)
	}
	if len(nonZero) == 0 {
		return nil
	}
	sort.Slice(nonZero, func(i, j int) bool { return nonZero[i].account < nonZero[j].account })

	var txID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO ledger_transactions (kind, reference_id) VALUES ($1, $2) RETURNING id`,
		kind, ref).Scan(&txID); err != nil {
		return err
	}
	for _, e := range nonZero {
		if _, err := tx.Exec(ctx,
			`INSERT INTO ledger_entries (transaction_id, account_id, amount_paisa) VALUES ($1, $2, $3)`,
			txID, e.account, e.amount); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE ledger_accounts SET balance_paisa = balance_paisa + $2 WHERE id = $1`,
			e.account, e.amount); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.ConstraintName == "ledger_accounts_non_negative" {
				return errInsufficientBalance
			}
			return err
		}
	}
	return nil
}
