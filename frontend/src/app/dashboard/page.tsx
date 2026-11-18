"use client";

import { RequireAuth } from "@/components/require-auth";
import { ButtonLink, Card, Container } from "@/components/ui";
import type { User } from "@/lib/api/client";
import { ROLE_LABELS } from "@/lib/format";
import { ConsultationSettings } from "./consultation-settings";

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
      <h2 className="font-semibold text-slate-900">{title}</h2>
      <p className="mt-1 text-sm text-slate-600">{body}</p>
      <ButtonLink href={href} variant={primary ? "primary" : "secondary"} className="mt-4">
        {cta}
      </ButtonLink>
    </Card>
  );
}

function Dashboard({ user }: { user: User }) {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-slate-900">Hello, {user.name}</h1>
        <p className="text-sm text-slate-600">
          {ROLE_LABELS[user.role ?? ""] ?? user.role} · {user.phone}
        </p>
      </div>

      {user.role === "patient" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <Tile title="My appointments" body="Your serials and e-tickets. Cancel a booking if plans change." href="/appointments" cta="Open" primary />
          <Tile title="Online consultations" body="Chat with a doctor and share reports from home." href="/consultations" cta="Open" primary />
          <Tile title="Prescriptions" body="Every prescription doctors wrote for you. Print or save as PDF." href="/prescriptions" cta="Open" />
          <Tile title="Ask doctors" body="Post a concern anonymously and choose from doctors' offers." href="/concerns" cta="My concerns" />
          <Tile title="Tokens" body="Online consultations are paid with tokens. See your balance or buy more." href="/wallet" cta="Wallet" />
          <Tile title="Book a doctor" body="Search by name, specialty or district." href="/doctors" cta="Find a doctor" />
        </div>
      )}

      {user.role === "doctor" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <ConsultationSettings />
          <Tile title="My patients" body="Today's chamber list in serial order. Look up a ticket, mark patients seen, prescribe." href="/patients" cta="Open" primary />
          <Tile title="Consultations" body="New requests, active chats and completed consultations." href="/consultations" cta="Open" primary />
          <Tile title="Prescriptions" body="Prescriptions you issued. Print or revoke them." href="/prescriptions" cta="Open" />
          <Tile title="Patient concerns" body="Anonymous questions from patients. Offer to help at your fee." href="/concerns" cta="Browse" />
        </div>
      )}

      {user.role !== "patient" && user.role !== "doctor" && (
        <Card>
          <h2 className="font-semibold text-slate-900">Your workspace is on its way</h2>
          <p className="mt-1 text-sm text-slate-600">
            Tools for {ROLE_LABELS[user.role ?? ""]?.toLowerCase() ?? "your"} accounts are being built. Your account
            is ready and you will see them here as soon as they launch.
          </p>
        </Card>
      )}
    </div>
  );
}
