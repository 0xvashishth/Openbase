"use client";

import { useEffect } from "react";
import { Button } from "@/components/ui";

/**
 * Route-segment error boundary (Phase 8.10): a thrown render error in any
 * panel previously blanked the entire shell. This catches it per segment
 * with a retry affordance instead.
 */
export default function AppError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // Surface render crashes in the console for operators; request logging
    // (X-Request-ID) covers the server side.
    // eslint-disable-next-line no-console
    console.error("dashboard segment error:", error);
  }, [error]);

  return (
    <div className="mx-auto flex max-w-xl flex-col items-center gap-3 px-4 py-16 text-center">
      <h2 className="text-caption font-w510 text-foreground-strong">Something went wrong</h2>
      <p className="text-label text-muted-foreground">
        This view hit an unexpected error. Your data is safe — try again, or go back and
        re-enter.
      </p>
      {error.digest && (
        <p className="font-mono text-label text-muted-foreground">ref: {error.digest}</p>
      )}
      <div className="flex gap-2">
        <Button onClick={() => reset()}>Try again</Button>
        <Button variant="outline" onClick={() => (window.location.href = "/orgs")}>
          Back to organizations
        </Button>
      </div>
    </div>
  );
}
