import { ProjectOverviewSkeleton } from "@/components/ui/skeletons";
import { PageShell } from "@/components/layout/PageShell";

export default function ProjectLoading() {
  return (
    <PageShell>
      <ProjectOverviewSkeleton />
    </PageShell>
  );
}
