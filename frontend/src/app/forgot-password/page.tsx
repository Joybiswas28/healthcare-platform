import type { Metadata } from "next";
import { Container } from "@/components/ui";
import { ResetForm } from "./reset-form";

export const metadata: Metadata = { title: "Reset password" };

export default function ForgotPasswordPage() {
  return (
    <Container className="max-w-md py-12">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">Reset your password</h1>
      <ResetForm />
    </Container>
  );
}
