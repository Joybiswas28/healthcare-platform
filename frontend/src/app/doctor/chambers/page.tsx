"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { MyChambers } from "./my-chambers";

export default function DoctorChambersPage() {
  return (
    <Container className="max-w-3xl py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">My chambers</h1>
      <RequireAuth>
        {(user) => (user.role === "doctor" ? <MyChambers /> : <p className="text-sm text-slate-600">This page is for doctors.</p>)}
      </RequireAuth>
    </Container>
  );
}
