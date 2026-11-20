"use client";

import { RequireAuth } from "@/components/require-auth";
import { ButtonLink, Container } from "@/components/ui";
import { MyMedicines } from "./my-medicines";
import { T } from "@/components/t";

export default function MedicinesPage() {
  return (
    <Container className="max-w-3xl py-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold text-slate-900"><T>Medicines</T></h1>
        <ButtonLink href="/medicines/new">Order medicine</ButtonLink>
      </div>
      <RequireAuth>
        {(user) =>
          user.role === "patient" ? <MyMedicines /> : <p className="text-sm text-slate-600"><T>This page is for patient accounts.</T></p>
        }
      </RequireAuth>
    </Container>
  );
}
