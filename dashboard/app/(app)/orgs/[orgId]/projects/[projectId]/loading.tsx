import { ProjectOverviewSkeleton } from "@/components/ui/skeletons";

export default function ProjectLoading() {
  return (
    <div>
      <div className="border-b border-border bg-background px-6 py-5">
        <div className="h-6 w-48 animate-pulse rounded-md bg-muted" />
      </div>
      <div className="mx-auto max-w-6xl px-6 py-6">
        <ProjectOverviewSkeleton />
      </div>
    </div>
  );
}
