package pharmacy

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/httpx"
)

// MaxUploadBytes limits an uploaded prescription (10 MB).
const MaxUploadBytes = 10 << 20

type Handler struct {
	store *Store
	log   *slog.Logger
}

func NewHandler(store *Store, log *slog.Logger) *Handler { return &Handler{store: store, log: log} }

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, h.log, err)
}

func uid(r *http.Request) string {
	p, _ := auth.FromContext(r.Context())
	return p.UserID
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := httpx.Decode(w, r, dst); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

// ids reads UUID path parameters in order; it writes the error and returns false on failure.
func (h *Handler) ids(w http.ResponseWriter, r *http.Request, names ...string) ([]string, bool) {
	out := make([]string, len(names))
	for i, n := range names {
		v, err := httpx.PathUUID(r, n)
		if err != nil {
			h.fail(w, r, err)
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, status, v)
}

// ---------------------------------------------------------------- ownership

type pharmacyKey struct{}

// Owner lets the request through only if the caller owns the pharmacy in the URL.
func (h *Handler) Owner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := httpx.PathUUID(r, "pharmacyID")
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if err := h.store.IsOwner(r.Context(), uid(r), id); err != nil {
			h.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), pharmacyKey{}, id)))
	})
}

func pid(r *http.Request) string { id, _ := r.Context().Value(pharmacyKey{}).(string); return id }

// ---------------------------------------------------------------- public

// GET /pharmacies?latitude=&longitude=&radius_km=
func (h *Handler) Nearby(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat, err1 := strconv.ParseFloat(q.Get("latitude"), 64)
	lng, err2 := strconv.ParseFloat(q.Get("longitude"), 64)
	if err1 != nil || err2 != nil || checkLocation(&lat, &lng) != nil {
		h.fail(w, r, apperr.Validation("latitude and longitude (inside Bangladesh) are required"))
		return
	}
	radius := 5.0
	if v, err := strconv.ParseFloat(q.Get("radius_km"), 64); err == nil && v > 0 {
		radius = min(v, 20)
	}
	limit, offset := httpx.Page(r)
	list, err := h.store.Nearby(r.Context(), lat, lng, radius, limit, offset)
	h.respond(w, r, http.StatusOK, map[string]any{"pharmacies": list, "limit": limit, "offset": offset}, err)
}

// GET /pharmacies/{pharmacyID}
func (h *Handler) Public(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.ids(w, r, "pharmacyID")
	if !ok {
		return
	}
	p, reviews, err := h.store.Public(r.Context(), ids[0])
	h.respond(w, r, http.StatusOK, map[string]any{"pharmacy": p, "reviews": reviews}, err)
}

// ---------------------------------------------------------------- patient

// POST /medicine-requests
//
// JSON body for a platform prescription. To upload a prescription instead, send
// multipart/form-data with a "data" field (the same JSON) followed by a "file" field.
func (h *Handler) CreateRequest(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		var in RequestInput
		if !decode(w, r, &in) {
			return
		}
		rq, err := h.store.CreateRequest(r.Context(), uid(r), in, nil)
		h.respond(w, r, http.StatusCreated, rq, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid multipart upload")
		return
	}
	var in RequestInput
	gotData := false
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid multipart upload (or file larger than 10 MB)")
			return
		}
		switch part.FormName() {
		case "data":
			dec := json.NewDecoder(io.LimitReader(part, 64<<10))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&in); err != nil {
				httpx.Error(w, http.StatusBadRequest, "data: invalid JSON: "+err.Error())
				return
			}
			gotData = true
		case "file":
			if !gotData {
				httpx.Error(w, http.StatusBadRequest, `send the "data" field before the "file" field`)
				return
			}
			br := bufio.NewReaderSize(part, 512)
			head, _ := br.Peek(512)
			ct, _, _ := mime.ParseMediaType(http.DetectContentType(head))
			if !UploadTypes[ct] {
				httpx.Error(w, http.StatusUnsupportedMediaType, "the prescription must be a PDF, JPEG or PNG file")
				return
			}
			limited := &limitReader{r: br, left: MaxUploadBytes}
			rq, err := h.store.CreateRequest(r.Context(), uid(r), in, &Upload{ContentType: ct, Body: limited})
			if limited.exceeded {
				httpx.Error(w, http.StatusRequestEntityTooLarge, "file is larger than 10 MB")
				return
			}
			h.respond(w, r, http.StatusCreated, rq, err)
			return
		}
	}
	if !gotData {
		httpx.Error(w, http.StatusBadRequest, `missing "data" field`)
		return
	}
	rq, err := h.store.CreateRequest(r.Context(), uid(r), in, nil)
	h.respond(w, r, http.StatusCreated, rq, err)
}

