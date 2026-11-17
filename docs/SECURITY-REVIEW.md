# Security review

Date: 17 November 2026. Scope: the whole backend after Phase 4 (auth, doctors, appointments, consultations, wallet, payments, prescriptions, hospitals, pharmacy, delivery) and its dependencies.

## Fixed in this review

| # | Finding | Risk | Fix |
|---|---|---|---|
| 1 | No rate limit on login, registration, OTP or prescription verification | Password guessing, OTP spam, code enumeration | 20 requests/min per IP on these endpoints; 600/min per IP overall |
| 2 | Tokens stayed valid after a password reset, and accounts could not be suspended | A stolen token kept working; abusive accounts could not be stopped | Tokens carry a version. A password reset or suspension bumps it, so old tokens stop working within 30 s. Admins can suspend accounts (`POST /admin/users/{id}/status`) |
| 3 | In production, SMS text (including OTP codes) would have gone to the server log | Anyone with log access could take over accounts | Production logs never contain SMS text |
| 4 | Client IP taken from `X-Forwarded-For` on every request | Rate limits could be bypassed by faking the header | Proxy headers are used only with `TRUST_PROXY=true`. Caddy overwrites them |
| 5 | No CORS policy | A browser frontend could not call the API at all | Exact allowed origins from `CORS_ALLOWED_ORIGINS` (no wildcard) |
| 6 | Login answered faster for unknown phone numbers | Revealed which numbers have accounts | Unknown numbers run the same bcrypt check |
| 7 | A patient could hold unlimited free bookings | One account could block every serial of a doctor | Max 5 upcoming appointments per patient |
| 8 | User-uploaded PDFs were served without a sandbox | A crafted PDF could run script in the API's origin | `Content-Security-Policy: sandbox` on all user uploads |
| 9 | No security headers | Clickjacking, MIME sniffing | `X-Frame-Options`, `X-Content-Type-Options`, `Referrer-Policy`, HSTS behind HTTPS |
| 10 | Go 1.26.4 standard library had 21 known vulnerabilities reachable from our code (TLS, HTTP) | Remote attacks on the HTTP server and outgoing payment calls | Toolchain pinned to Go 1.26.11 in `go.mod` and the Dockerfile |
| 11 | Production could start with the example JWT secret, plain-HTTP public URL or unencrypted remote database | Forged tokens, broken payment callbacks, data exposed in transit | The server refuses to start in production with these settings |
| 12 | 15 s read timeout | Large uploads failed on slow mobile networks | 2 min server timeouts. Ordinary requests still time out at 30 s |

## Checked and found sound

- **SQL injection:** every query uses parameters. The only string-built SQL comes from constants in the code.
- **Authorization:** every resource is scoped to its owner (patient, doctor, hospital member, pharmacy owner, rider), and tests cover cross-account access.
- **Money:** double-entry ledger with invariants checked in tests. Payments are credited only after SSLCommerz's validation API confirms the amount, and crediting is idempotent. Concurrent spending is safe through row locks.
- **Uploads:** size limits, type detected from content, random storage keys, and downloads restricted to participants.
- **OTP:** HMAC-stored codes, 5-minute expiry, rate limits, and burned after 5 wrong attempts.
- **Delivery codes:** shown only to the patient, constant-time comparison, and locked after 5 wrong attempts.
- **Patient privacy:** pharmacies see only the distance to a request. Doctors browsing concerns never see who posted them. Prescription verification masks the patient's name.

## Remaining recommendations (not blocking a pilot)

1. **SMS provider:** without one, nobody can receive an OTP in production, so registration is impossible.
2. **Re-verification on name changes:** a verified hospital or pharmacy can rename itself without review. Consider sending name changes back to admin review.
3. **Refresh tokens:** access tokens last 60 minutes and users then log in again. Add refresh tokens when the frontend needs longer sessions.
4. **Encryption at rest for medical files:** use S3-compatible storage with server-side encryption, or encrypted disks, before storing real patient reports at scale.
5. **Audit log:** record admin actions (verifications, suspensions, withdrawals) in a dedicated table.
6. **Rate limits are per server:** with several API servers, move them to Redis.
7. **Legal:** telemedicine practice, prescription rules (DGDA), data protection and e-commerce rules need review by a Bangladeshi lawyer before launch.
