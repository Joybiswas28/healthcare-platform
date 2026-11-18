"use client";

import { RequireAuth } from "@/components/require-auth";
import { ButtonLink, Container } from "@/components/ui";
import { ConsultationList } from "./consultation-list";

export default function ConsultationsPage() {
  return (
    <Container className="py-8">
      <RequireAuth>
        {(user) =>
          user.role === "patient" || user.role === "doctor" ? (
            <>
              <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
                <h1 className="text-2xl font-semibold text-slate-900">Online consultations</h1>
                {user.role === "patient" && (
                  <div className="flex gap-2">
                    <ButtonLink href="/concerns" variant="secondary">
                      Post a concern
                    </ButtonLink>
                    <ButtonLink href="/doctors?online=true">Find a doctor</ButtonLink>
                  </div>
                )}
                {user.role === "doctor" && (
                  <ButtonLink href="/concerns" variant="secondary">
                    Patient concerns
                  </ButtonLink>
                )}
              </div>
              <ConsultationList asDoctor={user.role === "doctor"} />
            </>
          ) : (
            <p className="text-sm text-slate-600">Online consultations are for patients and doctors.</p>
          )
        }
      </RequireAuth>
    </Container>
  );
}