type limitReader struct {
	r        *bufio.Reader
	left     int64
	exceeded bool
}

func (l *limitReader) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.left -= int64(n)
	if l.left < 0 {
		l.exceeded = true
		return n, apperr.Validation("file too large")
	}
	return n, err
}

// GET /medicine-requests
func (h *Handler) MyRequests(w http.ResponseWriter, r *http.Request) {
	limit, offset := httpx.Page(r)
	list, err := h.store.MyRequests(r.Context(), uid(r), limit, offset)
	h.respond(w, r, http.StatusOK, map[string]any{"requests": list, "limit": limit, "offset": offset}, err)
}

// GET /medicine-requests/{requestID}
func (h *Handler) MyRequest(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "requestID"); ok {
		rq, err := h.store.RequestForPatient(r.Context(), uid(r), ids[0])
		h.respond(w, r, http.StatusOK, rq, err)
	}
}

// POST /medicine-requests/{requestID}/cancel
func (h *Handler) CancelRequest(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "requestID"); ok {
		rq, err := h.store.CancelRequest(r.Context(), uid(r), ids[0])
		h.respond(w, r, http.StatusOK, rq, err)
	}
}

// GET /medicine-requests/{requestID}/attachment
func (h *Handler) MyAttachment(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "requestID"); ok {
		h.serveAttachment(w, r, uid(r), "", ids[0])
	}
}

func (h *Handler) serveAttachment(w http.ResponseWriter, r *http.Request, patientID, pharmacyID, requestID string) {
	ct, rc, err := h.store.OpenAttachment(r.Context(), patientID, pharmacyID, requestID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "inline")
	// User-uploaded content: never let it run scripts in our origin.
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = bufio.NewReader(rc).WriteTo(w)
}

// POST /quotes/{quoteID}/accept
func (h *Handler) AcceptQuote(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "quoteID"); ok {
		o, err := h.store.AcceptQuote(r.Context(), uid(r), ids[0])
		h.respond(w, r, http.StatusCreated, o, err)
	}
}

// GET /medicine-orders
func (h *Handler) MyOrders(w http.ResponseWriter, r *http.Request) {
	limit, offset := httpx.Page(r)
	list, err := h.store.PatientOrders(r.Context(), uid(r), limit, offset)
	h.respond(w, r, http.StatusOK, map[string]any{"orders": list, "limit": limit, "offset": offset}, err)
}

// GET /medicine-orders/{orderID}
func (h *Handler) MyOrder(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "orderID"); ok {
		o, err := h.store.OrderForPatient(r.Context(), uid(r), ids[0])
		h.respond(w, r, http.StatusOK, o, err)
	}
}

type reasonInput struct {
	Reason string `json:"reason"`
}

// POST /medicine-orders/{orderID}/cancel
func (h *Handler) CancelMyOrder(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.ids(w, r, "orderID")
	var in reasonInput
	if !ok || !decode(w, r, &in) {
		return
	}
	o, err := h.store.CancelByPatient(r.Context(), uid(r), ids[0], in.Reason)
	h.respond(w, r, http.StatusOK, o, err)
}

// POST /medicine-orders/{orderID}/review
func (h *Handler) ReviewOrder(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.ids(w, r, "orderID")
	var in ReviewInput
	if !ok || !decode(w, r, &in) {
		return
	}
	if err := h.store.Review(r.Context(), uid(r), ids[0], in); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- pharmacy

// POST /pharmacies
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in PharmacyInput
	if decode(w, r, &in) {
		p, err := h.store.Create(r.Context(), uid(r), in)
		h.respond(w, r, http.StatusCreated, p, err)
	}
}

// GET /manage/pharmacies
func (h *Handler) Mine(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.Mine(r.Context(), uid(r))
	h.respond(w, r, http.StatusOK, map[string]any{"pharmacies": list}, err)
}

// GET /manage/pharmacies/{pharmacyID}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p, err := h.store.Get(r.Context(), pid(r))
	h.respond(w, r, http.StatusOK, p, err)
}

// PUT /manage/pharmacies/{pharmacyID}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var in PharmacyInput
	if decode(w, r, &in) {
		p, err := h.store.Update(r.Context(), pid(r), in)
		h.respond(w, r, http.StatusOK, p, err)
	}
}

// GET /manage/pharmacies/{pharmacyID}/requests
func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	limit, offset := httpx.Page(r)
	list, err := h.store.Feed(r.Context(), pid(r), limit, offset)
	h.respond(w, r, http.StatusOK, map[string]any{"requests": list, "limit": limit, "offset": offset}, err)
}

