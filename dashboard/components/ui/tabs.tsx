"use client";

import * as React from "react";
import * as TabsPrimitive from "@radix-ui/react-tabs";
import { cn } from "@/lib/utils";

const Tabs = TabsPrimitive.Root;

/**
 * Tabs — an underlined rail rather than shadcn's filled pill tray.
 *
 * A filled tray at 12px radius fights the 12px card it usually sits inside, and
 * the spec's nav language is typographic (§ Nav Text Button: "pure typographic
 * nav with underline on hover"). `min-h-9` rather than `h-9` so a wrapping
 * `TabsList` (see ConnectPanel) grows instead of overflowing.
 */
const TabsList = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.List>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.List>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.List
    ref={ref}
    className={cn(
      "inline-flex min-h-9 items-center justify-start gap-1 border-b border-border text-muted-foreground",
      className
    )}
    {...props}
  />
));
TabsList.displayName = TabsPrimitive.List.displayName;

const TabsTrigger = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.Trigger>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.Trigger>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.Trigger
    ref={ref}
    className={cn(
      "-mb-px inline-flex items-center justify-center whitespace-nowrap border-b-2 border-transparent px-2.5 py-1.5 text-caption font-normal transition-colors",
      "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
      "disabled:pointer-events-none disabled:opacity-50",
      "hover:text-foreground",
      // The active tab is the one nav-scale use of the accent (§ Colors:
      // "active nav indicators"). An indicator, not a fill, so it never reads
      // as a second primary action.
      "data-[state=active]:border-primary data-[state=active]:font-w510 data-[state=active]:text-foreground-strong",
      className
    )}
    {...props}
  />
));
TabsTrigger.displayName = TabsPrimitive.Trigger.displayName;

const TabsContent = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.Content>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.Content
    ref={ref}
    className={cn("mt-4 focus-visible:outline-none", className)}
    {...props}
  />
));
TabsContent.displayName = TabsPrimitive.Content.displayName;

export { Tabs, TabsList, TabsTrigger, TabsContent };
