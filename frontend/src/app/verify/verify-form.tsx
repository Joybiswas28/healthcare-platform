"use client";

import { useRouter } from "next/navigation";
import type { FormEvent } from "react";
import { Button, Card } from "@/components/ui";
import { useT } from "@/components/use-lang";

export function VerifyForm({ initial = "" }: { initial?: string }) {
  const t = useT();
  const router = useRouter();

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const code = String(new FormData(e.currentTarget).get("code")).trim().toUpperCase().replace(/\s+/g, "");
    if (code) router.push(`/verify/${encodeURIComponent(code)}`);
  }

  return (
    <Card>
      <form onSubmit={onSubmit} className="flex gap-2">
        <input
          name="code"
          defaultValue={initial}
          placeholder="e.g. MFP3B5E7R2"
          aria-label={t("Prescription code")}
          autoCapitalize="characters"
          required
          className="flex-1 rounded-lg border border-slate-300 px-3 py-2 font-mono uppercase tracking-wider outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20"
        />
        <Button type="submit">Check</Button>
      </form>
    </Card>
  );
}
