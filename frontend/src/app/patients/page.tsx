"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { PatientDay } from "./patient-day";

// The doctor's chamber list for a day: who booked, in serial order.
export default function PatientsPage() {
  return (
    <Container className="py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">My patients</h1>
      <RequireAuth>
        {(user) =>
          user.role === "doctor" ? <PatientDay /> : <p className="text-sm text-slate-600">This page is for doctors.</p>
        }
      </RequireAuth>
    </Container>
  );
}
