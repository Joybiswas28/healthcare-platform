import type { Metadata } from "next";
import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { RequestForm } from "./request-form";
import { T } from "@/components/t";

export const metadata: Metadata = { title: "Order medicine" };

export default function NewMedicineRequestPage() {
  return (
    <Container className="max-w-2xl py-8">
      <h1 className="mb-2 text-2xl font-semibold text-slate-900"><T>Order medicine</T></h1>
      <p className="mb-6 text-sm text-slate-600">
        <T>Nearby pharmacies see what you need (not who you are) and send prices. You pick one and pay cash on delivery.</T>
      </p>
      <Suspense fallback={<Spinner />}>
        <RequestForm />
      </Suspense>
    </Container>
  );
}
