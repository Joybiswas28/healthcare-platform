import Link from "next/link";
import { T } from "@/components/t";
import { Badge } from "@/components/ui";
import type { Concern } from "@/lib/api/client";
import { dateTime } from "@/lib/format";

const STATUS = {
  open: { label: "Open", tone: "teal" },
  closed: { label: "Closed", tone: "slate" },
  converted: { label: "Consultation started", tone: "amber" },
} as const;

export function ConcernCard({ concern: c, showOffers }: { concern: Concern; showOffers: boolean }) {
  const status = STATUS[c.status ?? "open"];
  return (
    <Link
      href={`/concerns/${c.id}`}
      className="block rounded-xl border border-slate-200 bg-surface p-5 shadow-sm transition hover:border-teal-600"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <p className="font-semibold text-slate-900">{c.title}</p>
        <Badge tone={status.tone}>{status.label}</Badge>
      </div>
      <p className="mt-1 line-clamp-2 text-sm text-slate-600">{c.description}</p>
      <p className="mt-2 text-xs text-slate-500">
        {c.specialization ? `${c.specialization} · ` : ""}
        {dateTime(c.created_at)}
        {showOffers && (
          <>
            {" · "}
            <T vars={{ n: c.offer_count ?? 0 }}>{c.offer_count === 1 ? "{n} offer" : "{n} offers"}</T>
          </>
        )}
      </p>
    </Link>
  );
}
