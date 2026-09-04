"use client";

import { useParams } from "next/navigation";
import { RealtimeDemo } from "@/components/data/RealtimeDemo";
import { ProjectGuard } from "@/components/projects/ProjectGuard";

export default function RealtimePage() {
  const params = useParams<{ projectId: string }>();
  return (
    <div className="mx-auto max-w-6xl px-6 py-6">
      <ProjectGuard requireConnection requireRealtime toolName="Realtime">
        <RealtimeDemo projectId={params.projectId} />
      </ProjectGuard>
    </div>
  );
}
