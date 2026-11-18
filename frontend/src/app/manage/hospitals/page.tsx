"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { MyHospitals } from "./my-hospitals";

export default function ManageHospitalsPage() {
  return (
    <Container className="max-w-3xl py-8">
      <RequireAuth>
        {(user) =>
          user.role === "hospital_admin" ? (
            <MyHospitals />
          ) : (
            <p className="text-sm text-slate-600">This page is for hospital administrator accounts.</p>
          )
        }
      </RequireAuth>
    </Container>
  );
}
