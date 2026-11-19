"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { api, type Role, type Schemas } from "@/lib/api/client";
import { notificationHref, notificationIcon, timeAgo } from "@/lib/notification-links";

type Notification = Schemas["Notification"];

const POLL_MS = 60_000;
const SHOWN = 10;

// Bell with the unread count; opens a panel with the latest notifications.
export function NotificationBell({ role }: { role?: Role }) {
  const router = useRouter();
  const [unread, setUnread] = useState(0);
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<Notification[] | null>(null);
  const panel = useRef<HTMLDivElement>(null);

  const refreshCount = useCallback(async () => {
    const { data } = await api.GET("/api/v1/notifications", { params: { query: { unread: true, limit: 1 } } });
    if (data) setUnread(data.unread_count ?? 0);
  }, []);

  // Count on load, every minute, and when the tab comes back into view.
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    refreshCount();
    const t = setInterval(refreshCount, POLL_MS);
    const onVisible = () => document.visibilityState === "visible" && refreshCount();
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearInterval(t);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [refreshCount]);

  async function toggle() {
    if (open) {
      setOpen(false);
      return;
    }
    setOpen(true);
    const { data } = await api.GET("/api/v1/notifications", { params: { query: { limit: SHOWN } } });
    setItems(data?.notifications ?? []);
    if (data) setUnread(data.unread_count ?? 0);
  }

  // Close on a click outside or Escape.
  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (panel.current && !panel.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  async function openItem(n: Notification) {
    if (!n.read_at && n.id != null) {
      api.POST("/api/v1/notifications/{notificationID}/read", { params: { path: { notificationID: n.id } } });
      setUnread((u) => Math.max(0, u - 1));
    }
    setOpen(false);
    router.push(notificationHref(n.kind, role));
  }

  async function markAll() {
    await api.POST("/api/v1/notifications/read-all");
    setUnread(0);
    setItems((prev) => prev?.map((n) => ({ ...n, read_at: n.read_at ?? new Date().toISOString() })) ?? null);
  }

  return (
    <div ref={panel} className="relative">
      <button
        type="button"
        onClick={toggle}
        className="relative grid h-8 w-8 place-items-center rounded-lg text-white/80 hover:bg-white/10 hover:text-white"
        aria-label={unread ? `Notifications, ${unread} unread` : "Notifications"}
        aria-expanded={open}
        aria-haspopup="dialog"
      >
        <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
          <path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
          <path d="M13.73 21a2 2 0 0 1-3.46 0" />
        </svg>
        {unread > 0 && (
          <span className="absolute -right-1 -top-1 grid h-4 min-w-4 place-items-center rounded-full bg-red-600 px-1 text-[10px] font-bold leading-none text-white">
            {unread > 99 ? "99+" : unread}
          </span>
        )}
      </button>

      {open && (
        <div
          role="dialog"
          aria-label="Notifications"
          className="absolute right-0 top-full z-50 mt-2 w-[min(22rem,calc(100vw-2rem))] overflow-hidden rounded-xl border border-slate-200 bg-surface text-sm shadow-xl"
        >
          <div className="flex items-center justify-between border-b border-slate-100 px-4 py-2.5">
            <span className="font-semibold text-slate-900">Notifications</span>
            {unread > 0 && (
              <button onClick={markAll} className="text-xs text-teal-800 hover:underline">
                Mark all as read
              </button>
            )}
          </div>
          <ul className="max-h-96 divide-y divide-slate-100 overflow-y-auto">
            {items === null && <li className="px-4 py-6 text-center text-slate-500">Loading…</li>}
            {items?.length === 0 && <li className="px-4 py-8 text-center text-slate-500">No notifications yet.</li>}
            {items?.map((n) => (
              <li key={n.id}>
                <button
                  onClick={() => openItem(n)}
                  className={`flex w-full gap-3 px-4 py-3 text-left hover:bg-slate-50 ${n.read_at ? "" : "bg-teal-50"}`}
                >
                  <span className="text-lg leading-none" aria-hidden>
                    {notificationIcon(n.kind)}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-start justify-between gap-2">
                      <span className={`text-slate-900 ${n.read_at ? "" : "font-semibold"}`}>{n.title}</span>
                      {!n.read_at && <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-teal-600" aria-label="unread" />}
                    </span>
                    <span className="mt-0.5 line-clamp-2 block text-slate-600">{n.body}</span>
                    <span className="mt-1 block text-xs text-slate-500">{timeAgo(n.created_at)}</span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
          <Link
            href="/notifications"
            onClick={() => setOpen(false)}
            className="block border-t border-slate-100 px-4 py-2.5 text-center text-xs font-medium text-teal-800 hover:bg-slate-50"
          >
            See all notifications
          </Link>
        </div>
      )}
    </div>
  );
}
