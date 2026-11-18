"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { TextArea } from "@/components/textarea";
import { Alert, Button } from "@/components/ui";
import { api, errorMessage, type Consultation } from "@/lib/api/client";
import { tokens } from "@/lib/format";

type Props = { consultation: Consultation; asDoctor: boolean; onChange: (c: Consultation) => void };

type Action = "accept" | "reject" | "confirm" | "complete" | "cancel";

const PATHS = {
  accept: "/api/v1/consultations/{consultationID}/accept",
  confirm: "/api/v1/consultations/{consultationID}/confirm",
  complete: "/api/v1/consultations/{consultationID}/complete",
  cancel: "/api/v1/consultations/{consultationID}/cancel",
} as const;

// The buttons each side gets in each state. The backend enforces the same rules.
export function Actions({ consultation: c, asDoctor, onChange }: Props) {
  const [busy, setBusy] = useState<Action | null>(null);
  const [error, setError] = useState("");
  const [needsTokens, setNeedsTokens] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const id = c.id ?? "";

  async function run(action: Exclude<Action, "reject">, question?: string) {
    if (question && !confirm(question)) return;
    setBusy(action);
    setError("");
    setNeedsTokens(false);
    const { data, error, response } = await api.POST(PATHS[action], {
      params: { path: { consultationID: id } },
    });
    setBusy(null);
    if (data) {
      onChange(data);
      return;
    }
    if (action === "confirm" && response.status === 409) setNeedsTokens(true);
    setError(errorMessage(error));
  }

  async function reject(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const reason = String(new FormData(e.currentTarget).get("reason")).trim();
    setBusy("reject");
    setError("");
    const { data, error } = await api.POST("/api/v1/consultations/{consultationID}/reject", {
      params: { path: { consultationID: id } },
      body: { reason },
    });
    setBusy(null);
    if (data) {
      setRejecting(false);
      onChange(data);
    } else setError(errorMessage(error));
  }

  const buttons: React.ReactNode[] = [];
  let note = "";

  if (asDoctor) {
    if (c.status === "requested") {
      note = "Accept if you can help. The patient then pays and the chat opens.";
      buttons.push(
        <Button key="accept" onClick={() => run("accept")} disabled={busy !== null}>
          {busy === "accept" ? "Accepting…" : "Accept"}
        </Button>,
        <Button key="reject" variant="secondary" onClick={() => setRejecting(true)} disabled={busy !== null}>
          Decline
        </Button>,
      );
    } else if (c.status === "accepted") {
      note = "Waiting for the patient to pay.";
    } else if (c.status === "active") {
      note = `When you're done, complete the consultation to receive ${tokens(c.fee_tokens)}.`;
      buttons.push(
        <Button key="complete" onClick={() => run("complete", "Mark this consultation as complete?")} disabled={busy !== null}>
          {busy === "complete" ? "Completing…" : "Complete"}
        </Button>,
        <Button
          key="cancel"
          variant="danger"
          onClick={() => run("cancel", "Cancel and refund the patient in full?")}
          disabled={busy !== null}
        >
          Cancel with refund
        </Button>,
      );
    }
  } else {
    if (c.status === "requested") {
      note = "The doctor will reply soon. You pay only after they accept.";
      buttons.push(
        <Button key="cancel" variant="danger" onClick={() => run("cancel", "Withdraw this request?")} disabled={busy !== null}>
          Withdraw request
        </Button>,
      );
    } else if (c.status === "accepted") {
      note = `The doctor accepted. Pay ${tokens(c.fee_tokens)} to start the chat.`;
      buttons.push(
        <Button
          key="confirm"
          onClick={() => run("confirm", `Pay ${tokens(c.fee_tokens)} and start the consultation?`)}
          disabled={busy !== null}
        >
          {busy === "confirm" ? "Paying…" : `Pay ${tokens(c.fee_tokens)} and start`}
        </Button>,
        <Button key="cancel" variant="secondary" onClick={() => run("cancel", "Cancel this consultation?")} disabled={busy !== null}>
          Cancel
        </Button>,
      );
    } else if (c.status === "active") {
      note = "The chat is open. The doctor will complete the consultation when you're done.";
    }
  }

  if (!note && buttons.length === 0) return null;

  return (
    <div className="mt-4 space-y-3 border-t border-slate-100 pt-4">
      {note && <p className="text-sm text-slate-600">{note}</p>}
      {error && (
        <Alert>
          {error}
          {needsTokens && (
            <>
              {" "}
              <Link href="/wallet" className="font-medium underline">
                Buy tokens
              </Link>
            </>
          )}
        </Alert>
      )}
      {rejecting ? (
        <form onSubmit={reject} className="space-y-3">
          <TextArea label="Why are you declining?" name="reason" rows={3} minLength={5} required autoFocus />
          <div className="flex gap-2">
            <Button type="submit" variant="danger" disabled={busy !== null}>
              {busy === "reject" ? "Declining…" : "Decline request"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => setRejecting(false)}>
              Back
            </Button>
          </div>
        </form>
      ) : (
        buttons.length > 0 && <div className="flex flex-wrap gap-2">{buttons}</div>
      )}
    </div>
  );
}
