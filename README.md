# Digital Healthcare Platform: Backend

A Go backend for a healthcare platform in Bangladesh. It connects patients, doctors, hospitals, pharmacies and delivery riders.

## Tech stack

- Go 1.26, `chi` router
- PostgreSQL with `pgx` and `goose` migrations (embedded and run automatically on startup)
- JWT authentication with role-based access control

## Project layout

```
cmd/api/              entry point (main.go)
cmd/createadmin/      CLI to create an admin account
internal/
  auth/               register, login, JWT, auth + role middleware
  user/               user model and repository
  doctor/             doctor profiles, chambers, schedules, search, admin verification
  appointment/        serial-based chamber booking (e-ticketing), hospital cancel/reschedule
  hospital/           hospital profiles, staff doctors, hospital chambers, photos, members
  pharmacy/           pharmacies, medicine requests, quotes, orders, riders, reviews
  wallet/             tokens, payments, double-entry ledger, escrow
  consultation/       online consultations, public concerns and offers, messages, uploads, WebSocket
  realtime/           in-process hub fed by PostgreSQL LISTEN/NOTIFY
  prescription/       digital prescriptions, printable page, public verification
  notify/             in-app notifications, SMS outbox and worker
  payment/sslcommerz/ SSLCommerz checkout, validation and callbacks
  server/             HTTP routes and middleware
  platform/
    config/           environment configuration
    database/         connection pool and migrations
    httpx/            JSON helpers
    storage/          file storage (local disk in development)
migrations/           SQL migration files
```

## Documentation

- **Interactive API docs:** run the server and open http://localhost:8080/docs (Swagger UI). Click **Authorize** and paste the `access_token` from login to try authenticated calls. The raw spec is at `/openapi.yaml` (source: [api/openapi.yaml](api/openapi.yaml)). Frontends can generate typed clients from it, for example `npx openapi-typescript http://localhost:8080/openapi.yaml -o api.d.ts`.
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md): putting the API on a server with HTTPS, backups and updates
- [docs/SECURITY-REVIEW.md](docs/SECURITY-REVIEW.md): what was checked, what was fixed, what is still recommended

## Running locally

1. Start PostgreSQL. With Docker:
   ```
   docker compose up -d
   ```
   Or install PostgreSQL yourself and create a database that matches `DATABASE_URL`.
2. Copy the env file and set `JWT_SECRET`:
   ```
   cp .env.example .env
   ```
3. Run the server:
   ```
   go run ./cmd/api
   ```
4. Run the tests. The integration tests need a database, and each test uses its own temporary schema, so the dev database is safe to use:
   ```
   # PowerShell
   $env:TEST_DATABASE_URL = "postgres://healthcare:healthcare@127.0.0.1:55432/healthcare?sslmode=disable"
   go test ./...
   ```
   Without `TEST_DATABASE_URL`, the integration tests are skipped.

## Creating an admin

Admins cannot self-register. Create one from the command line:

```
go run ./cmd/createadmin -name "Admin" -phone 01700000000 -password "a-strong-password"
```

## API (v1)

All paths below start with `/api/v1` unless noted. Money is always in **paisa** (BDT 1 = 100 paisa). Dates are `YYYY-MM-DD` and times are `HH:MM`, both in Bangladesh time. `weekday` runs from 0 (Sunday) to 6 (Saturday).

**Public**

| Method | Path | Description |
|---|---|---|
| GET | `/healthz`, `/readyz` (no prefix) | Liveness and database readiness |
| POST | `/auth/otp` | `{"phone", "purpose": "register"\|"reset_password"}` sends a 6-digit code by SMS |
| POST | `/auth/register` | `{"name", "phone", "password", "role", "otp_code"}`: register as a patient or doctor |
| POST | `/auth/login` | Log in with phone and password |
| POST | `/auth/password/reset` | `{"phone", "otp_code", "new_password"}` |
| GET | `/prescriptions/verify/{code}` | Check a prescription (for pharmacies). The patient's name is masked |
| POST | `/payments/sslcommerz/{ipn,success,fail,cancel}` | SSLCommerz callbacks (only when SSLCommerz is configured) |
| GET | `/doctors?name=&specialization=&location=&limit=&offset=` | Search approved doctors |
| GET | `/doctors/{doctorID}` | Doctor profile with bookable chambers and schedules |
| GET | `/schedules/{scheduleID}/availability?date=` | Serials booked and remaining for a session |

