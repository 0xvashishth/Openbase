import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";

/**
 * Top-level 404 (Phase 8.10): unknown routes previously fell through to
 * Next's default page outside the design system.
 */
export default function NotFound() {
  return (
    <div className="mx-auto flex max-w-xl flex-col items-center gap-3 px-4 py-16 text-center">
      <h2 className="text-caption font-w510 text-foreground-strong">Page not found</h2>
      <p className="text-label text-muted-foreground">
        This URL doesn&apos;t match anything in the dashboard. Check the address, or head
        back to safety.
      </p>
      <Link href="/orgs" className={buttonVariants()}>
        Back to organizations
      </Link>
    </div>
  );
}
