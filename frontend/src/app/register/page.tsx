import type { Metadata } from "next";
import { T } from "@/components/t";
import { Container } from "@/components/ui";
import { RegisterForm } from "./register-form";

export const metadata: Metadata = { title: "Create an account" };

export default function RegisterPage() {
  return (
    <Container className="max-w-md py-12">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">
        <T>Create an account</T>
      </h1>
      <RegisterForm />
    </Container>
  );
}
