"use client";

import { useParams } from "next/navigation";
import { FunctionsPanel } from "@/components/projects/FunctionsPanel";
import { ProjectGuard } from "@/components/projects/ProjectGuard";

export default function FunctionsPage() {
  const params = useParams<{ projectId: string }>();
  return (
    <div className="mx-auto max-w-6xl px-6 py-6">
      <ProjectGuard requireConnection toolName="Functions">
        <FunctionsPanel projectId={params.projectId} />
      </ProjectGuard>
    </div>
  );
}
