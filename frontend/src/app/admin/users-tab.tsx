"use client";

import { useEffect, useState, type FormEvent } from "react";
import { useAuth } from "@/components/auth-provider";
import { Alert, Badge, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Role, type User } from "@/lib/api/client";
import { dateTime, ROLE_LABELS } from "@/lib/format";
import { T } from "@/components/t";
import { useT } from "@/components/use-lang";

const PAGE_SIZE = 30;

export function UsersTab() {
  const t = useT();
  const { user: me } = useAuth();
  const [query, setQuery] = useState({ q: "", role: "" as Role | "" });
  const [items, setItems] = useState<User[] | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState("");

  async function load(offset: number) {
    const { data, error } = await api.GET("/api/v1/admin/users", {
      params: { query: { q: query.q || undefined, role: query.role || undefined, limit: PAGE_SIZE, offset } },
    });
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    const page = data.users ?? [];
    setItems((prev) => (offset === 0 ? page : [...(prev ?? []), ...page]));
    setHasMore(page.length === PAGE_SIZE);
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load(0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query]);

  function search(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    setItems(null);
    setError("");
    setQuery({ q: String(f.get("q")).trim(), role: String(f.get("role")) as Role | "" });
  }

  async function setStatus(u: User, status: "active" | "suspended") {
    if (!u.id) return;
    const question =
      status === "suspended"
        ? t("Suspend {name}? They are signed out everywhere and can't log in until reactivated.", { name: u.name })
        : t("Reactivate {name}?", { name: u.name });
    if (!confirm(question)) return;
    const { data, error } = await api.POST("/api/v1/admin/users/{userID}/status", {
      params: { path: { userID: u.id } },
      body: { status },
    });
    if (data) setItems((prev) => prev?.map((x) => (x.id === data.id ? data : x)) ?? null);
    else setError(errorMessage(error));
  }

  return (
    <div className="space-y-4">
      <form onSubmit={search} className="flex flex-wrap gap-2">
        <input
          name="q"
          placeholder={t("Name or phone number")}
          aria-label={t("Name or phone number")}
          className="min-w-60 flex-1 rounded-lg border border-slate-300 bg-surface px-3 py-2 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20"
        />
        <select name="role" aria-label={t("Role")} className="rounded-lg border border-slate-300 bg-surface px-3 py-2 text-sm">
          <option value="">{t("All roles")}</option>
          {Object.entries(ROLE_LABELS).map(([value, label]) => (
            <option key={value} value={value}>
              {t(label)}
            </option>
          ))}
        </select>
        <Button type="submit">Search</Button>
      </form>

      {error && <Alert>{error}</Alert>}
      {items === null ? (
        !error && <Spinner />
      ) : items.length === 0 ? (
        <p className="text-sm text-slate-600"><T>No accounts match.</T></p>
      ) : (
        <Card className="p-0">
          <ul className="divide-y divide-slate-100">
            {items.map((u) => (
              <li key={u.id} className="flex flex-wrap items-center justify-between gap-3 px-5 py-3 text-sm">
                <span>
                  <span className="font-medium text-slate-900">{u.name}</span>
                  {u.id === me?.id && <span className="text-slate-500"> {t("(you)")}</span>}
                  <span className="block text-slate-600">
                    {u.phone} · <T>{ROLE_LABELS[u.role ?? ""] ?? u.role ?? ""}</T> · <T vars={{ date: dateTime(u.created_at) }}>{"joined {date}"}</T>
                  </span>
                </span>
                <span className="flex items-center gap-3">
                  {u.status === "suspended" ? <Badge tone="red">Suspended</Badge> : <Badge tone="teal">Active</Badge>}
                  {u.role !== "admin" &&
                    (u.status === "suspended" ? (
                      <Button variant="secondary" onClick={() => setStatus(u, "active")}>
                        <T>Reactivate</T>
                      </Button>
                    ) : (
                      <Button variant="danger" onClick={() => setStatus(u, "suspended")}>
                        <T>Suspend</T>
                      </Button>
                    ))}
                </span>
              </li>
            ))}
          </ul>
        </Card>
      )}
      {items && hasMore && (
        <Button variant="secondary" onClick={() => load(items.length)}>
          <T>Show more</T>
        </Button>
      )}
    </div>
  );
}
