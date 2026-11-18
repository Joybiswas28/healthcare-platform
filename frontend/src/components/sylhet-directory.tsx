import { Alert, Badge, Card } from "@/components/ui";
import {
  AMBULANCES,
  CHECKED_ON,
  HOSPITALS,
  NATIONAL_EMERGENCY,
  telHref,
  type DirectoryEntry,
} from "@/data/sylhet-directory";
import { prettyDate } from "@/lib/format";

// Phone numbers of Sylhet hospitals and ambulance services, shown on the
// hospitals page. These are a reference list, not hospitals on the platform.
export function SylhetDirectory() {
  return (
    <section id="sylhet" className="scroll-mt-6 space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-slate-900">Sylhet: hospitals and ambulances</h2>
        <p className="mt-1 text-sm text-slate-600">
          Phone numbers collected from public sources on {prettyDate(CHECKED_ON)}. Online booking is only for the hospitals
          listed above.
        </p>
      </div>

      <div className="rounded-xl border-2 border-red-300 bg-red-50 p-5">
        <h3 className="text-lg font-semibold text-red-800">In an emergency</h3>
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
      </div>

      <Alert kind="info">
        Numbers change. <strong>Confirmed</strong> numbers were printed on the hospital&apos;s own website or the UK
        government&apos;s list of medical facilities; <strong>From directories</strong> could not be confirmed with the
        provider. Each entry links to where its number came from.
      </Alert>

      <div className="space-y-3">
        <h3 className="text-lg font-semibold text-slate-900">Hospitals</h3>
        <div className="grid gap-3 lg:grid-cols-2">
          {HOSPITALS.map((h) => (
            <Card key={h.name} className="space-y-2">
              <EntryHeader entry={h} />
              <PhoneList phones={h.phones} />
              {h.note && <p className="text-xs text-slate-500">{h.note}</p>}
              <Sources entry={h} />
            </Card>
          ))}
        </div>
      </div>

      <div className="space-y-3">
        <h3 className="text-lg font-semibold text-slate-900">Ambulance services</h3>
        <div className="grid gap-3 lg:grid-cols-2">
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
        </div>
      </div>

      <p className="text-xs text-slate-500">
        Found a wrong or outdated number? Tell the platform team so it can be corrected. Last checked {prettyDate(CHECKED_ON)}.
      </p>
    </section>
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
