import { CardGridSkeleton } from "@/components/ui/skeletons";
import { Skeleton } from "@/components/ui/skeleton";

export default function AppLoading() {
  return (
    <div>
      <header className="border-b border-border bg-background px-6 py-5">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="mt-1 h-4 w-64" />
      </header>
      <div className="mx-auto max-w-6xl px-6 py-6">
        <CardGridSkeleton count={6} label="Loading page" />
      </div>
    </div>
  );
}
