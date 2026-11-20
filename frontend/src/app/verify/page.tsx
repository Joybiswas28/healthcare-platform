import type { Metadata } from "next";
import { Container } from "@/components/ui";
import { VerifyForm } from "./verify-form";
import { T } from "@/components/t";

export const metadata: Metadata = { title: "Verify a prescription" };

export default function VerifyPage() {
  return (
    <Container className="max-w-lg py-12">
      <h1 className="mb-2 text-2xl font-semibold text-slate-900"><T>Verify a prescription</T></h1>
      <p className="mb-6 text-sm text-slate-600">
        <T>For pharmacies: enter the code printed at the bottom of the prescription to check that it is genuine and still valid.</T>
      </p>
      <VerifyForm />
    </Container>
  );
}
