"use client";

import { EmailSettingsPanel } from "@/components/admin/EmailSettingsPanel";
import { PageShell } from "@/components/layout/PageShell";

export default function EmailSettingsPage() {
  return (
    <PageShell>
      <p className="mb-4 text-caption text-muted-foreground">
        Your own mail provider for verification, reset and invite emails.
      </p>
      <EmailSettingsPanel />
    </PageShell>
  );
}