**Any logged-in user**

| Method | Path | Description |
|---|---|---|
| GET | `/me` | Current user |
| GET | `/notifications?unread=true` | My notifications and unread count |
| POST | `/notifications/{id}/read`, `/notifications/read-all` | Mark as read |

**Prescriptions**

| Method | Path | Who | Description |
|---|---|---|---|
| POST | `/prescriptions` | doctor | Issue for a `consultation_id` (active or completed) or an `appointment_id` (today or earlier). Includes `medicines` (`name`, `strength`, `form`, `dosage`, `duration`, `instructions`), `investigations`, `advice`, `follow_up_date` |
| GET | `/prescriptions?consultation_id=&appointment_id=` | both | Patient: issued to me. Doctor: issued by me |
| GET | `/prescriptions/{id}` | both | One prescription |
| GET | `/prescriptions/{id}/print` | both | Printable HTML page with a QR code that opens `FRONTEND_URL/verify/{code}`. Use the browser's "Save as PDF" |
| POST | `/prescriptions/{id}/revoke` | doctor | `{"reason": "..."}`. Prescriptions are never edited: revoke and issue a new one |

**Patient**

| Method | Path | Description |
|---|---|---|
| POST | `/appointments` | Book: `{"schedule_id": "...", "date": "2026-11-24"}` returns a serial and ticket code |
| GET | `/appointments` | My appointments |
| POST | `/appointments/{id}/cancel` | Cancel an upcoming appointment |

**Doctor**

| Method | Path | Description |
|---|---|---|
| GET / PUT | `/doctor/profile` | BMDC number, specialization, degrees, experience, bio |
| GET / POST | `/doctor/chambers` | List or add chambers |
| PUT | `/doctor/chambers/{id}` | Update a chamber (fee, address, `booking_enabled`) |
| POST | `/doctor/chambers/{id}/schedules` | Add a weekly session `{weekday, start_time, end_time, max_patients}` |
| DELETE | `/doctor/schedules/{id}` | Remove a session (existing bookings stay valid) |
| GET | `/doctor/appointments?date=` | Patient list for a day (defaults to today) |
| POST | `/doctor/appointments/{id}/complete` | Mark as seen |
| POST | `/doctor/appointments/{id}/no-show` | Mark as no-show |
| GET | `/doctor/tickets/{code}` | Look up a ticket shown at the chamber |

| PUT | `/doctor/consultation-settings` | `{"enabled": true, "fee_tokens": 2}` (verified doctors only) |
| GET | `/doctor/earnings` | Earnings balance and history |

**Tokens and wallet (patient)**

| Method | Path | Description |
|---|---|---|
| GET | `/token-packages` | Available packages (public) |
| POST | `/wallet/purchases` | `{"package_id": "..."}` starts a payment and returns `checkout_url` |
| GET | `/wallet/purchases/{id}` | Payment status |
| POST | `/dev/payments/{id}/succeed` | **Development only**: simulates the gateway confirming the payment |
| GET | `/wallet` | Tokens available, per lot with expiry |
| GET | `/wallet/history` | Token purchases, holds, refunds and expiries |

**Online consultation (patient and doctor)**

