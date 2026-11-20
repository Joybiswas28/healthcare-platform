"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { DoctorFeed } from "./doctor-feed";
import { PatientConcerns } from "./patient-concerns";
import { T } from "@/components/t";

export default function ConcernsPage() {
  return (
    <Container className="max-w-3xl py-8">
      <RequireAuth>
        {(user) =>
          user.role === "patient" ? (
            <PatientConcerns />
          ) : user.role === "doctor" ? (
            <DoctorFeed />
          ) : (
            <p className="text-sm text-slate-600"><T>Concerns are for patients and doctors.</T></p>
          )
        }
      </RequireAuth>
    </Container>
  );
}
