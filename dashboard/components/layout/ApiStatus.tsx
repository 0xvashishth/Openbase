"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import { authToken } from "@/components/AuthProvider";
import { cn } from "@/lib/utils";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

export type ApiHealth = "checking" | "operational" | "degraded" | "down";

export interface ApiCheck {
  label: string;
  ok: boolean;
  ms: number | null;
}

const POLL_MS = 30000;
const SLOW_MS = 1500;

async function timed<T>(fn: () => Promise<T>): Promise<{ ok: boolean; ms: number }> {
  const start = performance.now();
  try {
    await fn();
    return { ok: true, ms: Math.round(performance.now() - start) };
  } catch {
    return { ok: false, ms: Math.round(performance.now() - start) };
  }
}

/**
 * Polls the platform API and reports overall health.
 * The first check is the unauthenticated liveness probe (GET /healthz), so
 * the badge works logged-out too; Auth/Data checks run when a token exists.
 * Never flashes "down" on mount — starts as "checking" until the first
 * round of checks settles.
 */
export function useApiStatus(enabled = true): { health: ApiHealth; latency: number | null; checks: ApiCheck[] } {
  const [health, setHealth] = useState<ApiHealth>("checking");
  const [latency, setLatency] = useState<number | null>(null);
  const [checks, setChecks] = useState<ApiCheck[]>([]);
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);

  const run = useCallback(async () => {
    const platform = await timed(() => api.health());
    const next: ApiCheck[] = [
      { label: "Platform", ok: platform.ok, ms: platform.ok ? platform.ms : null },
    ];
    const token = authToken();
    if (token) {
      const [auth, data] = await Promise.all([
        timed(() => api.me(token)),
        timed(() => api.listOrgs(token)),
      ]);
      next.push(
        { label: "Auth", ok: auth.ok, ms: auth.ok ? auth.ms : null },
        { label: "API", ok: data.ok, ms: data.ok ? data.ms : null },
      );
    }
    setChecks(next);
    if (next.every((c) => c.ok)) {
      const worst = Math.max(...next.map((c) => c.ms ?? 0));
      setLatency(worst);
      setHealth(worst >= SLOW_MS ? "degraded" : "operational");
    } else if (next.some((c) => c.ok)) {
      setLatency(null);
      setHealth("degraded");
    } else {
      setLatency(null);
      setHealth("down");
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;
    void run();
    timer.current = setInterval(() => void run(), POLL_MS);
    return () => {
      if (timer.current) clearInterval(timer.current);
    };
  }, [enabled, run]);

  return { health, latency, checks };
}

/**
 * Liveness dot fills.
 *
 * `degraded` has no amber to reach for — DESIGN.md's palette has no warning
 * color, and using the acid-lime accent would break the one-chromatic-element
 * rule. Signal Teal is the spec's "informational" accent, which reads as
 * "something to look at" without claiming the severity of Coral Red.
 * (Previously `bg-warning`, which compiled to nothing after the token was
 * removed — the dot rendered with no fill at all.)
 */
const dot: Record<ApiHealth, string> = {
  checking: "bg-muted-foreground",
  operational: "bg-success",
  degraded: "bg-info",
  down: "bg-destructive",
};

const label: Record<ApiHealth, string> = {
  checking: "Checking…",
  operational: "Operational",
  degraded: "Degraded",
  down: "Unreachable",
};

/** Compact status pill for the slim top bar, with per-API breakdown in a tooltip. */
export function ApiStatus() {
  const { health, latency, checks } = useApiStatus();
  return <ApiStatusBadge health={health} latency={latency} checks={checks} />;
}

export function ApiStatusBadge({
  health,
  latency,
  checks,
}: {
  health: ApiHealth;
  latency: number | null;
  checks: ApiCheck[];
}) {
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            role="status"
            tabIndex={0}
            aria-label={`API status: ${label[health]}`}
            className="inline-flex cursor-default items-center gap-1.5 whitespace-nowrap text-label text-muted-foreground outline-none focus-visible:ring-1 focus-visible:ring-ring"
          >
            <span aria-hidden className={cn("h-1.5 w-1.5 shrink-0 rounded-full", dot[health])} />
            {label[health]}
            {health === "operational" && latency != null && (
              <span className="font-mono tabular-nums">{latency}ms</span>
            )}
          </span>
        </TooltipTrigger>
        <TooltipContent side="bottom" className="min-w-44">
          {checks.length > 0 ? (
            <ul className="space-y-1">
              {checks.map((c) => (
                <li key={c.label} className="flex items-center gap-2 text-label">
                  <span
                    aria-hidden
                    className={cn(
                      "h-1.5 w-1.5 shrink-0 rounded-full",
                      c.ok ? "bg-success" : "bg-destructive"
                    )}
                  />
                  <span className="font-w510 text-popover-foreground">{c.label}</span>
                  <span className="ml-auto font-mono tabular-nums text-muted-foreground">
                    {c.ok && c.ms != null ? `${c.ms}ms` : "failing"}
                  </span>
                </li>
              ))}
            </ul>
          ) : (
            <span className="text-label text-muted-foreground">Probing platform APIs…</span>
          )}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
