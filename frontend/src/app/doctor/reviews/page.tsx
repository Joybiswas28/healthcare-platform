"use client";

import { RequireAuth } from "@/components/require-auth";
import { T } from "@/components/t";
import { Container } from "@/components/ui";
import { ReviewsView } from "./reviews-view";

export default function DoctorReviewsPage() {
  return (
    <Container className="max-w-3xl py-8">
      <h1 className="mb-6 text-2xl font-semibold text-slate-900">
        <T>Patient reviews</T>
      </h1>
      <RequireAuth>
        {(user) =>
          user.role === "doctor" ? (
            <ReviewsView />
          ) : (
            <p className="text-sm text-slate-600">
              <T>This page is for doctors.</T>
            </p>
          )
        }
      </RequireAuth>
    </Container>
  );
}
