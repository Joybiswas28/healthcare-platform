"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { EarningsView } from "./earnings-view";

export default function DoctorEarningsPage() {
  return (
    <Container className="max-w-3xl py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">Earnings</h1>
      <RequireAuth>
        {(user) => (user.role === "doctor" ? <EarningsView /> : <p className="text-sm text-slate-600">This page is for doctors.</p>)}
      </RequireAuth>
    </Container>
  );
}
