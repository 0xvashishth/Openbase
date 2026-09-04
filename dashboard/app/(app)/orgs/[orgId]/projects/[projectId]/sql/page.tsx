"use client";

import { useParams } from "next/navigation";
import { QueryEditor } from "@/components/data/QueryEditor";
import { ProjectGuard } from "@/components/projects/ProjectGuard";
import { useProject } from "@/lib/project-context";

function EditorBody({ projectId }: { projectId: string }) {
  const { engine } = useProject();
  return <QueryEditor projectId={projectId} engine={engine} />;
}

export default function SqlPage() {
  const params = useParams<{ projectId: string }>();
  return (
    <div className="mx-auto max-w-6xl px-6 py-6">
      <ProjectGuard requireConnection toolName="SQL Editor">
        <EditorBody projectId={params.projectId} />
      </ProjectGuard>
    </div>
  );
}
