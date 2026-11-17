package wallet

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"healthcare-platform/internal/platform/apperr"
)

// Payer identifies the person paying, for the payment provider's records.
type Payer struct {
	Name  string
	Phone string
}

// Gateway starts a payment with a provider and returns the URL the patient must
// open to pay. The provider later confirms the payment through a callback, which
// ends in CompletePayment.
type Gateway interface {
	Name() string
	Checkout(ctx context.Context, p Payment, payer Payer) (checkoutURL string, err error)
}

// DevGateway pretends to be a payment provider: its checkout URL is the
// development-only endpoint that marks the payment as paid.
type DevGateway struct{}

func (DevGateway) Name() string { return ProviderDev }

func (DevGateway) Checkout(_ context.Context, p Payment, _ Payer) (string, error) {
	return "/api/v1/dev/payments/" + p.ID + "/succeed", nil
}

func (s *Store) Payer(ctx context.Context, userID string) (Payer, error) {
	var p Payer
	err := s.db.QueryRow(ctx, `SELECT name, phone FROM users WHERE id = $1`, userID).Scan(&p.Name, &p.Phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payer{}, apperr.NotFound("user not found")
	}
	return p, err
}

// PaymentByID loads any payment. Only for trusted callers such as payment callbacks
// that have already verified the payment with the provider.
func (s *Store) PaymentByID(ctx context.Context, id string) (Payment, error) {
	return scanPayment(s.db.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id = $1`, id))
}

// FailPayment marks a pending payment as failed. It is informational: if the
// provider later confirms the payment after all, CompletePayment still credits it.
func (s *Store) FailPayment(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE payments SET status = 'failed' WHERE id = $1 AND status = 'pending'`, id)
	return err
}
