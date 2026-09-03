"use client";

import { useParams } from "next/navigation";
import { TriggersPanel } from "@/components/projects/TriggersPanel";
import { ProjectGuard, ProjectHeader } from "@/components/projects/ProjectGuard";

export default function TriggersPage() {
  const params = useParams<{ projectId: string }>();
  return (
    <div>
      <ProjectHeader title="Triggers" subtitle="When X happens on a table, run a function or webhook." />
      <div className="mx-auto max-w-6xl px-6 py-6">
        <ProjectGuard requireConnection requireTriggers toolName="Triggers">
          <TriggersPanel projectId={params.projectId} />
        </ProjectGuard>
      </div>
    </div>
  );
}