| Method | Path | Who | Description |
|---|---|---|---|
| POST | `/consultations` | patient | `{"doctor_id", "subject", "description"}` |
| GET | `/consultations?status=` | both | My consultations |
| GET | `/consultations/{id}` | both | One consultation |
| POST | `/consultations/{id}/accept` | doctor | Accept a request |
| POST | `/consultations/{id}/reject` | doctor | `{"reason": "..."}` |
| POST | `/consultations/{id}/confirm` | patient | Pay with tokens; the consultation becomes active |
| POST | `/consultations/{id}/complete` | doctor | Close and receive payment |
| POST | `/consultations/{id}/cancel` | both | Patient: before it starts. Doctor: while active (full refund) |
| GET | `/consultations/{id}/messages?after=` | both | Messages after a given id (poll for new ones) |
| POST | `/consultations/{id}/messages` | both | `{"body": "..."}` (active only) |
| POST | `/consultations/{id}/attachments` | both | Multipart field `file`: PDF, JPEG or PNG, max 10 MB |
| GET | `/attachments/{id}` | both | Download a shared file |
| GET (WebSocket) | `/consultations/{id}/ws?access_token=<jwt>&after=<last id>` | both | Live chat (see below) |

**Public medical concerns**

| Method | Path | Who | Description |
|---|---|---|---|
| POST | `/concerns` | patient | `{"title", "description", "specialization"}` (max 3 open at once) |
| GET | `/concerns` | both | Patient: my concerns. Doctor: open feed (`?specialization=`), never showing who posted |
| GET | `/concerns/{id}` | both | Patient: with all offers. Doctor: with their own offer |
| POST | `/concerns/{id}/close` | patient | Close; pending offers are declined |
| POST | `/concerns/{id}/offers` | doctor | `{"message": "..."}` at the doctor's current fee |
| POST | `/offers/{id}/withdraw` | doctor | Withdraw a pending offer (can be renewed later) |
| POST | `/offers/{id}/accept` | patient | Pays with tokens and starts an active consultation with that doctor |
| GET | `/doctor/offers` | doctor | My offers |

**Doctor withdrawals**

| Method | Path | Who | Description |
|---|---|---|---|
| POST | `/doctor/withdrawals` | doctor | `{"amount_paisa", "method": "bkash"\|"nagad"\|"bank", "account_details"}` (min BDT 500, one pending at a time) |
| GET | `/doctor/withdrawals` | doctor | My withdrawal requests |
| GET | `/admin/withdrawals?status=requested` | admin | Requests to process |
| POST | `/admin/withdrawals/{id}/process` | admin | `{"status": "paid", "payout_ref": "..."}` or `{"status": "rejected", "note": "..."}` |

### Payments

- Without SSLCommerz settings, purchases use a development simulator: open the returned `checkout_url` (`/api/v1/dev/payments/{id}/succeed`) to "pay".
- With `SSLCOMMERZ_STORE_ID` and `SSLCOMMERZ_STORE_PASSWORD` set, `checkout_url` is the SSLCommerz payment page (cards, bKash, Nagad and others). SSLCommerz must be able to reach `PUBLIC_BASE_URL` for callbacks. In local development, expose the server with a tunnel such as ngrok or cloudflared.
- Tokens are credited only after the SSLCommerz validation API confirms the payment and its amount. The posted callback form is never trusted.
- After paying, the patient is sent to `FRONTEND_URL/payments/result?status=success|pending|failed|cancelled&payment_id=...`.

### Notifications and SMS

- Events such as bookings, consultation updates, offers, prescriptions, verification and withdrawals create in-app notifications. Important ones are also sent by SMS.
- Notifications are written in the same transaction as the event. A background worker sends the SMS and retries up to 5 times.
- No SMS provider is connected yet, so SMS (including OTP codes) are written to the server log. In development, copy the OTP from there.
- OTP rules: codes expire after 5 minutes. A number can get one code per minute and five per hour. A code is burned after 5 wrong attempts.

### Live chat (WebSocket)

Connect to `/api/v1/consultations/{id}/ws?access_token=<jwt>&after=<last message id you have>`. Browsers cannot set headers on WebSockets, so the token goes in the query string on this route only.

- The server first sends every message after `after`, then pushes new ones as `{"type": "message", "message": {...}}`.
- Send `{"body": "..."}` to post a message. If it is rejected, you get `{"type": "error", "error": "..."}`.
- Messages sent through REST or file uploads also appear live.
- New messages are announced by a PostgreSQL trigger (`NOTIFY`), so this works with any number of API servers.

**Hospitals: public**

