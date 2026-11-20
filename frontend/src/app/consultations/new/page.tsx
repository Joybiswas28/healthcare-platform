import type { Metadata } from "next";
import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { RequestForm } from "./request-form";
import { T } from "@/components/t";

export const metadata: Metadata = { title: "Request a consultation" };

export default function NewConsultationPage() {
  return (
    <Container className="max-w-2xl py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900"><T>Request an online consultation</T></h1>
      <Suspense fallback={<Spinner />}>
        <RequestForm />
      </Suspense>
    </Container>
  );
}
