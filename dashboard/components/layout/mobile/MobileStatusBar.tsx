"use client";

import * as React from "react";

export function MobileStatusBar() {
  return (
    <div
      className="fixed left-0 right-0 top-0 z-30 h-[var(--safe-top)] border-b border-border bg-card/80 backdrop-blur-sm md:hidden"
      aria-hidden="true"
    />
  );
}
