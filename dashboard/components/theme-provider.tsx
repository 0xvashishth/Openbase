"use client";

import * as React from "react";
import { ThemeProvider as NextThemesProvider } from "next-themes";

export function ThemeProvider({
  children,
  ...props
}: React.ComponentProps<typeof NextThemesProvider>) {
  return (
    // DESIGN.md specifies a dark system ("Theme: dark"), so dark is the
    // default rather than following the OS. The light palette is a derivation
    // (see app/globals.css) and remains available via the theme toggle.
    <NextThemesProvider attribute="class" defaultTheme="dark" enableSystem {...props}>
      {children}
    </NextThemesProvider>
  );
}
