"use client";

import { useEffect, useState } from "react";
import { T } from "@/components/t";
import { ButtonLink, Card } from "@/components/ui";
import { api } from "@/lib/api/client";

// A new doctor is invisible to patients until verified; point them to what's missing.
export function DoctorProfileNudge() {
  const [state, setState] = useState<{ status?: string; note?: string | null; hasBMDC: boolean } | null>(null);

  useEffect(() => {
    api.GET("/api/v1/doctor/profile").then(({ data }) => {
      if (data)
        setState({ status: data.verification_status, note: data.verification_note, hasBMDC: !!data.bmdc_reg_no });
    });
  }, []);

  if (!state || state.status === "approved") return null;

  const title =
    state.status === "rejected" ? "Verification was rejected" : state.hasBMDC ? "Waiting for verification" : "Finish your profile";
  const body =
    state.status === "rejected"
      ? state.note ?? "Please check your details."
      : state.hasBMDC
        ? "An admin is checking your BMDC registration. Patients can find you once it's approved."
        : "Add your BMDC registration number and degrees. Patients can find you only after an admin verifies them.";

  return (
    <Card className="border-amber-200 bg-amber-50 sm:col-span-2">
      <h2 className="font-semibold text-amber-900">
        <T>{title}</T>
      </h2>
      <p className="mt-1 text-sm text-amber-900">
        {/* An admin's rejection note is shown as written; the fixed messages are translated. */}
        {state.status === "rejected" ? body : <T>{body}</T>}
      </p>
      <ButtonLink href="/doctor/profile" className="mt-3">
        {state.hasBMDC ? "Review profile" : "Complete profile"}
      </ButtonLink>
    </Card>
  );
}
