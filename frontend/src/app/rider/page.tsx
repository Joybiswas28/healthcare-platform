"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { RiderHome } from "./rider-home";
import { T } from "@/components/t";

export default function RiderPage() {
  return (
    <Container className="max-w-3xl py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900"><T>Deliveries</T></h1>
      <RequireAuth>
        {(user) => (user.role === "rider" ? <RiderHome /> : <p className="text-sm text-slate-600"><T>This page is for rider accounts.</T></p>)}
      </RequireAuth>
    </Container>
  );
}
