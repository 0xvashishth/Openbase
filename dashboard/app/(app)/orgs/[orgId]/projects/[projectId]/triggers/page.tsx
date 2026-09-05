"use client";

import { useParams } from "next/navigation";
import { TriggersPanel } from "@/components/projects/TriggersPanel";
import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { PageShell } from "@/components/layout/PageShell";

export default function TriggersPage() {
  const params = useParams<{ projectId: string }>();
  return (
    <PageShell>
      <ProjectGuard requireConnection requireTriggers toolName="Triggers">
        <TriggersPanel projectId={params.projectId} />
      </ProjectGuard>
    </PageShell>
  );
}
