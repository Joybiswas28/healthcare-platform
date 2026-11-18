"use client";

import { useEffect, useRef, useState } from "react";
import { Alert, Button, Card } from "@/components/ui";
import { api, errorMessage, type Hospital, type Photo } from "@/lib/api/client";
import { HospitalForm } from "../hospital-form";

const MAX_PHOTOS = 10;
const MAX_PHOTO_BYTES = 5 * 1024 * 1024;

export function ProfileTab({ hospital, onChange }: { hospital: Hospital; onChange: (h: Hospital) => void }) {
  const id = hospital.id ?? "";
  return (
    <div className="space-y-6">
      <Photos hospital={hospital} onLogoChange={(logo) => onChange({ ...hospital, logo_url: logo })} />
      <Card>
        <h2 className="mb-4 font-semibold text-slate-900">Hospital profile</h2>
        <HospitalForm
          initial={hospital}
          submitLabel="Save profile"
          onSave={async (input) => {
            const { data, error } = await api.PUT("/api/v1/manage/hospitals/{hospitalID}", {
              params: { path: { hospitalID: id } },
              body: input,
            });
            if (!data) return errorMessage(error);
            onChange(data);
            return "";
          }}
        />
      </Card>
    </div>
  );
}

function Photos({ hospital, onLogoChange }: { hospital: Hospital; onLogoChange: (url: string | null) => void }) {
  const id = hospital.id ?? "";
  const [photos, setPhotos] = useState<Photo[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api.GET("/api/v1/manage/hospitals/{hospitalID}/photos", { params: { path: { hospitalID: id } } }).then(({ data, error }) => {
      if (data) setPhotos(data.photos ?? []);
      else setError(errorMessage(error));
    });
  }, [id]);

  async function upload(file: File) {
    if (file.size > MAX_PHOTO_BYTES) {
      setError("Photos can be at most 5 MB.");
      return;
    }
    setBusy(true);
    setError("");
    const form = new FormData();
    form.append("file", file);
    const res = await fetch(`/api/v1/manage/hospitals/${id}/photos`, { method: "POST", body: form });
    const data = await res.json().catch(() => null);
    setBusy(false);
    if (input.current) input.current.value = "";
    if (!res.ok || !data) {
      setError(errorMessage(data, "Could not upload the photo."));
      return;
    }
    setPhotos((prev) => [...(prev ?? []), data as Photo]);
    if (!hospital.logo_url) onLogoChange((data as Photo).url ?? null);
  }

  async function remove(p: Photo) {
    if (!p.id || !confirm("Delete this photo?")) return;
    const { error } = await api.DELETE("/api/v1/manage/hospitals/{hospitalID}/photos/{photoID}", {
      params: { path: { hospitalID: id, photoID: p.id } },
    });
    if (error) setError(errorMessage(error));
    else setPhotos((prev) => prev?.filter((x) => x.id !== p.id) ?? null);
  }

  async function makeLogo(p: Photo) {
    if (!p.id) return;
    const { error } = await api.POST("/api/v1/manage/hospitals/{hospitalID}/photos/{photoID}/logo", {
      params: { path: { hospitalID: id, photoID: p.id } },
    });
    if (error) setError(errorMessage(error));
    else onLogoChange(p.url ?? null);
  }

  const list = photos ?? [];
  return (
    <Card>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="font-semibold text-slate-900">Photos</h2>
          <p className="text-xs text-slate-500">JPEG, PNG or WebP, up to 5 MB, at most {MAX_PHOTOS}. The first becomes the logo.</p>
        </div>
        <Button variant="secondary" disabled={busy || list.length >= MAX_PHOTOS} onClick={() => input.current?.click()}>
          {busy ? "Uploading…" : "Upload photo"}
        </Button>
        <input
          ref={input}
          type="file"
          accept="image/jpeg,image/png,image/webp"
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) upload(f);
          }}
        />
      </div>
      {error && <Alert>{error}</Alert>}
      {photos !== null && list.length === 0 && <p className="text-sm text-slate-500">No photos yet.</p>}
      <ul className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {list.map((p) => {
          const isLogo = !!hospital.logo_url && hospital.logo_url === p.url;
          return (
            <li key={p.id} className="overflow-hidden rounded-lg border border-slate-200">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={p.url} alt="" className="h-28 w-full object-cover" />
              <div className="flex justify-between gap-1 p-2 text-xs">
                {isLogo ? (
                  <span className="font-medium text-teal-800">Logo</span>
                ) : (
                  <button onClick={() => makeLogo(p)} className="text-teal-800 hover:underline">
                    Use as logo
                  </button>
                )}
                <button onClick={() => remove(p)} className="text-red-700 hover:underline">
                  Delete
                </button>
              </div>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}
