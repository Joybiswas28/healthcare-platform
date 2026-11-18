# Shasthya Seba — web frontend

Next.js (App Router, TypeScript, Tailwind) website for the Digital Healthcare Platform. It talks to the Go API in the repository root.

## Run it locally

1. Start the backend from the repository root (see the main README). By default it listens on `http://127.0.0.1:8080`.
2. In this folder:

   ```sh
   cp .env.example .env.local   # set API_URL if the backend runs somewhere else
   npm install
   npm run dev                  # http://localhost:3000
   ```

In development, SMS codes for sign-up and password reset are printed in the backend's log instead of being sent.

## How it talks to the API

The browser never calls the Go API directly and never sees the access token.

- `src/app/api/v1/[...path]/route.ts` forwards `/api/v1/*` to the same path on `API_URL`.
- On login and register it moves `access_token` out of the JSON into an `httpOnly`, `SameSite=Lax` cookie (`Secure` in production builds, so serve production over HTTPS). Every later request gets it back as `Authorization: Bearer …`.
- Any request other than GET/HEAD must carry a same-site `Origin` header, otherwise it gets 403.
- `POST /api/session/logout` clears the cookie.

Because the proxy keeps the API's paths, the typed client works unchanged.

## Typed API client

`src/lib/api/schema.d.ts` is generated from `../api/openapi.yaml`:

```sh
npm run gen:api
```

Run it whenever the backend's OpenAPI spec changes, then `npm run typecheck`. Use the client like this:

```ts
import { api } from "@/lib/api/client";
const { data, error } = await api.GET("/api/v1/doctors/{doctorID}", { params: { path: { doctorID } } });
```

## Pages

| Path | What it does |
|---|---|
| `/` | Landing page with doctor search |
| `/doctors` | Search verified doctors by name, specialty, district, online |
| `/doctors/[id]` | Profile, chambers and sessions; pick a date and book a serial |
| `/register` | Sign up with SMS code (patient, doctor, hospital admin, pharmacy, rider) |
| `/login`, `/forgot-password` | Log in; reset password with SMS code |
| `/dashboard` | Role-based home; doctors turn online consultations on and set their token fee |
| `/appointments` | Patient's serials and e-tickets; cancel upcoming ones |
| `/consultations` | Patient and doctor consultation lists, filtered by status |
| `/consultations/new?doctor=…` | Patient requests a consultation (linked from the doctor's profile) |
| `/consultations/[id]` | Accept / decline / pay / complete / cancel, plus chat and report sharing |
| `/concerns`, `/concerns/[id]` | Patients post anonymous concerns and accept doctors' offers; doctors browse and offer |
| `/patients` | Doctor's chamber list for a day: look up a ticket, mark seen / no-show, prescribe |
| `/prescriptions`, `/prescriptions/[id]` | Patient's and doctor's prescriptions; print / save as PDF; doctor revokes |
| `/prescriptions/new?consultation_id=…` or `?appointment_id=…&date=…` | Doctor writes a prescription |
| `/verify`, `/verify/[code]` | Public check for pharmacies (patient name masked) |
| `/hospitals`, `/hospitals/[id]` | Hospital directory (name, district, department, facility); hospital page with photos, doctors by department and bookable sessions |
| `/manage/hospitals` | Hospital admin: hospitals I manage; register a new one for verification |
| `/manage/hospitals/[id]` | Tabs: doctors & sessions (invite by BMDC, chambers, weekly sessions), bookings (cancel, reschedule), profile & photos, team |
| `/pharmacies`, `/pharmacies/[id]` | Verified pharmacies near the browser's location, with ratings and reviews |
| `/medicines/new` | Patient sends a platform prescription or a photo/PDF of a paper one to nearby pharmacies (delivery or pickup) |
| `/medicines`, `/medicines/requests/[id]`, `/medicines/orders/[id]` | Quotes (cheapest first), order, delivery code, rider location, timeline, cancel, rating |
| `/manage/pharmacies`, `/manage/pharmacies/[id]` | Pharmacy owner: register, new requests (no patient identity), orders, profile, commission statement |
| `/manage/pharmacies/[id]/requests/[id]`, `.../orders/[id]` | Send or revise a quote; prepare, ready, dispatch (own staff or platform rider), hand over with the patient's code |
| `/rider`, `/rider/orders/[id]` | Rider profile, on/off duty, jobs within 5 km, pickup and delivery with the code, earnings |
| `/admin` | Platform admins: verify doctors (and set their commission), hospitals, pharmacies and riders; pay or reject doctor withdrawals; find, suspend and reactivate accounts |
| `/wallet` | Token balance, lots and expiry, buying packages, history |
| `/payments/result` | Where SSLCommerz returns after paying (`FRONTEND_URL/payments/result`) |

Without SSLCommerz credentials the backend runs a development gateway: "Buy" asks for confirmation and marks the payment paid without taking money.

Chat messages are fetched every 3 seconds through the proxy. The backend's WebSocket needs the access token in its URL, and this app deliberately never gives the token to the browser.

## Scripts

| Command | |
|---|---|
| `npm run dev` | Development server |
| `npm run build` / `npm start` | Production build and server |
| `npm run lint` | ESLint |
| `npm run typecheck` | TypeScript only |
| `npm run gen:api` | Regenerate API types |

This project uses Next.js 16 with Cache Components enabled. Its documentation ships in `node_modules/next/dist/docs/`.
