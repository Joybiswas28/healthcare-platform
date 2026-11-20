"use client";

import { useState, type FormEvent, type ReactNode } from "react";
import type { DoctorReview } from "@/lib/api/client";
import { dateTime } from "@/lib/format";
import { bnDigits } from "@/lib/i18n";
import { T } from "./t";
import { TextArea } from "./textarea";
import { Alert, Badge, Button, Card } from "./ui";
import { useLang, useT } from "./use-lang";

// An average rating with its review count: "★ 4.6 (12)".
export function Stars({ rating, count }: { rating?: number | null; count?: number }) {
  const lang = useLang();
  if (!count || rating == null) return <span className="text-xs text-slate-500"><T>No reviews yet</T></span>;
  const fmt = (s: string) => (lang === "bn" ? bnDigits(s) : s);
  return (
    <span className="text-sm text-slate-700">
      <span className="text-amber-500" aria-hidden>
        ★
      </span>{" "}
      {fmt(rating.toFixed(1))} <span className="text-xs text-slate-500">({fmt(String(count))})</span>
    </span>
  );
}

// One review's stars, filled up to its rating.
export function StarRow({ rating = 0 }: { rating?: number }) {
  const t = useT();
  return (
    <p className="text-amber-500" aria-label={t("{n} out of 5", { n: rating })}>
      {"★".repeat(rating)}
      <span className="text-slate-300">{"★".repeat(5 - rating)}</span>
    </p>
  );
}

// A 1-5 star picker with an optional comment. `send` returns an error message,
// or "" once saved.
export function RatingForm({
  title,
  hint,
  send,
}: {
  title: string;
  hint?: ReactNode;
  send: (rating: number, comment: string | undefined) => Promise<string>;
}) {
  const t = useT();
  const [rating, setRating] = useState(0);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!rating) {
      setError(t("Pick 1 to 5 stars."));
      return;
    }
    setBusy(true);
    setError("");
    const err = await send(rating, String(new FormData(e.currentTarget).get("comment") ?? "").trim() || undefined);
    setBusy(false);
    setError(err);
  }

  return (
    <form onSubmit={submit} className="space-y-3">
      <h2 className="font-semibold text-slate-900">
        <T>{title}</T>
      </h2>
      {hint && <p className="text-sm text-slate-600">{hint}</p>}
      <div className="flex gap-1" role="radiogroup" aria-label={t("Rating")}>
        {[1, 2, 3, 4, 5].map((n) => (
          <button
            type="button"
            key={n}
            role="radio"
            aria-checked={rating === n}
            aria-label={t("{n} out of 5", { n })}
            onClick={() => setRating(n)}
            className={`text-3xl leading-none ${n <= rating ? "text-amber-500" : "text-slate-300"}`}
          >
            ★
          </button>
        ))}
      </div>
      <TextArea label="Comment (optional)" name="comment" rows={2} maxLength={1000} />
      {error && <Alert>{error}</Alert>}
      <Button type="submit" disabled={busy}>
        {busy ? "Saving…" : "Submit rating"}
      </Button>
    </form>
  );
}

// A doctor review as the public and the doctor see it. `footer` holds the
// doctor's reply controls on their own reviews page.
export function DoctorReviewCard({ review: r, footer }: { review: DoctorReview; footer?: ReactNode }) {
  return (
    <Card className="p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <StarRow rating={r.rating} />
        <Badge>
          <T>{r.source === "online" ? "Online consultation" : "Chamber visit"}</T>
        </Badge>
      </div>
      {r.comment && <p className="mt-1 whitespace-pre-line text-sm text-slate-700">{r.comment}</p>}
      <p className="mt-1 text-xs text-slate-500">{dateTime(r.created_at)}</p>
      {r.reply && !footer && (
        <div className="mt-3 border-l-2 border-teal-600 pl-3">
          <p className="text-xs font-medium text-teal-800">
            <T>Doctor&apos;s reply</T>
          </p>
          <p className="whitespace-pre-line text-sm text-slate-700">{r.reply}</p>
        </div>
      )}
      {footer}
    </Card>
  );
}