| Method | Path | Description |
|---|---|---|
| GET | `/hospitals?name=&location=&department=&facility=` | Search verified hospitals |
| GET | `/hospitals/{id}` | Hospital page: details, map link, photos, doctors with bookable chambers and schedules |
| GET | `/hospitals/{id}/photos/{photoID}` | Photo |

**Hospitals: management** (`hospital_admin` accounts. Register with `"role": "hospital_admin"`)

| Method | Path | Description |
|---|---|---|
| POST | `/hospitals` | Create a hospital (you become its owner). It stays hidden until an admin verifies it |
| GET | `/manage/hospitals` | Hospitals I manage |
| GET / PUT | `/manage/hospitals/{id}` | View or edit the profile (name, registration no, address, departments, facilities, visiting hours, latitude/longitude) |
| GET | `/manage/hospitals/{id}/photos` | List photos (also while verification is pending) |
| POST | `/manage/hospitals/{id}/photos` | Upload a photo (multipart `file`: JPEG/PNG/WebP, max 5 MB, max 10). The first one becomes the logo |
| POST / DELETE | `/manage/hospitals/{id}/photos/{photoID}/logo`, `/photos/{photoID}` | Set logo, delete photo |
| GET / POST | `/manage/hospitals/{id}/doctors` | List doctors. Invite one: `{"bmdc_reg_no", "department", "designation"}` |
| PUT / DELETE | `/manage/hospitals/{id}/doctors/{doctorID}` | Change department/designation. Remove the doctor (returns upcoming bookings to handle) |
| POST | `/manage/hospitals/{id}/doctors/{doctorID}/chambers` | Open a chamber for a doctor on staff: `{"fee_paisa", "room"}` |
| GET / PUT | `/manage/hospitals/{id}/chambers`, `/chambers/{chamberID}` | List or update (fee, room, `booking_enabled`) |
| POST / DELETE | `/manage/hospitals/{id}/chambers/{chamberID}/schedules`, `/schedules/{scheduleID}` | Weekly sessions |
| GET | `/manage/hospitals/{id}/appointments?date=&doctor_id=` | Bookings for a day |
| POST | `/manage/hospitals/{id}/appointments/{aid}/cancel` | `{"reason"}`. The patient gets an SMS |
| POST | `/manage/hospitals/{id}/appointments/{aid}/reschedule` | `{"schedule_id", "date"}`: same doctor, this hospital. The patient gets a new serial and ticket by SMS |
| GET / POST / DELETE | `/manage/hospitals/{id}/members[/{userID}]` | Staff accounts (owner only adds or removes). Add by `{"phone"}` of a `hospital_admin` account |

**Hospitals: doctor side**

| Method | Path | Description |
|---|---|---|
| GET | `/doctor/hospitals` | Invitations and hospitals I work at |
| POST | `/doctor/hospitals/{id}/accept`, `/decline`, `/leave` | Respond to an invitation, or leave |

### How hospitals fit in

- A hospital chamber is an ordinary chamber run by the hospital. Bookings, serials, tickets and prescriptions work exactly as for private chambers, and a doctor's sessions never overlap across their own chambers and any hospitals.
- Hospitals can only add verified doctors, and only after the doctor accepts. Hospitals set the department and designation, but never the doctor's professional profile.
- Doctors cannot edit hospital chambers or schedules. Hospitals cannot touch a doctor's private chambers.
- A hospital chamber takes bookings only while the hospital is verified, the doctor is on staff and booking is on. Removing a doctor turns booking off but keeps existing bookings, so the hospital can cancel or reschedule them.

**Pharmacy: patient**

