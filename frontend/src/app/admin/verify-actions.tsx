"use client";

import { useState, type FormEvent } from "react";
import { Alert, Button } from "@/components/ui";
import { T } from "@/components/t";
import { useT } from "@/components/use-lang";

type Decision = { status: "approved" | "rejected"; note?: string };

// Approve, or reject with a note the applicant sees. `onDecide` returns an error message or "".
export function VerifyActions({
  current,
  onDecide,
}: {
  current?: "pending" | "approved" | "rejected";
  onDecide: (d: Decision) => Promise<string>;
}) {
  const [rejecting, setRejecting] = useState(false);
  const t = useT();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function decide(d: Decision) {
    setBusy(true);
    setError("");
    const problem = await onDecide(d);
    setBusy(false);
    if (problem) setError(problem);
    else setRejecting(false);
  }

  function reject(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    decide({ status: "rejected", note: String(new FormData(e.currentTarget).get("note")).trim() });
  }

  return (
    <div className="space-y-2">
      {rejecting ? (
        <form onSubmit={reject} className="flex flex-wrap items-end gap-2">
          <label className="min-w-60 flex-1 text-sm">
            <span className="mb-1 block font-medium text-slate-700"><T>Reason (shown to the applicant)</T></span>
            <input
              name="note"
              required
              minLength={3}
              autoFocus
              placeholder={t("e.g. Licence number does not match the certificate")}
              className="w-full rounded-lg border border-slate-300 px-3 py-2 outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20"
            />
          </label>
          <Button type="submit" variant="danger" disabled={busy}>
            <T>Reject</T>
          </Button>
          <Button type="button" variant="secondary" onClick={() => setRejecting(false)}>
            <T>Back</T>
          </Button>
        </form>
      ) : (
        <div className="flex flex-wrap gap-2">
          {current !== "approved" && (
            <Button onClick={() => decide({ status: "approved" })} disabled={busy}>
              <T>Approve</T>
            </Button>
          )}
          {current !== "rejected" && (
            <Button variant="danger" onClick={() => setRejecting(true)} disabled={busy}>
              {current === "approved" ? "Revoke approval" : "Reject"}
            </Button>
          )}
        </div>
      )}
      {error && <Alert>{error}</Alert>}
    </div>
  );
}
