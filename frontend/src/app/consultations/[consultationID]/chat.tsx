"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import { Alert, Button } from "@/components/ui";
import { api, errorMessage, type ChatMessage, type Consultation, type User } from "@/lib/api/client";
import { dateTime, fileSize } from "@/lib/format";

// While the consultation is active, new messages arrive live over a Server-Sent
// Events stream relayed by this app's /api/v1 proxy, so the token stays in the
// httpOnly cookie. EventSource reconnects by itself and resumes from the last
// message id. If the stream is refused outright, fall back to polling.
const FALLBACK_POLL_MS = 3000;
const MAX_FILE_BYTES = 10 * 1024 * 1024;

export function Chat({ consultation, me }: { consultation: Consultation; me: User }) {
  const id = consultation.id ?? "";
  const open = consultation.status === "active";
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [error, setError] = useState("");
  const [sending, setSending] = useState(false);
  const [text, setText] = useState("");
  const [live, setLive] = useState(false);
  const [polling, setPolling] = useState(false);
  const lastID = useRef(0);
  const bottom = useRef<HTMLDivElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  function add(incoming: ChatMessage[]) {
    if (incoming.length === 0) return;
    setMessages((prev) => {
      const seen = new Set(prev.map((m) => m.id));
      const merged = [...prev, ...incoming.filter((m) => !seen.has(m.id))];
      lastID.current = Math.max(lastID.current, ...merged.map((m) => m.id ?? 0));
      return merged;
    });
  }

  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setInterval> | undefined;
    let source: EventSource | undefined;

    async function poll() {
      const { data } = await api.GET("/api/v1/consultations/{consultationID}/messages", {
        params: { path: { consultationID: id }, query: { after: lastID.current, limit: 100 } },
      });
      if (stopped || !data) return;
      add(data.messages ?? []);
    }

    if (!open || typeof EventSource === "undefined") {
      // A closed consultation's history, or a browser without EventSource.
      poll();
      if (open) timer = setInterval(poll, FALLBACK_POLL_MS);
    } else {
      source = new EventSource(`/api/v1/consultations/${id}/events?after=${lastID.current}`);
      source.addEventListener("open", () => setLive(true));
      source.addEventListener("message", (e) => {
        try {
          add([JSON.parse((e as MessageEvent<string>).data) as ChatMessage]);
        } catch {
          // ignore a malformed event; the next one or a reconnect catches up
        }
      });
      source.addEventListener("error", () => {
        setLive(false);
        // CONNECTING means EventSource is retrying by itself. CLOSED means the server
        // refused the stream (e.g. logged out); keep the chat working by polling.
        if (source?.readyState === EventSource.CLOSED && !timer) {
          setPolling(true);
          poll();
          timer = setInterval(poll, FALLBACK_POLL_MS);
        }
      });
    }
    return () => {
      stopped = true;
      source?.close();
      clearInterval(timer);
    };
  }, [id, open]);

  useEffect(() => {
    bottom.current?.scrollIntoView({ block: "nearest" });
  }, [messages.length]);

  async function send(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const body = text.trim();
    if (!body) return;
    setSending(true);
    setError("");
    const { data, error } = await api.POST("/api/v1/consultations/{consultationID}/messages", {
      params: { path: { consultationID: id } },
      body: { body },
    });
    setSending(false);
    if (data) {
      setText("");
      add([data]);
    } else setError(errorMessage(error));
  }

  async function upload(file: File) {
    if (file.size > MAX_FILE_BYTES) {
      setError("Files can be at most 10 MB.");
      return;
    }
    setSending(true);
    setError("");
    const form = new FormData();
    form.append("file", file);
    const res = await fetch(`/api/v1/consultations/${id}/attachments`, { method: "POST", body: form });
    setSending(false);
    const data = await res.json().catch(() => null);
    if (res.ok && data) add([data as ChatMessage]);
    else setError(errorMessage(data, "Could not share the file."));
    if (fileInput.current) fileInput.current.value = "";
  }

  return (
    <section className="rounded-xl border border-slate-200 bg-surface shadow-sm">
      {open && (
        <p className="flex items-center gap-2 border-b border-slate-100 px-4 py-2 text-xs text-slate-500" aria-live="polite">
          <span className={`h-2 w-2 rounded-full ${live ? "bg-emerald-500" : "bg-amber-400"}`} aria-hidden />
          {live ? "Live — new messages appear instantly" : polling ? "Checking for new messages every few seconds" : "Connecting…"}
        </p>
      )}
      <div className="max-h-[28rem] min-h-48 space-y-3 overflow-y-auto p-4">
        {messages.length === 0 && (
          <p className="py-8 text-center text-sm text-slate-500">
            {open ? "No messages yet. Say hello." : "No messages were exchanged."}
          </p>
        )}
        {messages.map((m) => {
          const mine = m.sender_id === me.id;
          return (
            <div key={m.id} className={`flex ${mine ? "justify-end" : "justify-start"}`}>
              <div
                className={`max-w-[80%] rounded-2xl px-4 py-2 text-sm ${
                  mine ? "rounded-br-sm bg-teal-700 text-white" : "rounded-bl-sm bg-slate-100 text-slate-900"
                }`}
              >
                {!mine && <p className="mb-0.5 text-xs font-medium text-slate-500">{m.sender_name}</p>}
                {m.body && <p className="whitespace-pre-line break-words">{m.body}</p>}
                {m.attachment && (
                  <a
                    href={`/api/v1/attachments/${m.attachment.id}`}
                    className={`block underline ${mine ? "text-white" : "text-teal-800"}`}
                  >
                    📎 {m.attachment.file_name} ({fileSize(m.attachment.size_bytes)})
                  </a>
                )}
                <p className={`mt-1 text-[11px] ${mine ? "text-white/70" : "text-slate-500"}`}>{dateTime(m.created_at)}</p>
              </div>
            </div>
          );
        })}
        <div ref={bottom} />
      </div>

      {open && (
        <form onSubmit={send} className="space-y-2 border-t border-slate-100 p-3">
          {error && <Alert>{error}</Alert>}
          <div className="flex gap-2">
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  e.currentTarget.form?.requestSubmit();
                }
              }}
              rows={2}
              maxLength={4000}
              placeholder="Write a message…"
              aria-label="Message"
              className="flex-1 resize-none rounded-lg border border-slate-300 px-3 py-2 text-sm outline-none focus:border-teal-600 focus:ring-2 focus:ring-teal-600/20"
            />
            <div className="flex flex-col gap-2">
              <Button type="submit" disabled={sending || !text.trim()}>
                Send
              </Button>
              <Button type="button" variant="secondary" disabled={sending} onClick={() => fileInput.current?.click()}>
                Attach
              </Button>
            </div>
          </div>
          <input
            ref={fileInput}
            type="file"
            accept="application/pdf,image/jpeg,image/png"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) upload(f);
            }}
          />
          <p className="text-xs text-slate-500">Share reports as PDF, JPEG or PNG, up to 10 MB.</p>
        </form>
      )}
    </section>
  );
}
