// Package sslcommerz integrates the SSLCommerz payment gateway (cards, bKash,
// Nagad, Rocket, internet banking) for token purchases.
//
// Flow:
//  1. Checkout creates a session and returns SSLCommerz's hosted payment page.
//  2. After paying, SSLCommerz calls our IPN URL (server to server) and redirects the
//     patient's browser to our success / fail / cancel URL.
//  3. On IPN and success we never trust the posted form: we ask SSLCommerz's
//     validation API about the val_id, check the amount and currency against our
//     payment record, and only then credit tokens. Crediting is idempotent, so the
//     IPN and the redirect can both arrive without double-crediting.
package sslcommerz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"healthcare-platform/internal/wallet"
)

const (
	SandboxURL = "https://sandbox.sslcommerz.com"
	LiveURL    = "https://securepay.sslcommerz.com"
)

type Config struct {
	StoreID       string
	StorePassword string
	Sandbox       bool
	// PublicBaseURL is where SSLCommerz can reach this API (for IPN and redirects).
	PublicBaseURL string
	// APIURL overrides the SSLCommerz host (used in tests).
	APIURL string
}

type Client struct {
	cfg  Config
	base string
	http *http.Client
}

func New(cfg Config) *Client {
	base := LiveURL
	if cfg.Sandbox {
		base = SandboxURL
	}
	if cfg.APIURL != "" {
		base = cfg.APIURL
	}
	cfg.PublicBaseURL = strings.TrimRight(cfg.PublicBaseURL, "/")
	return &Client{cfg: cfg, base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) Name() string { return "sslcommerz" }

func (c *Client) callback(kind string) string {
	return c.cfg.PublicBaseURL + "/api/v1/payments/sslcommerz/" + kind
}

type initResponse struct {
	Status         string `json:"status"`
	FailedReason   string `json:"failedreason"`
	GatewayPageURL string `json:"GatewayPageURL"`
}

// Checkout opens an SSLCommerz session for p. tran_id is our payment id.
func (c *Client) Checkout(ctx context.Context, p wallet.Payment, payer wallet.Payer) (string, error) {
	form := url.Values{
		"store_id":         {c.cfg.StoreID},
		"store_passwd":     {c.cfg.StorePassword},
		"total_amount":     {FormatTaka(p.AmountPaisa)},
		"currency":         {"BDT"},
		"tran_id":          {p.ID},
		"success_url":      {c.callback("success")},
		"fail_url":         {c.callback("fail")},
		"cancel_url":       {c.callback("cancel")},
		"ipn_url":          {c.callback("ipn")},
		"cus_name":         {payer.Name},
		"cus_phone":        {payer.Phone},
		"cus_email":        {"no-email@" + hostOf(c.cfg.PublicBaseURL)}, // required by SSLCommerz; we don't collect emails
		"cus_add1":         {"Bangladesh"},
		"cus_city":         {"Dhaka"},
		"cus_country":      {"Bangladesh"},
		"shipping_method":  {"NO"},
		"num_of_item":      {"1"},
		"product_name":     {fmt.Sprintf("%d consultation tokens", p.Tokens)},
		"product_category": {"Healthcare"},
		"product_profile":  {"non-physical-goods"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/gwprocess/v4/api.php", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var res initResponse
	if err := c.do(req, &res); err != nil {
		return "", err
	}
	if !strings.EqualFold(res.Status, "SUCCESS") || res.GatewayPageURL == "" {
		return "", fmt.Errorf("sslcommerz session refused: %s %s", res.Status, res.FailedReason)
	}
	return res.GatewayPageURL, nil
}

// Validation is the part of SSLCommerz's validation response we rely on.
type Validation struct {
	Status     string `json:"status"` // VALID, VALIDATED (already validated before), INVALID_TRANSACTION
	TranID     string `json:"tran_id"`
	ValID      string `json:"val_id"`
	Amount     string `json:"amount"`
	Currency   string `json:"currency"`
	BankTranID string `json:"bank_tran_id"`
	RiskLevel  string `json:"risk_level"` // "0" safe, "1" risky
}

func (v Validation) OK() bool { return v.Status == "VALID" || v.Status == "VALIDATED" }

// Validate asks SSLCommerz whether a payment really succeeded.
func (c *Client) Validate(ctx context.Context, valID string) (Validation, error) {
	q := url.Values{
		"val_id":       {valID},
		"store_id":     {c.cfg.StoreID},
		"store_passwd": {c.cfg.StorePassword},
		"v":            {"1"},
		"format":       {"json"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/validator/api/validationserverAPI.php?"+q.Encode(), nil)
	if err != nil {
		return Validation{}, err
	}
	var v Validation
	return v, c.do(req, &v)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sslcommerz: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(out)
}

// FormatTaka renders paisa as "1000.00".
func FormatTaka(paisa int64) string {
	return fmt.Sprintf("%d.%02d", paisa/100, paisa%100)
}

// ParseTaka parses an amount such as "1000", "1000.5" or "1000.00" into paisa.
func ParseTaka(s string) (int64, error) {
	whole, frac, _ := strings.Cut(strings.TrimSpace(s), ".")
	if whole == "" || len(frac) > 2 {
		return 0, errors.New("invalid amount")
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w < 0 {
		return 0, errors.New("invalid amount")
	}
	frac = (frac + "00")[:2]
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, errors.New("invalid amount")
	}
	return w*100 + f, nil
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Hostname() == "localhost" {
		return "example.com"
	}
	return u.Hostname()
}
