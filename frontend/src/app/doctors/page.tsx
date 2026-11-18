import type { Metadata } from "next";
import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { DoctorResults } from "./doctor-results";

export const metadata: Metadata = { title: "Find a doctor" };

export default function DoctorsPage() {
  return (
    <Container className="py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">Find a doctor</h1>
      <Suspense fallback={<Spinner />}>
        <DoctorResults />
      </Suspense>
    </Container>
  );
}
