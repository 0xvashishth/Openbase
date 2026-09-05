"use client";

import { useParams } from "next/navigation";
import { APIKeysPanel } from "@/components/projects/APIKeysPanel";
import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { PageShell } from "@/components/layout/PageShell";

export default function ApiPage() {
  const params = useParams<{ projectId: string }>();
  return (
    <PageShell>
      <ProjectGuard>
        <APIKeysPanel projectId={params.projectId} />
      </ProjectGuard>
    </PageShell>
  );
}