| Method | Path | Description |
|---|---|---|
| GET | `/pharmacies?latitude=&longitude=&radius_km=` | Verified pharmacies nearby, nearest first (public) |
| GET | `/pharmacies/{id}` | Pharmacy with rating and reviews (public) |
| POST | `/medicine-requests` | Ask nearby pharmacies for medicines. JSON with `prescription_id` (a platform prescription), or multipart with a `data` field (the JSON) followed by a `file` field (scanned prescription: PDF/JPEG/PNG, max 10 MB). Also `fulfilment` (`delivery`/`pickup`), `address`, `latitude`, `longitude`, optional `items`, `note`, `contact_phone` |
| GET | `/medicine-requests[/{id}]` | My requests. One request includes all quotes, cheapest first |
| POST | `/medicine-requests/{id}/cancel` | Cancel an open request |
| POST | `/quotes/{id}/accept` | Choose a pharmacy. Creates the order and returns your 6-digit **delivery code** |
| GET | `/medicine-orders[/{id}]` | Orders with status timeline and live rider location |
| POST | `/medicine-orders/{id}/cancel` | Before the order is ready |
| POST | `/medicine-orders/{id}/review` | `{"rating": 1-5, "comment"}` after delivery |

**Pharmacy: owner** (`pharmacy` accounts)

| Method | Path | Description |
|---|---|---|
| POST | `/pharmacies` | Register a pharmacy (DGDA drug licence, address, location, delivery radius) |
| GET / PUT | `/manage/pharmacies[/{id}]` | My pharmacies, edit (including `accepting_orders`) |
| GET | `/manage/pharmacies/{id}/requests[/{rid}]` | Open requests nearby, with distance only (no patient identity or address) |
| GET | `/manage/pharmacies/{id}/requests/{rid}/attachment` | The uploaded prescription |
| POST | `/manage/pharmacies/{id}/requests/{rid}/quote` | `{"items": [{name, strength, quantity, available, price_paisa}], "delivery_charge_paisa", "eta_minutes", "note"}`. Can be revised while pending |
| POST | `/manage/pharmacies/{id}/quotes/{qid}/withdraw` | Withdraw a quote |
| GET | `/manage/pharmacies/{id}/orders?status=active` | Orders |
| POST | `/manage/pharmacies/{id}/orders/{oid}/preparing`, `/ready` | Progress |
| POST | `/manage/pharmacies/{id}/orders/{oid}/dispatch` | `{"mode": "pharmacy"}` (own staff) or `{"mode": "rider"}` (platform riders) |
| POST | `/manage/pharmacies/{id}/orders/{oid}/complete` | `{"code"}`: hand over a pickup order, or the pharmacy's own delivery |
| POST | `/manage/pharmacies/{id}/orders/{oid}/cancel` | `{"reason"}` before it leaves the shop |
| GET | `/manage/pharmacies/{id}/statement?from=&to=` | Delivered orders and platform commission due |

**Riders** (`rider` accounts)

| Method | Path | Description |
|---|---|---|
| GET / PUT | `/rider/profile` | `{"nid_number", "vehicle"}`. Verified by an admin |
| PUT | `/rider/availability` | `{"available", "latitude", "longitude"}` |
| PUT | `/rider/location` | Send position periodically while on duty (shown to the patient) |
| GET | `/rider/jobs` | Deliveries waiting at pharmacies within 5 km |
| POST | `/rider/jobs/{orderID}/accept` | Take a job (one at a time) |
| POST | `/rider/orders/{id}/release`, `/picked-up`, `/delivered` | `delivered` needs `{"code"}` from the patient |
| GET | `/rider/orders`, `/rider/earnings` | History and earnings |

### How medicine orders work

```
request (open 24 h) ──quotes──> patient accepts one ──> order: confirmed → preparing → ready
   pickup:            ready ──patient shows code──> delivered
   pharmacy delivery: ready → out_for_delivery ──code──> delivered
   rider delivery:    ready → rider_assigned → out_for_delivery ──code──> delivered
```

- Only verified pharmacies that are taking orders, within their delivery radius (and delivering, for delivery requests), see a request.
- Pharmacies can mark a prescribed medicine unavailable, but cannot substitute another one.
- The handover needs the patient's 6-digit code. Pharmacies and riders never see it. After 5 wrong codes the order is locked for support.
- Payment is cash on delivery for now. The platform commission (`PHARMACY_COMMISSION_BPS`, default 3% of the medicine total) is recorded on each delivered order and summed in the pharmacy statement. Riders earn the delivery charge.

