"use client";

import { RequireAuth } from "@/components/require-auth";
import { T } from "@/components/t";
import { Container } from "@/components/ui";
import { FamilyView } from "./family-view";

export default function FamilyPage() {
  return (
    <Container className="max-w-2xl py-8">
      <h1 className="mb-2 text-2xl font-semibold text-slate-900">
        <T>Family members</T>
      </h1>
      <p className="mb-6 text-sm text-slate-600">
        <T>Book serials and online consultations for your children, parents or spouse from your own account. Prescriptions are written in their name.</T>
      </p>
      <RequireAuth>
        {(user) =>
          user.role === "patient" ? (
            <FamilyView />
          ) : (
            <p className="text-sm text-slate-600">
              <T>Only patient accounts can add family members.</T>
            </p>
          )
        }
      </RequireAuth>
    </Container>
  );
}
