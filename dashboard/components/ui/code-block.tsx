"use client";

import * as React from "react";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * Read-only code block with a copy button. Used by the Connect tab for
 * copy-paste snippets. Falls back silently when the Clipboard API is
 * unavailable (non-secure origins, older browsers) — the code stays
 * selectable either way.
 */
export function CodeBlock({
  code,
  label,
  language,
  className,
}: {
  code: string;
  /** Accessible name for the copy button, e.g. "cURL example". */
  label: string;
  /** Shown as a small tag in the header, e.g. "bash". */
  language?: string;
  className?: string;
}) {
  const [copied, setCopied] = React.useState(false);
  const timer = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  React.useEffect(() => {
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, []);

  async function copy() {
    try {
      await navigator.clipboard?.writeText(code);
      setCopied(true);
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard blocked — leave the code for manual selection.
    }
  }

  return (
    <div className={cn("overflow-hidden rounded-lg border border-border bg-card", className)}>
      <div className="flex items-center justify-between gap-2 border-b border-border px-3 py-1.5">
        <span className="font-mono text-micro uppercase text-muted-foreground">
          {language ?? "code"}
        </span>
        <button
          type="button"
          onClick={copy}
          aria-label={`Copy ${label}`}
          className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-label text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        >
          {copied ? (
            <>
              <Check className="h-3.5 w-3.5" aria-hidden /> Copied
            </>
          ) : (
            <>
              <Copy className="h-3.5 w-3.5" aria-hidden /> Copy
            </>
          )}
        </button>
      </div>
      <pre className="overflow-x-auto p-3 text-label leading-[1.7]">
        <code className="font-mono text-foreground">{code}</code>
      </pre>
    </div>
  );
}

/**
 * Single-line copyable value (API URL, project id, key). Renders as a
 * labelled row so the Connect tab reads like Supabase's parameters list.
 */
export function CopyField({
  label,
  value,
  hint,
  mono = true,
}: {
  label: string;
  value: string;
  hint?: string;
  mono?: boolean;
}) {
  const [copied, setCopied] = React.useState(false);
  const timer = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  React.useEffect(() => {
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, []);

  async function copy() {
    try {
      await navigator.clipboard?.writeText(value);
      setCopied(true);
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 1500);
    } catch {
      // ignore
    }
  }

  return (
    <div className="space-y-1">
      <p className="text-label font-w510 text-foreground">{label}</p>
      <div className="flex items-stretch gap-2">
        <code
          className={cn(
            "min-w-0 flex-1 overflow-x-auto whitespace-nowrap rounded-md border border-border bg-foreground/[0.02] px-2 py-1.5 text-label text-foreground",
            mono && "font-mono"
          )}
        >
          {value}
        </code>
        <button
          type="button"
          onClick={copy}
          aria-label={`Copy ${label}`}
          className="inline-flex shrink-0 items-center gap-1 rounded-md border border-border px-2 text-label text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        >
          {copied ? <Check className="h-3.5 w-3.5" aria-hidden /> : <Copy className="h-3.5 w-3.5" aria-hidden />}
          <span className="sr-only">{copied ? "Copied" : "Copy"}</span>
        </button>
      </div>
      {hint && <p className="text-label text-muted-foreground">{hint}</p>}
    </div>
  );
}
