"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { AppointmentList } from "./appointment-list";

export default function AppointmentsPage() {
  return (
    <Container className="py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">My appointments</h1>
      <RequireAuth>
        {(user) =>
          user.role === "patient" ? (
            <AppointmentList />
          ) : (
            <p className="text-sm text-slate-600">Appointments here are for patient accounts.</p>
          )
        }
      </RequireAuth>
    </Container>
  );
}
