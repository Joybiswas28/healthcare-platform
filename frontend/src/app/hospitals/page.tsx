import type { Metadata } from "next";
import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { HospitalResults } from "./hospital-results";

export const metadata: Metadata = { title: "Hospitals" };

export default function HospitalsPage() {
  return (
    <Container className="py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">Hospitals</h1>
      <Suspense fallback={<Spinner />}>
        <HospitalResults />
      </Suspense>
    </Container>
  );
}
