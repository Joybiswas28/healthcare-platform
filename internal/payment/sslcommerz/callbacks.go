package sslcommerz

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"healthcare-platform/internal/platform/httpx"
	"healthcare-platform/internal/wallet"
)

// Callbacks handles the requests SSLCommerz sends back to us.
type Callbacks struct {
	client      *Client
	wallet      *wallet.Store
	frontendURL string
	log         *slog.Logger
}

func NewCallbacks(client *Client, ws *wallet.Store, frontendURL string, log *slog.Logger) *Callbacks {
	return &Callbacks{client: client, wallet: ws, frontendURL: strings.TrimRight(frontendURL, "/"), log: log}
}

var errNotConfirmed = errors.New("payment not confirmed by SSLCommerz")

// confirm validates val_id with SSLCommerz and, if the payment checks out, credits it.
func (cb *Callbacks) confirm(ctx context.Context, valID string) (string, error) {
	if valID == "" {
		return "", errNotConfirmed
	}
	v, err := cb.client.Validate(ctx, valID)
	if err != nil {
		return "", fmt.Errorf("validate: %w", err)
	}
	if !v.OK() {
		return v.TranID, fmt.Errorf("%w: status %s", errNotConfirmed, v.Status)
	}
	if !httpx.IsUUID(v.TranID) {
		return "", fmt.Errorf("%w: unknown tran_id %q", errNotConfirmed, v.TranID)
	}
	p, err := cb.wallet.PaymentByID(ctx, v.TranID)
	if err != nil {
		return v.TranID, err
	}
	paid, err := ParseTaka(v.Amount)
	if err != nil || paid != p.AmountPaisa || v.Currency != "BDT" || p.Provider != cb.client.Name() {
		return p.ID, fmt.Errorf("%w: amount %s %s does not match payment %d paisa", errNotConfirmed, v.Amount, v.Currency, p.AmountPaisa)
	}
	if v.RiskLevel == "1" {
		// SSLCommerz flags the transaction as risky: keep it pending for a human to review.
		cb.log.WarnContext(ctx, "risky payment held for review", "payment", p.ID, "val_id", valID)
		return p.ID, fmt.Errorf("%w: flagged as risky", errNotConfirmed)
	}
	ref := v.BankTranID
	if ref == "" {
		ref = valID
	}
	if _, err := cb.wallet.CompletePayment(ctx, p.ID, ref); err != nil {
		return p.ID, err
	}
	return p.ID, nil
}

func formValue(w http.ResponseWriter, r *http.Request, key string) string {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		return ""
	}
	return strings.TrimSpace(r.PostForm.Get(key))
}

// IPN (POST /payments/sslcommerz/ipn) is SSLCommerz's server-to-server notification.
func (cb *Callbacks) IPN(w http.ResponseWriter, r *http.Request) {
	valID := formValue(w, r, "val_id")
	id, err := cb.confirm(r.Context(), valID)
	if err != nil {
		cb.log.WarnContext(r.Context(), "sslcommerz ipn not credited", "payment", id, "val_id", valID, "err", err)
		// Answer 200 anyway: the IPN was received, and retries would not change the outcome.
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// Success (POST /payments/sslcommerz/success) is where the patient's browser lands
// after paying. We confirm with SSLCommerz, then send them to the frontend.
func (cb *Callbacks) Success(w http.ResponseWriter, r *http.Request) {
	valID := formValue(w, r, "val_id")
	tranID := r.PostForm.Get("tran_id")
	id, err := cb.confirm(r.Context(), valID)
	if id == "" {
		id = tranID
	}
	status := "success"
	if err != nil {
		cb.log.WarnContext(r.Context(), "sslcommerz success redirect not credited", "payment", id, "err", err)
		status = "pending" // the IPN may still confirm it; the frontend should poll the payment
	}
	cb.redirect(w, r, id, status)
}

// Fail (POST /payments/sslcommerz/fail) and Cancel (POST /payments/sslcommerz/cancel).
func (cb *Callbacks) Fail(w http.ResponseWriter, r *http.Request)   { cb.closeOut(w, r, "failed") }
func (cb *Callbacks) Cancel(w http.ResponseWriter, r *http.Request) { cb.closeOut(w, r, "cancelled") }

func (cb *Callbacks) closeOut(w http.ResponseWriter, r *http.Request, status string) {
	tranID := formValue(w, r, "tran_id")
	if httpx.IsUUID(tranID) {
		// Informational only; a later confirmed IPN can still complete the payment.
		if err := cb.wallet.FailPayment(r.Context(), tranID); err != nil {
			cb.log.ErrorContext(r.Context(), "mark payment failed", "payment", tranID, "err", err)
		}
	}
	cb.redirect(w, r, tranID, status)
}

func (cb *Callbacks) redirect(w http.ResponseWriter, r *http.Request, paymentID, status string) {
	target := cb.frontendURL + "/payments/result?status=" + url.QueryEscape(status)
	if httpx.IsUUID(paymentID) {
		target += "&payment_id=" + paymentID
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
