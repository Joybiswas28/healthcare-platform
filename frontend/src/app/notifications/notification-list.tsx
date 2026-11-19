"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Alert, Button, Card, Spinner } from "@/components/ui";
import { api, errorMessage, type Role, type Schemas } from "@/lib/api/client";
import { dateTime } from "@/lib/format";
import { notificationHref, notificationIcon, timeAgo } from "@/lib/notification-links";

type Notification = Schemas["Notification"];
const PAGE_SIZE = 30;

export function NotificationList({ role }: { role?: Role }) {
  const router = useRouter();
  const [onlyUnread, setOnlyUnread] = useState(false);
  const [items, setItems] = useState<Notification[] | null>(null);
  const [unread, setUnread] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState("");

  async function load(offset: number, unreadOnly: boolean) {
    const { data, error } = await api.GET("/api/v1/notifications", {
      params: { query: { unread: unreadOnly || undefined, limit: PAGE_SIZE, offset } },
    });
    if (!data) {
      setError(errorMessage(error));
      return;
    }
    const page = data.notifications ?? [];
    setItems((prev) => (offset === 0 ? page : [...(prev ?? []), ...page]));
    setUnread(data.unread_count ?? 0);
    setHasMore(page.length === PAGE_SIZE);
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load(0, onlyUnread);
  }, [onlyUnread]);

  function open(n: Notification) {
    if (!n.read_at && n.id != null) {
      api.POST("/api/v1/notifications/{notificationID}/read", { params: { path: { notificationID: n.id } } });
    }
    router.push(notificationHref(n.kind, role));
  }

  async function markAll() {
    const { error } = await api.POST("/api/v1/notifications/read-all");
    if (error) {
      setError(errorMessage(error));
      return;
    }
    setUnread(0);
    if (onlyUnread) setItems([]);
    else setItems((prev) => prev?.map((n) => ({ ...n, read_at: n.read_at ?? new Date().toISOString() })) ?? null);
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold text-slate-900">Notifications</h1>
        {unread > 0 && (
          <Button variant="secondary" onClick={markAll}>
            Mark all as read ({unread})
          </Button>
        )}
      </div>

      <div className="flex gap-2">
        {[
          { v: false, label: "All" },
          { v: true, label: "Unread" },
        ].map((f) => (
          <button
            key={f.label}
            onClick={() => {
              setItems(null);
              setOnlyUnread(f.v);
            }}
            className={`rounded-full px-3 py-1 text-sm ${
              onlyUnread === f.v ? "bg-teal-700 text-white" : "bg-surface text-slate-700 ring-1 ring-slate-200 hover:ring-teal-600"
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      {error && <Alert>{error}</Alert>}
      {items === null ? (
        !error && <Spinner />
      ) : items.length === 0 ? (
        <Card>
          <p className="text-center text-slate-600">{onlyUnread ? "You're all caught up." : "No notifications yet."}</p>
        </Card>
      ) : (
        <Card className="p-0">
          <ul className="divide-y divide-slate-100">
            {items.map((n) => (
              <li key={n.id}>
                <button
                  onClick={() => open(n)}
                  className={`flex w-full gap-3 px-5 py-4 text-left hover:bg-slate-50 ${n.read_at ? "" : "bg-teal-50"}`}
                >
                  <span className="text-xl leading-none" aria-hidden>
                    {notificationIcon(n.kind)}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-start justify-between gap-3">
                      <span className={`text-slate-900 ${n.read_at ? "" : "font-semibold"}`}>{n.title}</span>
                      <span className="shrink-0 text-xs text-slate-500" title={dateTime(n.created_at)}>
                        {timeAgo(n.created_at)}
                      </span>
                    </span>
                    <span className="mt-1 block text-sm text-slate-600">{n.body}</span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </Card>
      )}
      {items && hasMore && (
        <div className="text-center">
          <Button variant="secondary" onClick={() => load(items.length, onlyUnread)}>
            Show more
          </Button>
        </div>
      )}
    </div>
  );
}
