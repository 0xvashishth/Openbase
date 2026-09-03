"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { useAuth } from "@/components/AuthProvider";
import { AppShell } from "@/components/layout/AppShell";
import { AppShellSkeleton } from "@/components/ui/skeletons";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth();
  const router = useRouter();

  useEffect(() => {
    if (!loading && !user) {
      router.replace("/login");
    }
  }, [loading, user, router]);

  if (loading || !user) {
    return (
      <div className="h-full bg-background">
        <AppShellSkeleton label="Loading workspace" />
      </div>
    );
  }

  return (
    <div className="h-full">
      <AppShell>{children}</AppShell>
    </div>
  );
}