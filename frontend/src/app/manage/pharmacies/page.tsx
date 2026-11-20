"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { MyPharmacies } from "./my-pharmacies";
import { T } from "@/components/t";

export default function ManagePharmaciesPage() {
  return (
    <Container className="max-w-3xl py-8">
      <RequireAuth>
        {(user) =>
          user.role === "pharmacy" ? <MyPharmacies /> : <p className="text-sm text-slate-600"><T>This page is for pharmacy accounts.</T></p>
        }
      </RequireAuth>
    </Container>
  );
}
