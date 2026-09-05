"use client";

import * as React from "react";
import * as LabelPrimitive from "@radix-ui/react-label";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

/**
 * Label — 12px/510 sits a step below the 13px control text it names, so the
 * field value stays the dominant element in a form row.
 */
const labelVariants = cva(
  "text-label font-w510 leading-none text-foreground peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
);

const Label = React.forwardRef<
  React.ElementRef<typeof LabelPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof LabelPrimitive.Root> & VariantProps<typeof labelVariants>
>(({ className, ...props }, ref) => (
  <LabelPrimitive.Root ref={ref} className={cn(labelVariants(), "mb-1.5 block", className)} {...props} />
));
Label.displayName = LabelPrimitive.Root.displayName;

export { Label };
