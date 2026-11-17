package wallet

import (
	"log/slog"
	"net/http"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/httpx"
)

// ProviderDev simulates a payment gateway in development. A real provider
// (SSLCommerz, bKash) will redirect the patient to its checkout page and
// call back to confirm the payment.
const ProviderDev = "dev"

type Handler struct {
	store *Store
	log   *slog.Logger
}

func NewHandler(store *Store, log *slog.Logger) *Handler {
	return &Handler{store: store, log: log}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, h.log, err)
}

func callerID(r *http.Request) string {
	p, _ := auth.FromContext(r.Context())
	return p.UserID
}

// GET /token-packages
func (h *Handler) Packages(w http.ResponseWriter, r *http.Request) {
	ps, err := h.store.Packages(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"packages": ps, "token_value_paisa": TokenValuePaisa})
}

type purchaseInput struct {
	PackageID string `json:"package_id"`
}

// POST /wallet/purchases
func (h *Handler) StartPurchase(w http.ResponseWriter, r *http.Request) {
	var in purchaseInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if !httpx.IsUUID(in.PackageID) {
		h.fail(w, r, apperr.Validation("package_id is required"))
		return
	}
	p, err := h.store.StartPurchase(r.Context(), callerID(r), in.PackageID, ProviderDev)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p.CheckoutURL = "/api/v1/dev/payments/" + p.ID + "/succeed"
	httpx.JSON(w, http.StatusCreated, p)
}

// GET /wallet/purchases/{paymentID}
func (h *Handler) GetPurchase(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "paymentID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p, err := h.store.GetPayment(r.Context(), callerID(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// POST /dev/payments/{paymentID}/succeed (development only)
// Simulates the payment provider confirming a payment.
func (h *Handler) DevSucceed(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "paymentID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.store.GetPayment(r.Context(), callerID(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	p, err := h.store.CompletePayment(r.Context(), id, "dev-"+id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// GET /wallet
func (h *Handler) Wallet(w http.ResponseWriter, r *http.Request) {
	wl, err := h.store.Wallet(r.Context(), callerID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, wl)
}

// GET /wallet/history
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	limit, offset := httpx.Page(r)
	es, err := h.store.History(r.Context(), callerID(r), AcctPatientTokens, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"entries": es, "limit": limit, "offset": offset})
}

// GET /doctor/earnings
func (h *Handler) Earnings(w http.ResponseWriter, r *http.Request) {
	limit, offset := httpx.Page(r)
	e, err := h.store.Earnings(r.Context(), callerID(r), limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, e)
}
