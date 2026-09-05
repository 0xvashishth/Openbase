"use client";

import { useParams } from "next/navigation";
import { RealtimeDemo } from "@/components/data/RealtimeDemo";
import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { PageShell } from "@/components/layout/PageShell";

export default function RealtimePage() {
  const params = useParams<{ projectId: string }>();
  return (
    <PageShell>
      <ProjectGuard requireConnection requireRealtime toolName="Realtime">
        <RealtimeDemo projectId={params.projectId} />
      </ProjectGuard>
    </PageShell>
  );
}
