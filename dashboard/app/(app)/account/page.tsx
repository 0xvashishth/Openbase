"use client";

import { AccountPanel } from "@/components/account/AccountPanel";
import { PageShell } from "@/components/layout/PageShell";

export default function AccountPage() {
  return (
    <PageShell>
      <div className="mb-5">
        <h1 className="text-heading font-w510 text-foreground-strong">Account settings</h1>
        <p className="mt-1 text-caption text-muted-foreground">Manage your profile, security, and active sessions.</p>
      </div>
      <AccountPanel />
    </PageShell>
  );
}
