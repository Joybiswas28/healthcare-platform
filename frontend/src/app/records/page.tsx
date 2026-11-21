"use client";

import { RequireAuth } from "@/components/require-auth";
import { T } from "@/components/t";
import { Container } from "@/components/ui";
import { RecordsView } from "./records-view";

export default function RecordsPage() {
  return (
    <Container className="max-w-3xl py-8">
      <h1 className="mb-2 text-2xl font-semibold text-slate-900">
        <T>Health records</T>
      </h1>
      <p className="mb-6 text-sm text-slate-600">
        <T>Keep your reports, X-rays and old prescriptions in one place. Only you can see them until you share one with a doctor.</T>
      </p>
      <RequireAuth>
        {(user) =>
          user.role === "patient" ? (
            <RecordsView me={user.name ?? ""} />
          ) : (
            <p className="text-sm text-slate-600">
              <T>Only patient accounts keep health records.</T>
            </p>
          )
        }
      </RequireAuth>
    </Container>
  );
}
