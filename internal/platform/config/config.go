// Package config loads application settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env         string // "development" | "production"
	HTTPAddr    string
	DatabaseURL string
	JWTSecret   string
	JWTTTL      time.Duration
	UploadDir   string

	// PublicBaseURL is where this API is reachable from outside (QR codes, payment callbacks).
	PublicBaseURL string
	// FrontendURL is where patients are sent back to after paying.
	FrontendURL string

	// SSLCommerz is enabled when a store id is set; otherwise payments are simulated.
	SSLCommerzStoreID       string
	SSLCommerzStorePassword string
	SSLCommerzSandbox       bool
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
