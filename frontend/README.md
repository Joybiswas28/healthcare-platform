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
| `/dashboard` | Role-based home |
| `/appointments` | Patient's serials and e-tickets; cancel upcoming ones |

## Scripts

| Command | |
|---|---|
| `npm run dev` | Development server |
| `npm run build` / `npm start` | Production build and server |
| `npm run lint` | ESLint |
| `npm run typecheck` | TypeScript only |
| `npm run gen:api` | Regenerate API types |

This project uses Next.js 16 with Cache Components enabled. Its documentation ships in `node_modules/next/dist/docs/`.
