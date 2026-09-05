"use client";

import { useParams } from "next/navigation";
import { DBSourcePanel } from "@/components/projects/DBSourcePanel";
import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { PageShell } from "@/components/layout/PageShell";

export default function DBSourcePage() {
  const params = useParams<{ projectId: string }>();
  return (
    <PageShell>
      <ProjectGuard>
        <DBSourcePanel projectId={params.projectId} />
      </ProjectGuard>
    </PageShell>
  );
}
