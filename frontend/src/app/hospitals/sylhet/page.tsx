import type { Metadata } from "next";
import Link from "next/link";
import { Alert, Badge, Card, Container } from "@/components/ui";
import {
  AMBULANCES,
  CHECKED_ON,
  HOSPITALS,
  NATIONAL_EMERGENCY,
  telHref,
  type DirectoryEntry,
} from "@/data/sylhet-directory";
import { prettyDate } from "@/lib/format";

export const metadata: Metadata = {
  title: "Sylhet hospitals and ambulances",
  description: "Phone numbers of hospitals and ambulance services in Sylhet, with the source of each number.",
};

export default function SylhetDirectoryPage() {
  return (
    <Container className="max-w-3xl space-y-8 py-8">
      <div>
        <Link href="/hospitals" className="text-sm text-teal-800 hover:underline">
          ← Hospitals
        </Link>
        <h1 className="mt-2 text-2xl font-semibold text-slate-900">Sylhet: hospitals and ambulances</h1>
        <p className="mt-1 text-sm text-slate-600">
          A reference list of phone numbers, collected from public sources on {prettyDate(CHECKED_ON)}. These hospitals
          are not necessarily on this platform; to book a doctor online, use{" "}
          <Link href="/hospitals?location=Sylhet" className="text-teal-800 underline">
            hospitals on the platform
          </Link>
          .
        </p>
      </div>

      <section className="rounded-xl border-2 border-red-300 bg-red-50 p-5">
        <h2 className="text-lg font-semibold text-red-800">In an emergency</h2>
        <div className="mt-3 flex flex-wrap gap-3">
          {NATIONAL_EMERGENCY.map((p) => (
            <a
              key={p.number}
              href={telHref(p.number)}
              className="flex items-center gap-3 rounded-lg bg-red-600 px-4 py-3 text-white hover:bg-red-500"
            >
              <span className="text-2xl font-bold">{p.number}</span>
              <span className="text-sm leading-tight">{p.label}</span>
            </a>
          ))}
        </div>
        <p className="mt-3 text-sm text-red-800">999 is free and works from any phone. Call it first if life is at risk.</p>
      </section>

      <Alert kind="info">
        Numbers change. Entries marked <strong>Confirmed</strong> were printed on the hospital&apos;s own website or the UK
        government&apos;s list of medical facilities; <strong>From directories</strong> could not be confirmed with the
        provider. Each entry links to where its number came from.
      </Alert>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold text-slate-900">Ambulance services</h2>
        {HOSPITALS.filter((h) => h.ambulance?.length).map((h) => (
          <Card key={`amb-${h.name}`} className="space-y-2">
            <EntryHeader entry={h} />
            <PhoneList phones={h.ambulance ?? []} />
            <Sources entry={h} />
          </Card>
        ))}
        {AMBULANCES.map((a) => (
          <Card key={a.name} className="space-y-2">
            <EntryHeader entry={a} />
            <PhoneList phones={a.phones} />
            {a.note && <p className="text-xs text-slate-500">{a.note}</p>}
            <Sources entry={a} />
          </Card>
        ))}
      </section>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold text-slate-900">Hospitals</h2>
        {HOSPITALS.map((h) => (
          <Card key={h.name} className="space-y-2">
            <EntryHeader entry={h} />
            <PhoneList phones={h.phones} />
            {h.note && <p className="text-xs text-slate-500">{h.note}</p>}
            <Sources entry={h} />
          </Card>
        ))}
      </section>

      <p className="text-xs text-slate-500">
        Found a wrong or outdated number? Tell the platform team so it can be corrected. Last checked {prettyDate(CHECKED_ON)}.
      </p>
    </Container>
  );
}

function EntryHeader({ entry: e }: { entry: DirectoryEntry }) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div>
        <p className="font-semibold text-slate-900">{e.name}</p>
        <p className="text-sm text-slate-600">
          {e.address} · {e.kind}
        </p>
      </div>
      {e.confidence === "official" ? <Badge tone="teal">Confirmed</Badge> : <Badge tone="amber">From directories</Badge>}
    </div>
  );
}

function PhoneList({ phones }: { phones: { label: string; number: string }[] }) {
  return (
    <ul className="flex flex-wrap gap-2">
      {phones.map((p, i) => (
        <li key={i}>
          <a
            href={telHref(p.number)}
            className="inline-flex flex-col rounded-lg border border-slate-300 px-3 py-1.5 hover:border-teal-600"
          >
            <span className="font-mono text-sm font-semibold text-slate-900">📞 {p.number}</span>
            <span className="text-xs text-slate-500">{p.label}</span>
          </a>
        </li>
      ))}
    </ul>
  );
}

function Sources({ entry: e }: { entry: DirectoryEntry }) {
  return (
    <p className="text-xs text-slate-500">
      Source:{" "}
      {e.sources.map((s, i) => (
        <span key={s.url}>
          {i > 0 && ", "}
          <a href={s.url} target="_blank" rel="noopener noreferrer nofollow" className="underline hover:text-teal-800">
            {s.label}
          </a>
        </span>
      ))}
      {e.website && !e.sources.some((s) => s.url.startsWith(e.website!)) && (
        <>
          {" · "}
          <a href={e.website} target="_blank" rel="noopener noreferrer nofollow" className="underline hover:text-teal-800">
            Website
          </a>
        </>
      )}
    </p>
  );
}
