import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { HospitalResults } from "./hospital-results";

export const metadata: Metadata = { title: "Hospitals" };

export default function HospitalsPage() {
  return (
    <Container className="py-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold text-slate-900">Hospitals</h1>
        <Link href="/hospitals/sylhet" className="text-sm font-medium text-teal-800 hover:underline">
          📞 Sylhet hospital &amp; ambulance numbers
        </Link>
      </div>
      <Suspense fallback={<Spinner />}>
        <HospitalResults />
      </Suspense>
    </Container>
  );
}
