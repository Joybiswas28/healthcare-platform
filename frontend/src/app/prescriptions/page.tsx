"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { PrescriptionList } from "./prescription-list";
import { T } from "@/components/t";

export default function PrescriptionsPage() {
  return (
    <Container className="max-w-3xl py-8">
      <RequireAuth>
        {(user) =>
          user.role === "patient" || user.role === "doctor" ? (
            <>
              <h1 className="mb-6 text-2xl font-semibold text-slate-900">
                <T>{user.role === "doctor" ? "Prescriptions I issued" : "My prescriptions"}</T>
              </h1>
              <PrescriptionList asDoctor={user.role === "doctor"} />
            </>
          ) : (
            <p className="text-sm text-slate-600"><T>Prescriptions are for patients and doctors.</T></p>
          )
        }
      </RequireAuth>
    </Container>
  );
}
