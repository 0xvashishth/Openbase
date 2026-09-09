"use client";

import { useParams } from "next/navigation";
import { AuthPanel } from "@/components/projects/AuthPanel";
import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { PageShell } from "@/components/layout/PageShell";

export default function AuthPage() {
  const params = useParams<{ projectId: string }>();
  return (
    <PageShell>
      <ProjectGuard>
        <AuthPanel projectId={params.projectId} />
      </ProjectGuard>
    </PageShell>
  );
}
