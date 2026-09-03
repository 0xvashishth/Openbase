"use client";

import { useParams } from "next/navigation";
import { SchemaExplorer } from "@/components/schema/SchemaExplorer";
import { ProjectGuard, ProjectHeader } from "@/components/projects/ProjectGuard";
import { useProject } from "@/lib/project-context";

function SchemaBody({ projectId }: { projectId: string }) {
  const { engine } = useProject();
  return <SchemaExplorer projectId={projectId} engine={engine ?? "unknown"} />;
}

export default function SchemaPage() {
  const params = useParams<{ orgId: string; projectId: string }>();
  return (
    <div>
      <ProjectHeader title="Schema" subtitle="Visual tables and relationships." />
      <div className="mx-auto max-w-6xl px-6 py-6">
        <ProjectGuard requireConnection toolName="Schema">
          <SchemaBody projectId={params.projectId} />
        </ProjectGuard>
      </div>
    </div>
  );
}
