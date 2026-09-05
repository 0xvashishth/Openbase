"use client";

import { useParams } from "next/navigation";
import { FunctionsPanel } from "@/components/projects/FunctionsPanel";
import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { PageShell } from "@/components/layout/PageShell";

export default function FunctionsPage() {
  const params = useParams<{ projectId: string }>();
  return (
    <PageShell>
      <ProjectGuard requireConnection toolName="Functions">
        <FunctionsPanel projectId={params.projectId} />
      </ProjectGuard>
    </PageShell>
  );
}
