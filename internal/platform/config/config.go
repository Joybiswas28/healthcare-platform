// Package config loads application settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env         string // "development" | "production"
	HTTPAddr    string
	DatabaseURL string
	JWTSecret   string
	JWTTTL      time.Duration
	UploadDir   string

	// PublicBaseURL is where this API is reachable from outside (payment callbacks).
	PublicBaseURL string
	// FrontendURL is the website: patients return there after paying, and
	// prescription QR codes open its /verify/{code} page.
	FrontendURL string

	// SSLCommerz is enabled when a store id is set; otherwise payments are simulated.
	SSLCommerzStoreID       string
	SSLCommerzStorePassword string
	SSLCommerzSandbox       bool

	// TURN server for video calls (comma-separated turn:/turns: URLs and one
	// static username/credential). Without it calls use a public STUN server,
	// which works on most but not all networks.
	TURNURLs       []string
	TURNUsername   string
	TURNCredential string

	// PharmacyCommissionBps is the platform's share of the medicine total on each
	// delivered pharmacy order, in basis points (300 = 3%).
	PharmacyCommissionBps int

	// TrustProxy: read the client IP from proxy headers (only behind a reverse proxy).
	TrustProxy bool
	// CORSOrigins: comma-separated browser origins allowed to call the API.
	CORSOrigins []string
}

func Load() (Config, error) {
	cfg := Config{
		Env:                     getEnv("APP_ENV", "development"),
		HTTPAddr:                getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		JWTSecret:               os.Getenv("JWT_SECRET"),
		UploadDir:               getEnv("UPLOAD_DIR", "./data/uploads"),
		PublicBaseURL:           getEnv("PUBLIC_BASE_URL", "http://localhost:8080"),
		FrontendURL:             getEnv("FRONTEND_URL", "http://localhost:3000"),
		SSLCommerzStoreID:       os.Getenv("SSLCOMMERZ_STORE_ID"),
		SSLCommerzStorePassword: os.Getenv("SSLCOMMERZ_STORE_PASSWORD"),
		SSLCommerzSandbox:       getEnv("SSLCOMMERZ_SANDBOX", "true") == "true",
	}
	bps, err := strconv.Atoi(getEnv("PHARMACY_COMMISSION_BPS", "300"))
	if err != nil || bps < 0 || bps > 10000 {
		return Config{}, errors.New("PHARMACY_COMMISSION_BPS must be between 0 and 10000 (300 = 3%)")
	}
	cfg.PharmacyCommissionBps = bps

	cfg.TrustProxy = getEnv("TRUST_PROXY", "false") == "true"
	for _, u := range strings.Split(os.Getenv("TURN_URLS"), ",") {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if !strings.HasPrefix(u, "turn:") && !strings.HasPrefix(u, "turns:") {
			return Config{}, fmt.Errorf("TURN_URLS: %q must start with turn: or turns:", u)
		}
		cfg.TURNURLs = append(cfg.TURNURLs, u)
	}
	cfg.TURNUsername = os.Getenv("TURN_USERNAME")
	cfg.TURNCredential = os.Getenv("TURN_CREDENTIAL")
	if len(cfg.TURNURLs) > 0 && (cfg.TURNUsername == "" || cfg.TURNCredential == "") {
		return Config{}, errors.New("TURN_URLS needs TURN_USERNAME and TURN_CREDENTIAL")
	}
	for _, o := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if o == "" {
			continue
		}
		if o == "*" {
			return Config{}, errors.New("CORS_ALLOWED_ORIGINS must list exact origins, not *")
		}
		if !strings.HasPrefix(o, "https://") && !strings.HasPrefix(o, "http://") {
			return Config{}, fmt.Errorf("CORS_ALLOWED_ORIGINS: %q must start with http:// or https://", o)
		}
		cfg.CORSOrigins = append(cfg.CORSOrigins, o)
	}

	if cfg.IsProduction() {
		if strings.Contains(cfg.JWTSecret, "change-me") {
			return Config{}, errors.New("JWT_SECRET still has the example value; generate a random secret")
		}
		if !strings.HasPrefix(cfg.PublicBaseURL, "https://") {
			return Config{}, errors.New("PUBLIC_BASE_URL must use https:// in production")
		}
		if !strings.HasPrefix(cfg.FrontendURL, "https://") {
			return Config{}, errors.New("FRONTEND_URL must use https:// in production (it is printed in prescription QR codes)")
		}
		if strings.Contains(cfg.DatabaseURL, "sslmode=disable") && !strings.Contains(cfg.DatabaseURL, "@db:") &&
			!strings.Contains(cfg.DatabaseURL, "@localhost") && !strings.Contains(cfg.DatabaseURL, "@127.0.0.1") {
			return Config{}, errors.New("DATABASE_URL disables TLS to a remote database; use sslmode=require")
		}
	}
	if cfg.SSLCommerzStoreID != "" && cfg.SSLCommerzStorePassword == "" {
		return Config{}, errors.New("SSLCOMMERZ_STORE_PASSWORD is required when SSLCOMMERZ_STORE_ID is set")
	}

	ttlMinutes, err := strconv.Atoi(getEnv("JWT_TTL_MINUTES", "60"))
	if err != nil || ttlMinutes <= 0 {
		return Config{}, fmt.Errorf("JWT_TTL_MINUTES must be a positive integer")
	}
	cfg.JWTTTL = time.Duration(ttlMinutes) * time.Minute

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, errors.New("JWT_SECRET is required and must be at least 32 characters")
	}
	return cfg, nil
}

func (c Config) IsProduction() bool { return c.Env == "production" }

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
