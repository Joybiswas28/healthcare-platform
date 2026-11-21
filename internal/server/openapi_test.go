package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"

	"healthcare-platform/api"
	"healthcare-platform/internal/appointment"
	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/consultation"
	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/family"
	"healthcare-platform/internal/hospital"
	"healthcare-platform/internal/notify"
	"healthcare-platform/internal/payment/sslcommerz"
	"healthcare-platform/internal/pharmacy"
	"healthcare-platform/internal/prescription"
	"healthcare-platform/internal/wallet"
)

// fullRouter builds the real router with every optional route switched on. The
// handlers have no stores: the test only inspects routes, it never calls them.
func fullRouter() http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(Deps{
		Log:           log,
		Auth:          auth.NewHandler(nil, nil, log),
		Doctors:       doctor.NewHandler(nil, log),
		Family:        family.NewHandler(nil, log),
		Appointments:  appointment.NewHandler(nil, log),
		Wallet:        wallet.NewHandler(nil, nil, log),
		Consultations: consultation.NewHandler(nil, nil, log),
		Prescriptions: prescription.NewHandler(nil, "", log),
		Hospitals:     hospital.NewHandler(nil, nil, nil, log),
		Pharmacies:    pharmacy.NewHandler(nil, log),
		Notifications: notify.NewHandler(nil, log),
		SSLCommerz:    sslcommerz.NewCallbacks(nil, nil, "", log),
		AdminUsers:    auth.NewAdminHandler(nil, nil, log),
		DevRoutes:     true,
	})
}

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(api.Spec)
	if err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	return doc
}

func TestOpenAPISpecIsValid(t *testing.T) {
	if err := loadSpec(t).Validate(context.Background()); err != nil {
		t.Fatalf("openapi.yaml is not valid OpenAPI 3: %v", err)
	}
}

// TestOpenAPICoversEveryRoute fails when a route is added without documenting it,
// or when the documentation describes a route that does not exist.
func TestOpenAPICoversEveryRoute(t *testing.T) {
	router, ok := fullRouter().(chi.Routes)
	if !ok {
		t.Fatal("router does not expose its routes")
	}

	served := map[string]bool{}
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if route != "/" {
			route = strings.TrimSuffix(route, "/")
		}
		if route == "/openapi.yaml" || route == "/docs" {
			return nil // the documentation itself
		}
		served[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	documented := map[string]bool{}
	for path, item := range loadSpec(t).Paths.Map() {
		for method := range item.Operations() {
			documented[method+" "+path] = true
		}
	}

	var missing, stale []string
	for r := range served {
		if !documented[r] {
			missing = append(missing, r)
		}
	}
	for r := range documented {
		if !served[r] {
			stale = append(stale, r)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("routes missing from api/openapi.yaml:\n  %s", strings.Join(missing, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("api/openapi.yaml documents routes that do not exist:\n  %s", strings.Join(stale, "\n  "))
	}
	t.Logf("%d routes, all documented", len(served))
}

func TestDocsAreServed(t *testing.T) {
	h := fullRouter()
	for path, want := range map[string]string{"/openapi.yaml": "openapi: 3.0.3", "/docs": "swagger-ui-bundle.js"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("GET %s: %d, missing %q", path, rec.Code, want)
		}
	}
}
