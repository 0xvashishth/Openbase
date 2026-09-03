import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

/**
 * Merge class names, shadcn-style.
 * Handles conditional classes + Tailwind conflicts (e.g. px-2 vs px-4).
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
