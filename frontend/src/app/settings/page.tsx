"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { AccountSettings } from "./account-settings";
import { T } from "@/components/t";

export default function SettingsPage() {
  return (
    <Container className="max-w-2xl py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900"><T>Account settings</T></h1>
      <RequireAuth>{(user) => <AccountSettings user={user} />}</RequireAuth>
    </Container>
  );
}
