"use client";

import { useEffect, useState, type FormEvent } from "react";
import { DoctorReviewCard, Stars } from "@/components/rating";
import { T } from "@/components/t";
import { TextArea } from "@/components/textarea";
import { Alert, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type DoctorReview } from "@/lib/api/client";
import { dateTime } from "@/lib/format";

const PAGE_SIZE = 20;

export function ReviewsView() {
  const [reviews, setReviews] = useState<DoctorReview[]>([]);
  const [summary, setSummary] = useState<{ rating?: number | null; count?: number }>({});
  const [loading, setLoading] = useState(true);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState("");

  async function load(offset: number) {
    setLoading(true);
    const { data, error } = await api.GET("/api/v1/doctor/reviews", { params: { query: { limit: PAGE_SIZE, offset } } });
    setLoading(false);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    const page = data.reviews ?? [];
    setSummary({ rating: data.rating, count: data.review_count });
    setReviews((prev) => (offset === 0 ? page : [...prev, ...page]));
    setHasMore(page.length === PAGE_SIZE);
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load(0);
  }, []);

  const replaced = (r: DoctorReview) => setReviews((prev) => prev.map((x) => (x.id === r.id ? r : x)));

  return (
    <div className="space-y-4">
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-lg">
            <Stars rating={summary.rating} count={summary.count} />
          </p>
          <p className="text-sm text-slate-600">
            <T>Patients see these on your profile, without their names. Your replies are public too.</T>
          </p>
        </div>
      </Card>

      {error && <Alert>{error}</Alert>}
      {!loading && !error && reviews.length === 0 && (
        <p className="text-sm text-slate-600">
          <T>No reviews yet. Patients can rate you after a completed appointment or consultation.</T>
        </p>
      )}

      <ul className="space-y-2">
        {reviews.map((r) => (
          <li key={r.id}>
            <DoctorReviewCard review={r} footer={<Reply review={r} onSaved={replaced} />} />
          </li>
        ))}
      </ul>

      {loading && <Spinner />}
      {!loading && hasMore && (
        <div className="text-center">
          <Button variant="secondary" onClick={() => load(reviews.length)}>
            Show more
          </Button>
        </div>
      )}
    </div>
  );
}

function Reply({ review: r, onSaved }: { review: DoctorReview; onSaved: (r: DoctorReview) => void }) {
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function save(reply: string) {
    setBusy(true);
    setError("");
    const { data, error } = await api.PUT("/api/v1/doctor/reviews/{reviewID}/reply", {
      params: { path: { reviewID: r.id ?? "" } },
      body: { reply },
    });
    setBusy(false);
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    onSaved(data);
    setEditing(false);
  }

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    save(String(new FormData(e.currentTarget).get("reply") ?? "").trim());
  }

  if (editing) {
    return (
      <form onSubmit={submit} className="mt-3 space-y-2 border-t border-slate-100 pt-3">
        <TextArea label="Your reply" name="reply" rows={2} maxLength={1000} defaultValue={r.reply ?? ""} required />
        {error && <Alert>{error}</Alert>}
        <div className="flex flex-wrap gap-2">
          <Button type="submit" disabled={busy}>
            {busy ? "Saving…" : "Post reply"}
          </Button>
          <Button type="button" variant="secondary" onClick={() => setEditing(false)} disabled={busy}>
            Cancel
          </Button>
        </div>
      </form>
    );
  }

  return (
    <div className="mt-3 border-t border-slate-100 pt-3">
      {r.reply ? (
        <div className="border-l-2 border-teal-600 pl-3">
          <p className="text-xs font-medium text-teal-800">
            <T>Your reply</T> · {dateTime(r.replied_at)}
          </p>
          <p className="whitespace-pre-line text-sm text-slate-700">{r.reply}</p>
          <div className="mt-2 flex flex-wrap gap-2">
            <Button variant="secondary" onClick={() => setEditing(true)}>
              Edit reply
            </Button>
            <Button variant="secondary" onClick={() => save("")} disabled={busy}>
              Remove reply
            </Button>
          </div>
        </div>
      ) : (
        <Button variant="secondary" onClick={() => setEditing(true)}>
          Reply
        </Button>
      )}
      {error && !editing && <Alert>{error}</Alert>}
    </div>
  );
}
