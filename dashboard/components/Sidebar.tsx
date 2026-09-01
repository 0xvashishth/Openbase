"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useAuth } from "@/components/AuthProvider";
import type { ReactNode } from "react";

function NavItem({
  href,
  active,
  children,
}: {
  href: string;
  active: boolean;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      className={`flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm font-medium transition-colors ${
        active
          ? "bg-brand-50 text-brand-700"
          : "text-slate-600 hover:bg-slate-100 hover:text-slate-900"
      }`}
    >
      {children}
    </Link>
  );
}

const navSections = [
  {
    label: "Platform",
    items: [
      { href: "/orgs", label: "Organizations", icon: "◧" },
      { href: "/projects", label: "Projects", icon: "▦" },
    ],
  },
  {
    label: "Per-project (coming in later phases)",
    items: [
      { href: "", label: "Table Editor", icon: "▤" },
      { href: "", label: "Schema", icon: "⬡" },
      { href: "", label: "API", icon: "↗" },
      { href: "", label: "Realtime", icon: "◉" },
      { href: "", label: "Trigger Builder", icon: "⚙" },
    ],
  },
];

export function Sidebar({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { user, logout } = useAuth();

  return (
    <div className="flex h-full">
      <aside className="flex w-60 shrink-0 flex-col border-r border-slate-200 bg-slate-50">
        <div className="flex h-14 items-center gap-2 border-b border-slate-200 px-4">
          <div className="flex h-7 w-7 items-center justify-center rounded-md bg-brand-600 text-white text-sm font-bold">
            O
          </div>
          <span className="text-sm font-semibold text-slate-900">Openbase</span>
        </div>

        <nav className="flex-1 space-y-4 overflow-y-auto px-3 py-3">
          {navSections.map((section) => (
            <div key={section.label}>
              <p className="px-2.5 pb-1 text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                {section.label}
              </p>
              <div className="space-y-0.5">
                {section.items.map((item) => {
                  const active = item.href !== "" && pathname?.startsWith(item.href);
                  return item.href ? (
                    <NavItem key={item.label} href={item.href} active={!!active}>
                      <span className="text-xs">{item.icon}</span>
                      {item.label}
                    </NavItem>
                  ) : (
                    <div
                      key={item.label}
                      className="flex cursor-not-allowed items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm font-medium text-slate-400"
                      title="Arrives in a later phase"
                    >
                      <span className="text-xs">{item.icon}</span>
                      {item.label}
                    </div>
                  );
                })}
              </div>
            </div>
          ))}
        </nav>

        <div className="border-t border-slate-200 px-3 py-3">
          <div className="mb-2 flex items-center gap-2.5 px-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-full bg-slate-300 text-xs font-semibold text-slate-700">
              {user?.email?.[0]?.toUpperCase() ?? "?"}
            </div>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-slate-800">{user?.email}</p>
            </div>
          </div>
          <button
            onClick={() => {
              logout();
              router.push("/login");
            }}
            className="w-full rounded-md px-2.5 py-1.5 text-left text-sm font-medium text-slate-600 hover:bg-slate-100"
          >
            Sign out
          </button>
        </div>
      </aside>

      <main className="flex-1 overflow-y-auto bg-slate-50/60">{children}</main>
    </div>
  );
}