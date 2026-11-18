import type { Medicine } from "@/lib/api/client";

// The medicine table used on the prescription page and the public verify page.
export function MedicineList({ medicines }: { medicines: Medicine[] }) {
  if (medicines.length === 0) return <p className="text-sm text-slate-500">No medicines.</p>;
  return (
    <ol className="space-y-2">
      {medicines.map((m, i) => (
        <li key={i} className="rounded-lg border border-slate-200 px-3 py-2 text-sm">
          <p className="font-medium text-slate-900">
            {i + 1}. {[m.form, m.name, m.strength].filter(Boolean).join(" ")}
          </p>
          <p className="text-slate-700">
            {m.dosage} · {m.duration}
            {m.instructions ? ` · ${m.instructions}` : ""}
          </p>
        </li>
      ))}
    </ol>
  );
}
