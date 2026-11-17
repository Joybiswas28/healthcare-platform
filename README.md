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
  appointment/        serial-based chamber booking (e-ticketing)
  server/             HTTP routes and middleware
  platform/
    config/           environment configuration
    database/         connection pool and migrations
    httpx/            JSON helpers
migrations/           SQL migration files
```

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
| POST | `/auth/register` | Register as a patient or doctor |
| POST | `/auth/login` | Log in with phone and password |
| GET | `/doctors?name=&specialization=&location=&limit=&offset=` | Search approved doctors |
| GET | `/doctors/{doctorID}` | Doctor profile with bookable chambers and schedules |
| GET | `/schedules/{scheduleID}/availability?date=` | Serials booked and remaining for a session |

**Any logged-in user**

| Method | Path | Description |
|---|---|---|
| GET | `/me` | Current user |

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

**Admin**

| Method | Path | Description |
|---|---|---|
| GET | `/admin/doctors?status=pending` | Doctors awaiting verification |
| POST | `/admin/doctors/{id}/verification` | `{"status": "approved"}` or `{"status": "rejected", "note": "..."}` |

### Booking rules

- Booking is serial-based. Each booking gets the next serial for that session and date. Serials are never reused after a cancellation.
- Only approved doctors with `booking_enabled` chambers can be booked. Bookings open up to 30 days in advance.
- A patient may hold only one booking per session. A session never takes more than `max_patients` bookings, even under concurrent requests.
- A doctor's sessions may not overlap on the same weekday, even across different chambers.
- Payment is not collected yet (Phase 2), so bookings are confirmed immediately.

Example:

```
curl -X POST localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Rahim","phone":"01712345678","password":"secret123","role":"patient"}'
```

Phone numbers can be sent as `01XXXXXXXXX`, `8801XXXXXXXXX` or `+8801XXXXXXXXX`. They are stored as `01XXXXXXXXX`.
Doctors start with `verification_status = pending` until an admin approves them.

## Roadmap

- [x] Phase 0: project skeleton, auth, roles
- [x] Phase 1: doctor profiles, chambers, schedules, e-ticket booking, admin verification
- [ ] Phase 1: private online consultation (chat + report upload)
- [ ] Phase 2: token wallet (double-entry ledger), payment gateway, public concerns, video calls
- [ ] Phase 3: hospitals
- [ ] Phase 4: pharmacy and delivery
