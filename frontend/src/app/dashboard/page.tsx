"use client";

import { RequireAuth } from "@/components/require-auth";
import { ButtonLink, Card, Container } from "@/components/ui";
import type { User } from "@/lib/api/client";
import { ROLE_LABELS } from "@/lib/format";

export default function DashboardPage() {
  return (
    <Container className="py-8">
      <RequireAuth>{(user) => <Dashboard user={user} />}</RequireAuth>
    </Container>
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

      {user.role === "patient" ? (
        <div className="grid gap-4 sm:grid-cols-2">
          <Card>
            <h2 className="font-semibold text-slate-900">My appointments</h2>
            <p className="mt-1 text-sm text-slate-600">See your serials and e-tickets, or cancel a booking.</p>
            <ButtonLink href="/appointments" className="mt-4">
              Open
            </ButtonLink>
          </Card>
          <Card>
            <h2 className="font-semibold text-slate-900">Book a doctor</h2>
            <p className="mt-1 text-sm text-slate-600">Search by name, specialty or district.</p>
            <ButtonLink href="/doctors" variant="secondary" className="mt-4">
              Find a doctor
            </ButtonLink>
          </Card>
        </div>
      ) : (
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
