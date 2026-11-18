"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { LocationPicker, validCoords } from "@/components/location-picker";
import { RequireAuth } from "@/components/require-auth";
import { TextArea } from "@/components/textarea";
import { Alert, Button, Card, Field, Spinner } from "@/components/ui";
import { api, errorMessage, type MedicineRequestInput, type Prescription, type RequestItem } from "@/lib/api/client";
import { dateTime } from "@/lib/format";
import type { Coords } from "@/lib/geo";

// Every request needs a prescription: one from this site, or a photo/PDF of a paper one.
type Source = "prescription" | "paper";

const MAX_FILE_BYTES = 10 * 1024 * 1024;
const input =
  "w-full rounded-lg border border-slate-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20";

export function RequestForm() {
  return (
    <RequireAuth>
      {(user) =>
        user.role === "patient" ? <Form /> : <p className="text-sm text-slate-600">Only patient accounts can order medicine.</p>
      }
    </RequireAuth>
  );
}

function Form() {
  const router = useRouter();
  const preset = useSearchParams().get("prescription_id");
  const [source, setSource] = useState<Source>("prescription");
  const [prescriptions, setPrescriptions] = useState<Prescription[] | null>(null);
  const [prescriptionID, setPrescriptionID] = useState(preset ?? "");
  const [items, setItems] = useState<RequestItem[]>([{ name: "", strength: "", quantity: "" }]);
  const [fulfilment, setFulfilment] = useState<"delivery" | "pickup">("delivery");
  const [coords, setCoords] = useState<Coords | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const file = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api.GET("/api/v1/prescriptions", { params: { query: { limit: 50 } } }).then(({ data }) => {
      const valid = (data?.prescriptions ?? []).filter((p) => p.status === "issued");
      setPrescriptions(valid);
      if (valid.length === 0 && !preset) setSource("paper");
    });
  }, [preset]);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    if (!validCoords(coords)) {
      setError(fulfilment === "delivery" ? "Add the delivery location." : "Add your location so nearby pharmacies can see the request.");
      return;
    }
    const cleanItems = items
      .map((i) => ({ name: i.name.trim(), strength: i.strength?.trim() || undefined, quantity: i.quantity?.trim() || undefined }))
      .filter((i) => i.name);
    const body: MedicineRequestInput = {
      fulfilment,
      latitude: coords.latitude,
      longitude: coords.longitude,
      address: String(f.get("address") ?? "").trim() || undefined,
      contact_phone: String(f.get("contact_phone") ?? "").trim() || undefined,
      note: String(f.get("note") ?? "").trim() || undefined,
      items: source === "paper" && cleanItems.length ? cleanItems : undefined,
    };

    setBusy(true);
    setError("");
    let res: Response;
    if (source === "prescription") {
      if (!prescriptionID) {
        setBusy(false);
        setError("Choose a prescription.");
        return;
      }
      res = await fetch("/api/v1/medicine-requests", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ ...body, prescription_id: prescriptionID }),
      });
    } else {
      const picked = file.current?.files?.[0];
      if (!picked) {
        setBusy(false);
        setError("Attach a photo or PDF of the prescription.");
        return;
      }
      if (picked.size > MAX_FILE_BYTES) {
        setBusy(false);
        setError("The file can be at most 10 MB.");
        return;
      }
      // The backend reads "data" first, then "file".
      const form = new FormData();
      form.append("data", JSON.stringify(body));
      form.append("file", picked);
      res = await fetch("/api/v1/medicine-requests", { method: "POST", body: form });
    }
    const data = await res.json().catch(() => null);
    setBusy(false);
    if (res.ok && data?.id) router.push(`/medicines/requests/${data.id}`);
    else setError(errorMessage(data, "Could not send the request."));
  }

  if (prescriptions === null) return <Spinner />;

  const tabs: { id: Source; label: string }[] = [
    { id: "prescription", label: "Prescription from this site" },
    { id: "paper", label: "Paper prescription" },
  ];

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <Card className="space-y-4">
        <div className="flex flex-wrap gap-2" role="tablist">
          {tabs.map((t) => (
            <button
              type="button"
              key={t.id}
              role="tab"
              aria-selected={source === t.id}
              onClick={() => setSource(t.id)}
              className={`rounded-full px-3 py-1 text-sm ${
                source === t.id ? "bg-teal-700 text-white" : "bg-white text-slate-700 ring-1 ring-slate-200 hover:ring-teal-600"
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>

        {source === "prescription" &&
          (prescriptions.length === 0 ? (
            <Alert kind="info">You have no valid prescriptions here. Upload a photo of a paper prescription instead.</Alert>
          ) : (
            <ul className="space-y-2">
              {prescriptions.map((p) => (
                <li key={p.id}>
                  <label
                    className={`flex cursor-pointer gap-3 rounded-lg border p-3 text-sm ${
                      prescriptionID === p.id ? "border-teal-700 bg-teal-50" : "border-slate-200"
                    }`}
                  >
                    <input
                      type="radio"
                      name="prescription"
                      checked={prescriptionID === p.id}
                      onChange={() => setPrescriptionID(p.id ?? "")}
                      className="mt-1 accent-teal-700"
                    />
                    <span>
                      <span className="font-medium text-slate-900">{p.doctor_name}</span> · {dateTime(p.issued_at)}
                      <span className="block text-slate-600">
                        {(p.medicines ?? []).map((m) => m.name).join(", ") || "No medicines"}
                      </span>
                    </span>
                  </label>
                </li>
              ))}
            </ul>
          ))}

        {source === "paper" && (
          <label className="block text-sm">
            <span className="mb-1 block font-medium text-slate-700">Photo or PDF of the prescription</span>
            <input ref={file} type="file" accept="application/pdf,image/jpeg,image/png" className="text-sm" />
            <span className="mt-1 block text-xs text-slate-500">Up to 10 MB. Only pharmacies quoting for it can open it.</span>
          </label>
        )}

        {source === "paper" && (
          <div className="space-y-2">
            <p className="text-sm font-medium text-slate-700">
              Medicines <span className="font-normal text-slate-500">(optional, helps pharmacies read the prescription)</span>
            </p>
            {items.map((it, i) => (
              <div key={i} className="grid grid-cols-[1fr_6rem_7rem_auto] gap-2">
                <input aria-label="Name" placeholder="Name, e.g. Napa" value={it.name} onChange={(e) => setItems((p) => p.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} className={input} />
                <input aria-label="Strength" placeholder="500 mg" value={it.strength ?? ""} onChange={(e) => setItems((p) => p.map((x, j) => (j === i ? { ...x, strength: e.target.value } : x)))} className={input} />
                <input aria-label="Quantity" placeholder="10 tablets" value={it.quantity ?? ""} onChange={(e) => setItems((p) => p.map((x, j) => (j === i ? { ...x, quantity: e.target.value } : x)))} className={input} />
                <button type="button" onClick={() => setItems((p) => p.filter((_, j) => j !== i))} className="px-2 text-sm text-red-700" aria-label="Remove">
                  ✕
                </button>
              </div>
            ))}
            <button type="button" onClick={() => setItems((p) => [...p, { name: "", strength: "", quantity: "" }])} className="text-sm text-teal-800 hover:underline">
              + Add medicine
            </button>
          </div>
        )}
      </Card>

      <Card className="space-y-4">
        <fieldset>
          <legend className="mb-2 text-sm font-medium text-slate-700">How do you want it?</legend>
          <div className="flex gap-4 text-sm">
            {(["delivery", "pickup"] as const).map((v) => (
              <label key={v} className="flex items-center gap-2">
                <input type="radio" name="fulfilment" checked={fulfilment === v} onChange={() => setFulfilment(v)} className="accent-teal-700" />
                {v === "delivery" ? "Home delivery" : "I'll pick it up"}
              </label>
            ))}
          </div>
        </fieldset>
        <LocationPicker
          value={coords}
          onChange={setCoords}
          label={fulfilment === "delivery" ? "Delivery location" : "Your location (to find pharmacies nearby)"}
        />
        {fulfilment === "delivery" && (
          <Field label="Delivery address" name="address" required placeholder="House, road, area, landmarks" />
        )}
        <Field label="Contact phone (optional)" name="contact_phone" type="tel" placeholder="Defaults to your account's number" />
        <TextArea label="Note for the pharmacy (optional)" name="note" rows={2} maxLength={500} />
      </Card>

      {error && <Alert>{error}</Alert>}
      <Button type="submit" disabled={busy}>
        {busy ? "Sending…" : "Ask nearby pharmacies"}
      </Button>
    </form>
  );
}
