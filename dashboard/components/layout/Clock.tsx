"use client";

import { useEffect, useState } from "react";

function formatNow(d: Date): string {
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: true,
    timeZoneName: "short",
  }).format(d);
}

/**
 * Live wall-clock with seconds + timezone abbreviation, e.g.
 * "Sep 4, 12:37:07 PM PDT". Ticks every second, tabular numerals so the
 * status bar doesn't jitter.
 */
export function Clock() {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(t);
  }, []);

  return (
    <span
      role="timer"
      aria-label={`Current time ${formatNow(now)}`}
      title={now.toISOString()}
      className="inline-flex items-center gap-1.5 font-mono tabular-nums text-muted-foreground"
    >
      {formatNow(now)}
    </span>
  );
}
