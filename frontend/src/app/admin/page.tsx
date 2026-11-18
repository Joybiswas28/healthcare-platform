import type { Metadata } from "next";
import { Suspense } from "react";
import { Container, Spinner } from "@/components/ui";
import { AdminPanel } from "./admin-panel";

export const metadata: Metadata = { title: "Admin" };

export default function AdminPage() {
  return (
    <Container className="py-8">
      <Suspense fallback={<Spinner />}>
        <AdminPanel />
      </Suspense>
    </Container>
  );
}
