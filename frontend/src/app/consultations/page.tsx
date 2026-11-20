"use client";

import { RequireAuth } from "@/components/require-auth";
import { ButtonLink, Container } from "@/components/ui";
import { ConsultationList } from "./consultation-list";
import { T } from "@/components/t";

export default function ConsultationsPage() {
  return (
    <Container className="py-8">
      <RequireAuth>
        {(user) =>
          user.role === "patient" || user.role === "doctor" ? (
            <>
              <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
                <h1 className="text-2xl font-semibold text-slate-900"><T>Online consultations</T></h1>
                {user.role === "patient" && (
                  <div className="flex gap-2">
                    <ButtonLink href="/concerns" variant="secondary">
                      <T>Post a concern</T>
                    </ButtonLink>
                    <ButtonLink href="/doctors?online=true">Find a doctor</ButtonLink>
                  </div>
                )}
                {user.role === "doctor" && (
                  <ButtonLink href="/concerns" variant="secondary">
                    <T>Patient concerns</T>
                  </ButtonLink>
                )}
              </div>
              <ConsultationList asDoctor={user.role === "doctor"} />
            </>
          ) : (
            <p className="text-sm text-slate-600"><T>Online consultations are for patients and doctors.</T></p>
          )
        }
      </RequireAuth>
    </Container>
  );
}
