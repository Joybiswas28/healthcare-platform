"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { MyPharmacies } from "./my-pharmacies";

export default function ManagePharmaciesPage() {
  return (
    <Container className="max-w-3xl py-8">
      <RequireAuth>
        {(user) =>
          user.role === "pharmacy" ? <MyPharmacies /> : <p className="text-sm text-slate-600">This page is for pharmacy accounts.</p>
        }
      </RequireAuth>
    </Container>
  );
}
