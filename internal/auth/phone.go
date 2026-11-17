package auth

import (
	"errors"
	"regexp"
	"strings"
)

var bdMobile = regexp.MustCompile(`^01[3-9][0-9]{8}$`)

var ErrInvalidPhone = errors.New("invalid Bangladeshi mobile number")

// NormalizePhone converts common Bangladeshi mobile formats
// (01XXXXXXXXX, 8801XXXXXXXXX, +8801XXXXXXXXX) to the 11-digit local form.
func NormalizePhone(raw string) (string, error) {
	p := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(raw))
	p = strings.TrimPrefix(p, "+")
	if strings.HasPrefix(p, "880") {
		p = p[2:] // "8801..." -> "01..."
	}
	if !bdMobile.MatchString(p) {
		return "", ErrInvalidPhone
	}
	return p, nil
}
