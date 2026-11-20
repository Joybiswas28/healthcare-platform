import type { Metadata } from "next";
import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { PrescriptionForm } from "./prescription-form";
import { T } from "@/components/t";

export const metadata: Metadata = { title: "Write a prescription" };

export default function NewPrescriptionPage() {
  return (
    <Container className="max-w-3xl py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900"><T>Write a prescription</T></h1>
      <Suspense fallback={<Spinner />}>
        <PrescriptionForm />
      </Suspense>
    </Container>
  );
}
