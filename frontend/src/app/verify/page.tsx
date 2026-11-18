import type { Metadata } from "next";
import { Container } from "@/components/ui";
import { VerifyForm } from "./verify-form";

export const metadata: Metadata = { title: "Verify a prescription" };

export default function VerifyPage() {
  return (
    <Container className="max-w-lg py-12">
      <h1 className="mb-2 text-2xl font-semibold text-slate-900">Verify a prescription</h1>
      <p className="mb-6 text-sm text-slate-600">
        For pharmacies: enter the code printed at the bottom of the prescription to check that it is genuine and still
        valid.
      </p>
      <VerifyForm />
    </Container>
  );
}