// GET /manage/pharmacies/{pharmacyID}/requests/{requestID}
func (h *Handler) Request(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "requestID"); ok {
		rq, err := h.store.RequestForPharmacy(r.Context(), pid(r), ids[0])
		h.respond(w, r, http.StatusOK, rq, err)
	}
}

// GET /manage/pharmacies/{pharmacyID}/requests/{requestID}/attachment
func (h *Handler) RequestAttachment(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "requestID"); ok {
		h.serveAttachment(w, r, "", pid(r), ids[0])
	}
}

// POST /manage/pharmacies/{pharmacyID}/requests/{requestID}/quote
func (h *Handler) Quote(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.ids(w, r, "requestID")
	var in QuoteInput
	if !ok || !decode(w, r, &in) {
		return
	}
	q, err := h.store.SubmitQuote(r.Context(), pid(r), ids[0], in)
	h.respond(w, r, http.StatusCreated, q, err)
}

// POST /manage/pharmacies/{pharmacyID}/quotes/{quoteID}/withdraw
func (h *Handler) WithdrawQuote(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "quoteID"); ok {
		q, err := h.store.WithdrawQuote(r.Context(), pid(r), ids[0])
		h.respond(w, r, http.StatusOK, q, err)
	}
}

// GET /manage/pharmacies/{pharmacyID}/orders?status=active
func (h *Handler) Orders(w http.ResponseWriter, r *http.Request) {
	limit, offset := httpx.Page(r)
	list, err := h.store.PharmacyOrders(r.Context(), pid(r), r.URL.Query().Get("status"), limit, offset)
	h.respond(w, r, http.StatusOK, map[string]any{"orders": list, "limit": limit, "offset": offset}, err)
}

// GET /manage/pharmacies/{pharmacyID}/orders/{orderID}
func (h *Handler) Order(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "orderID"); ok {
		o, err := h.store.OrderForPharmacy(r.Context(), pid(r), ids[0])
		h.respond(w, r, http.StatusOK, o, err)
	}
}

// POST /manage/pharmacies/{pharmacyID}/orders/{orderID}/preparing and /ready
func (h *Handler) Advance(to string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ids, ok := h.ids(w, r, "orderID"); ok {
			o, err := h.store.Advance(r.Context(), pid(r), uid(r), ids[0], to)
			h.respond(w, r, http.StatusOK, o, err)
		}
	}
}

type dispatchInput struct {
	Mode string `json:"mode"` // pharmacy | rider
}

// POST /manage/pharmacies/{pharmacyID}/orders/{orderID}/dispatch
func (h *Handler) Dispatch(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.ids(w, r, "orderID")
	var in dispatchInput
	if !ok || !decode(w, r, &in) {
		return
	}
	o, err := h.store.Dispatch(r.Context(), pid(r), uid(r), ids[0], in.Mode)
	h.respond(w, r, http.StatusOK, o, err)
}

type codeInput struct {
	Code string `json:"code"`
}

// POST /manage/pharmacies/{pharmacyID}/orders/{orderID}/complete
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.ids(w, r, "orderID")
	var in codeInput
	if !ok || !decode(w, r, &in) {
		return
	}
	o, err := h.store.Complete(r.Context(), pid(r), uid(r), ids[0], in.Code)
	h.respond(w, r, http.StatusOK, o, err)
}

// POST /manage/pharmacies/{pharmacyID}/orders/{orderID}/cancel
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.ids(w, r, "orderID")
	var in reasonInput
	if !ok || !decode(w, r, &in) {
		return
	}
	o, err := h.store.CancelByPharmacy(r.Context(), pid(r), uid(r), ids[0], in.Reason)
	h.respond(w, r, http.StatusOK, o, err)
}

// GET /manage/pharmacies/{pharmacyID}/statement?from=YYYY-MM-DD&to=YYYY-MM-DD
func (h *Handler) Statement(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	st, err := h.store.Statement(r.Context(), pid(r), q.Get("from"), q.Get("to"))
	h.respond(w, r, http.StatusOK, st, err)
}

// ---------------------------------------------------------------- rider

// GET /rider/profile
func (h *Handler) RiderProfile(w http.ResponseWriter, r *http.Request) {
	rd, err := h.store.Rider(r.Context(), uid(r))
	h.respond(w, r, http.StatusOK, rd, err)
}

// PUT /rider/profile
func (h *Handler) RegisterRider(w http.ResponseWriter, r *http.Request) {
	var in RiderInput
	if decode(w, r, &in) {
		rd, err := h.store.RegisterRider(r.Context(), uid(r), in)
		h.respond(w, r, http.StatusOK, rd, err)
	}
}

// PUT /rider/availability
func (h *Handler) Availability(w http.ResponseWriter, r *http.Request) {
	var in AvailabilityInput
	if decode(w, r, &in) {
		rd, err := h.store.SetAvailability(r.Context(), uid(r), in)
		h.respond(w, r, http.StatusOK, rd, err)
	}
}

