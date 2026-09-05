"use client";

import { useParams } from "next/navigation";
import { ConnectPanel } from "@/components/projects/ConnectPanel";
import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { PageShell } from "@/components/layout/PageShell";

export default function ConnectPage() {
  const params = useParams<{ projectId: string }>();
  return (
    <PageShell>
      <ProjectGuard>
        <ConnectPanel projectId={params.projectId} />
      </ProjectGuard>
    </PageShell>
  );
}
