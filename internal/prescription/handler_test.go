package prescription

import "testing"

// The QR code on a printed prescription must open the website's verify page,
// which pharmacies can read, not the API's JSON answer.
func TestVerifyURLPointsToWebsite(t *testing.T) {
	h := NewHandler(nil, "https://www.example.com/", nil)
	if got, want := h.verifyURL("MFP3B5E7R2"), "https://www.example.com/verify/MFP3B5E7R2"; got != want {
		t.Fatalf("verifyURL = %q, want %q", got, want)
	}
}
