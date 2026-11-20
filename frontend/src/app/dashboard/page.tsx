"use client";

import { RequireAuth } from "@/components/require-auth";
import { T } from "@/components/t";
import { ButtonLink, Card, Container } from "@/components/ui";
import type { User } from "@/lib/api/client";
import { ROLE_LABELS } from "@/lib/format";
import { ConsultationSettings } from "./consultation-settings";
import { DoctorHospitals } from "./doctor-hospitals";
import { DoctorProfileNudge } from "./doctor-profile-nudge";

export default function DashboardPage() {
  return (
    <Container className="py-8">
      <RequireAuth>{(user) => <Dashboard user={user} />}</RequireAuth>
    </Container>
  );
}

function Tile({ title, body, href, cta, primary }: { title: string; body: string; href: string; cta: string; primary?: boolean }) {
  return (
    <Card>
      <h2 className="font-semibold text-slate-900">
        <T>{title}</T>
      </h2>
      <p className="mt-1 text-sm text-slate-600">
        <T>{body}</T>
      </p>
      <ButtonLink href={href} variant={primary ? "primary" : "secondary"} className="mt-4">
        {cta}
      </ButtonLink>
    </Card>
  );
}

function Dashboard({ user }: { user: User }) {
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900"><T vars={{ name: user.name }}>{"Hello, {name}"}</T></h1>
          <p className="text-sm text-slate-600">
            <T>{ROLE_LABELS[user.role ?? ""] ?? user.role ?? ""}</T> · {user.phone}
          </p>
        </div>
        <ButtonLink href="/settings" variant="secondary">
          <span aria-hidden>⚙️</span>&nbsp;<T>Account settings</T>
        </ButtonLink>
      </div>

      {user.role === "patient" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Tile title="My appointments" body="Your serials and e-tickets. Cancel a booking if plans change." href="/appointments" cta="Open" primary />
          <Tile title="Online consultations" body="Chat with a doctor and share reports from home." href="/consultations" cta="Open" primary />
          <Tile title="Prescriptions" body="Every prescription doctors wrote for you. Print or save as PDF." href="/prescriptions" cta="Open" />
          <Tile title="Medicines" body="Send a prescription to nearby pharmacies, compare prices, get it delivered." href="/medicines" cta="Open" primary />
          <Tile title="Ask doctors" body="Post a concern anonymously and choose from doctors' offers." href="/concerns" cta="My concerns" />
          <Tile title="Tokens" body="Online consultations are paid with tokens. See your balance or buy more." href="/wallet" cta="Wallet" />
          <Tile title="Book a doctor" body="Search by name, specialty or district." href="/doctors" cta="Find a doctor" />
        </div>
      )}

      {user.role === "doctor" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <DoctorProfileNudge />
          <Tile title="My chambers" body="Your private chambers and weekly sessions that patients book." href="/doctor/chambers" cta="Manage" primary />
          <ConsultationSettings />
          <Tile title="Earnings" body="Online consultation earnings and withdrawals to bKash, Nagad or bank." href="/doctor/earnings" cta="Open" />
          <Tile title="Professional profile" body="BMDC number, specialty, degrees and the bio patients see." href="/doctor/profile" cta="Edit" />
          <Tile title="Reviews" body="What patients say after a visit or consultation. Reply to thank them or explain." href="/doctor/reviews" cta="Open" />
          <Tile title="My patients" body="Today's chamber list in serial order. Look up a ticket, mark patients seen, prescribe." href="/patients" cta="Open" primary />
          <Tile title="Consultations" body="New requests, active chats and completed consultations." href="/consultations" cta="Open" primary />
          <Tile title="Prescriptions" body="Prescriptions you issued. Print or revoke them." href="/prescriptions" cta="Open" />
          <Tile title="Patient concerns" body="Anonymous questions from patients. Offer to help at your fee." href="/concerns" cta="Browse" />
          <DoctorHospitals />
        </div>
      )}

      {user.role === "hospital_admin" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Tile
            title="My hospitals"
            body="Register your hospital, invite doctors, set up chambers and sessions, and manage bookings."
            href="/manage/hospitals"
            cta="Open"
            primary
          />
        </div>
      )}

      {user.role === "pharmacy" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Tile
            title="My pharmacies"
            body="Register your pharmacy, quote on nearby requests, prepare and dispatch orders."
            href="/manage/pharmacies"
            cta="Open"
            primary
          />
        </div>
      )}

      {user.role === "rider" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Tile title="Deliveries" body="Go on duty, take deliveries near you, and track your earnings." href="/rider" cta="Open" primary />
        </div>
      )}

      {user.role === "admin" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Tile
            title="Admin panel"
            body="Verify doctors, hospitals, pharmacies and riders; pay doctor withdrawals; suspend accounts."
            href="/admin"
            cta="Open"
            primary
          />
        </div>
      )}
    </div>
  );
}
