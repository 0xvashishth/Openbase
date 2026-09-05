"use client";

import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { ProjectSettingsPanel } from "@/components/projects/ProjectSettingsPanel";
import { PageShell } from "@/components/layout/PageShell";

export default function SettingsPage() {
  return (
    <PageShell>
      <ProjectGuard>
        <ProjectSettingsPanel />
      </ProjectGuard>
    </PageShell>
  );
}
