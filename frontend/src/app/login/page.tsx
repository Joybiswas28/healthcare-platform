import type { Metadata } from "next";
import { Suspense } from "react";
import { T } from "@/components/t";
import { Container, Spinner } from "@/components/ui";
import { LoginForm } from "./login-form";

export const metadata: Metadata = { title: "Log in" };

export default function LoginPage() {
  return (
    <Container className="max-w-md py-12">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">
        <T>Log in</T>
      </h1>
      <Suspense fallback={<Spinner />}>
        <LoginForm />
      </Suspense>
    </Container>
  );
}