type locationInput struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// PUT /rider/location
func (h *Handler) Location(w http.ResponseWriter, r *http.Request) {
	var in locationInput
	if !decode(w, r, &in) {
		return
	}
	if err := h.store.UpdateLocation(r.Context(), uid(r), in.Latitude, in.Longitude); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /rider/jobs
func (h *Handler) Jobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.store.Jobs(r.Context(), uid(r))
	h.respond(w, r, http.StatusOK, map[string]any{"jobs": jobs}, err)
}

// POST /rider/jobs/{orderID}/accept
func (h *Handler) AcceptJob(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "orderID"); ok {
		o, err := h.store.AcceptJob(r.Context(), uid(r), ids[0])
		h.respond(w, r, http.StatusOK, o, err)
	}
}

// GET /rider/orders
func (h *Handler) RiderOrders(w http.ResponseWriter, r *http.Request) {
	limit, offset := httpx.Page(r)
	list, err := h.store.RiderOrders(r.Context(), uid(r), limit, offset)
	h.respond(w, r, http.StatusOK, map[string]any{"orders": list, "limit": limit, "offset": offset}, err)
}

// GET /rider/orders/{orderID}
func (h *Handler) RiderOrder(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "orderID"); ok {
		o, err := h.store.OrderForRider(r.Context(), uid(r), ids[0])
		h.respond(w, r, http.StatusOK, o, err)
	}
}

// POST /rider/orders/{orderID}/release
func (h *Handler) ReleaseJob(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "orderID"); ok {
		if err := h.store.ReleaseJob(r.Context(), uid(r), ids[0]); err != nil {
			h.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// POST /rider/orders/{orderID}/picked-up
func (h *Handler) PickedUp(w http.ResponseWriter, r *http.Request) {
	if ids, ok := h.ids(w, r, "orderID"); ok {
		o, err := h.store.PickedUp(r.Context(), uid(r), ids[0])
		h.respond(w, r, http.StatusOK, o, err)
	}
}

// POST /rider/orders/{orderID}/delivered
func (h *Handler) Delivered(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.ids(w, r, "orderID")
	var in codeInput
	if !ok || !decode(w, r, &in) {
		return
	}
	o, err := h.store.Deliver(r.Context(), uid(r), ids[0], in.Code)
	h.respond(w, r, http.StatusOK, o, err)
}

// GET /rider/earnings
func (h *Handler) RiderEarnings(w http.ResponseWriter, r *http.Request) {
	e, err := h.store.Earnings(r.Context(), uid(r))
	h.respond(w, r, http.StatusOK, e, err)
}

// ---------------------------------------------------------------- admin

func statusFilter(w http.ResponseWriter, r *http.Request, h *Handler) (string, bool) {
	s := r.URL.Query().Get("status")
	if s != "" && s != StatusPending && s != StatusApproved && s != StatusRejected {
		h.fail(w, r, apperr.Validation("status must be pending, approved or rejected"))
		return "", false
	}
	return s, true
}

// GET /admin/pharmacies?status=pending
func (h *Handler) AdminPharmacies(w http.ResponseWriter, r *http.Request) {
	if s, ok := statusFilter(w, r, h); ok {
		limit, offset := httpx.Page(r)
		list, err := h.store.AdminList(r.Context(), s, limit, offset)
		h.respond(w, r, http.StatusOK, map[string]any{"pharmacies": list, "limit": limit, "offset": offset}, err)
	}
}

// POST /admin/pharmacies/{pharmacyID}/verification
func (h *Handler) AdminVerifyPharmacy(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.ids(w, r, "pharmacyID")
	var in VerifyInput
	if ok && decode(w, r, &in) {
		p, err := h.store.Verify(r.Context(), ids[0], in)
		h.respond(w, r, http.StatusOK, p, err)
	}
}

// GET /admin/riders?status=pending
func (h *Handler) AdminRiders(w http.ResponseWriter, r *http.Request) {
	if s, ok := statusFilter(w, r, h); ok {
		limit, offset := httpx.Page(r)
		list, err := h.store.AdminRiders(r.Context(), s, limit, offset)
		h.respond(w, r, http.StatusOK, map[string]any{"riders": list, "limit": limit, "offset": offset}, err)
	}
}

// POST /admin/riders/{userID}/verification
func (h *Handler) AdminVerifyRider(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.ids(w, r, "userID")
	var in VerifyInput
	if ok && decode(w, r, &in) {
		rd, err := h.store.VerifyRider(r.Context(), ids[0], in)
		h.respond(w, r, http.StatusOK, rd, err)
	}
}
