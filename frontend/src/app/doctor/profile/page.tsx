"use client";

import { RequireAuth } from "@/components/require-auth";
import { Container } from "@/components/ui";
import { ProfileForm } from "./profile-form";

export default function DoctorProfilePage() {
  return (
    <Container className="max-w-2xl py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">Professional profile</h1>
      <RequireAuth>
        {(user) => (user.role === "doctor" ? <ProfileForm /> : <p className="text-sm text-slate-600">This page is for doctors.</p>)}
      </RequireAuth>
    </Container>
  );
}
