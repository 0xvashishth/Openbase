"use client";

import { useParams } from "next/navigation";
import { SchemaExplorer } from "@/components/schema/SchemaExplorer";
import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { useProject } from "@/lib/project-context";
import { PageShell } from "@/components/layout/PageShell";

function SchemaBody({ projectId }: { projectId: string }) {
  const { engine } = useProject();
  return <SchemaExplorer projectId={projectId} engine={engine ?? "unknown"} />;
}

export default function SchemaPage() {
  const params = useParams<{ orgId: string; projectId: string }>();
  return (
    <PageShell>
      <ProjectGuard requireConnection toolName="Schema">
        <SchemaBody projectId={params.projectId} />
      </ProjectGuard>
    </PageShell>
  );
}
