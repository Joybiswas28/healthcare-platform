import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { VerifyResult } from "./verify-result";

export default function VerifyCodePage() {
  return (
    <Container className="max-w-2xl py-12">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">Prescription check</h1>
      <Suspense fallback={<Spinner />}>
        <VerifyResult />
      </Suspense>
    </Container>
  );
}
