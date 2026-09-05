"use client";

import { OrgSettingsPanel } from "@/components/orgs/OrgSettingsPanel";
import { PageShell } from "@/components/layout/PageShell";

export default function OrgSettingsPage() {
  return (
    <PageShell>
      <OrgSettingsPanel />
    </PageShell>
  );
}
