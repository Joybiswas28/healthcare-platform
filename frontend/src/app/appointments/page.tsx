"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { AppointmentList } from "./appointment-list";
import { T } from "@/components/t";

export default function AppointmentsPage() {
  return (
    <Container className="py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900"><T>My appointments</T></h1>
      <RequireAuth>
        {(user) =>
          user.role === "patient" ? (
            <AppointmentList />
          ) : (
            <p className="text-sm text-slate-600"><T>Appointments here are for patient accounts.</T></p>
          )
        }
      </RequireAuth>
    </Container>
  );
}
