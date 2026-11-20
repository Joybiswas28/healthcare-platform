"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { WalletView } from "./wallet-view";
import { T } from "@/components/t";

export default function WalletPage() {
  return (
    <Container className="py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900"><T>Consultation tokens</T></h1>
      <RequireAuth>
        {(user) =>
          user.role === "patient" ? (
            <WalletView />
          ) : (
            <p className="text-sm text-slate-600"><T>Tokens are bought by patient accounts.</T></p>
          )
        }
      </RequireAuth>
    </Container>
  );
}
