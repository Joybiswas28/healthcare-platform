# Digital Healthcare Platform: Backend

A Go backend for a healthcare platform in Bangladesh. It connects patients, doctors, hospitals, pharmacies and delivery riders.

## Tech stack

- Go 1.26, `chi` router
- PostgreSQL with `pgx` and `goose` migrations (embedded and run automatically on startup)
- JWT authentication with role-based access control

## Project layout

```
cmd/api/              entry point (main.go)
internal/
  auth/               register, login, JWT, auth + role middleware
  user/               user model and repository
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
4. Run the tests:
   ```
   go test ./...
   ```

## API (v1)

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/healthz` | none | Liveness check |
| GET | `/readyz` | none | Readiness check (pings the database) |
| POST | `/api/v1/auth/register` | none | Register as a patient or doctor |
| POST | `/api/v1/auth/login` | none | Log in with phone and password |
| GET | `/api/v1/me` | Bearer | Current user |

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
- [ ] Phase 1: doctor profiles, chambers, schedules, e-ticket booking, admin verification
- [ ] Phase 1: private online consultation (chat + report upload)
- [ ] Phase 2: token wallet (double-entry ledger), payment gateway, public concerns, video calls
- [ ] Phase 3: hospitals
- [ ] Phase 4: pharmacy and delivery
