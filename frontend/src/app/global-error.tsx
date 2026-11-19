"use client";

import { THEME_SCRIPT } from "@/lib/theme";

// Last resort when the root layout itself fails. It replaces the whole page,
// so it can't use the site's stylesheet or components: everything is inline,
// with the saved theme applied by the same script the layout uses.
export default function GlobalError({ retry }: { error: unknown; retry: () => void }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <title>Something went wrong · Digital Healthcare Platform</title>
        <script dangerouslySetInnerHTML={{ __html: THEME_SCRIPT }} />
        <style>{`
          :root { color-scheme: light; --bg: #f8fafc; --card: #fff; --text: #0f172a; --muted: #475569; --line: #e2e8f0; }
          [data-theme="dark"] { color-scheme: dark; --bg: #0b1220; --card: #111c2e; --text: #f1f5f9; --muted: #a8b4c6; --line: #2a3851; }
          body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: var(--bg); color: var(--text);
                 font-family: system-ui, -apple-system, "Segoe UI", sans-serif; padding: 16px; }
          .card { max-width: 28rem; background: var(--card); border: 1px solid var(--line); border-radius: 12px; padding: 32px; text-align: center; }
          h1 { font-size: 1.25rem; margin: 8px 0; }
          p { color: var(--muted); font-size: 0.9rem; line-height: 1.5; }
          .row { display: flex; gap: 8px; justify-content: center; margin-top: 20px; flex-wrap: wrap; }
          button, a { font: inherit; font-size: 0.875rem; font-weight: 500; padding: 8px 16px; border-radius: 8px; cursor: pointer; text-decoration: none; }
          button { background: #0f766e; color: #fff; border: 0; }
          a { border: 1px solid var(--line); color: var(--text); }
        `}</style>
      </head>
      <body>
        <div className="card">
          <div style={{ fontSize: "2.25rem" }} aria-hidden>
            ⚠️
          </div>
          <h1>Something went wrong</h1>
          <p>The site hit an unexpected problem. Please try again in a moment.</p>
          <div className="row">
            <button onClick={() => retry()}>Try again</button>
            {/* A full page load, since the app itself may be broken. */}
            {/* eslint-disable-next-line @next/next/no-html-link-for-pages */}
            <a href="/">Home</a>
          </div>
        </div>
      </body>
    </html>
  );
}
