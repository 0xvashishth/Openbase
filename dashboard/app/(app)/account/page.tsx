"use client";

import { AccountPanel } from "@/components/account/AccountPanel";
import { PageShell } from "@/components/layout/PageShell";

export default function AccountPage() {
  return (
    <PageShell>
      <p className="mb-4 text-caption text-muted-foreground">
        Profile, password, and active sessions.
      </p>
      <AccountPanel />
    </PageShell>
  );
}