**Admin**

| Method | Path | Description |
|---|---|---|
| GET / POST | `/admin/pharmacies[/{id}/verification]` | List, approve or reject pharmacies |
| GET / POST | `/admin/riders[/{userID}/verification]` | List, approve or reject riders |
| POST | `/admin/users/{id}/status` | `{"status": "suspended"}` or `active`. Suspending signs the user out everywhere |
| GET | `/admin/hospitals?status=pending` | Hospitals awaiting verification |
| POST | `/admin/hospitals/{id}/verification` | `{"status": "approved"}` or `{"status": "rejected", "note": "..."}` |
| GET | `/admin/doctors?status=pending` | Doctors awaiting verification |
| POST | `/admin/doctors/{id}/verification` | `{"status": "approved"}` or `{"status": "rejected", "note": "..."}` |
| PUT | `/admin/doctors/{id}/commission` | `{"commission_bps": 3333}`: platform share in basis points (5000 = 50%) |

### Tokens and money

- 1 token = BDT 100. The standard package is 10 tokens for BDT 1,000, valid for 365 days.
- Tokens are spent from the batch that expires first. Expired tokens are swept every hour and booked as platform revenue.
- Confirming a consultation moves its fee into escrow. Completing it splits the fee between doctor and platform by the doctor's commission rate. A doctor cancelling an active consultation refunds the patient in full, back to the same batches.
- Every money movement is recorded in a double-entry ledger in paisa: each transaction sums to zero, and account balances can be rebuilt from their entries.
- The fee and commission are fixed when the patient sends the request, so later price changes don't affect it.

### Consultation lifecycle

```
requested ──accept──> accepted ──confirm (pay)──> active ──complete──> completed
    │                     │                          │
    ├─reject──> rejected  └─patient cancel─┐         └─doctor cancel (refund)─┐
    └─patient cancel──────────────────────> cancelled <───────────────────────┘
```

### Booking rules

- Booking is serial-based. Each booking gets the next serial for that session and date. Serials are never reused after a cancellation.
- Only approved doctors with `booking_enabled` chambers can be booked. Bookings open up to 30 days in advance.
- A patient may hold only one booking per session. A session never takes more than `max_patients` bookings, even under concurrent requests.
- A doctor's sessions may not overlap on the same weekday, even across different chambers.
- Payment is not collected yet (Phase 2), so bookings are confirmed immediately.

Example:

```
# 1. Get a code (in development it appears in the server log)
curl -X POST localhost:8080/api/v1/auth/otp \
  -H "Content-Type: application/json" \
  -d '{"phone":"01712345678","purpose":"register"}'

# 2. Register with it
curl -X POST localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Rahim","phone":"01712345678","password":"secret123","role":"patient","otp_code":"123456"}'
```

Phone numbers can be sent as `01XXXXXXXXX`, `8801XXXXXXXXX` or `+8801XXXXXXXXX`. They are stored as `01XXXXXXXXX`.
Doctors start with `verification_status = pending` until an admin approves them.

## Roadmap

- [x] Phase 0: project skeleton, auth, roles
- [x] Phase 1: doctor profiles, chambers, schedules, e-ticket booking, admin verification
- [x] Phase 2A: token wallet (double-entry ledger), private online consultation (chat + report upload), simulated payments
- [x] Phase 2B: public medical concerns and doctor offers, doctor withdrawals, live chat over WebSocket
- [x] Phase 2C: digital prescriptions, notifications and SMS outbox, OTP registration and password reset, SSLCommerz integration
- [ ] Before launch: SMS provider account, SSLCommerz merchant account (test in sandbox), S3 storage, video calls
- [x] Phase 3: hospitals (profiles, verification, doctor invitations, hospital chambers, booking management, photos, staff accounts)
- [ ] Billing: subscription packages for doctors and hospitals
- [x] Phase 4: pharmacies, medicine requests and quotes, orders, riders and delivery tracking, reviews, commission statements
- [ ] Later: online payment for medicine orders, cash settlement between riders and pharmacies, pharmacy inventory
