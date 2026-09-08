"use client";

import { useCallback } from "react";

type HapticPattern =
  | "light"
  | "medium"
  | "heavy"
  | "selection"
  | "success"
  | "warning"
  | "error";

const PATTERNS: Record<HapticPattern, number[]> = {
  light: [10],
  medium: [20],
  heavy: [40],
  selection: [5],
  success: [15, 50, 15],
  warning: [30, 20, 30],
  error: [50, 50, 50],
};

export function useHaptics() {
  const vibrate = useCallback((pattern: HapticPattern | number[]) => {
    if (typeof navigator === "undefined" || !navigator.vibrate) return;
    const p = Array.isArray(pattern) ? pattern : PATTERNS[pattern];
    navigator.vibrate(p);
  }, []);

  const light = useCallback(() => vibrate("light"), [vibrate]);
  const medium = useCallback(() => vibrate("medium"), [vibrate]);
  const heavy = useCallback(() => vibrate("heavy"), [vibrate]);
  const selection = useCallback(() => vibrate("selection"), [vibrate]);
  const success = useCallback(() => vibrate("success"), [vibrate]);
  const warning = useCallback(() => vibrate("warning"), [vibrate]);
  const error = useCallback(() => vibrate("error"), [vibrate]);

  return { vibrate, light, medium, heavy, selection, success, warning, error };
}

export function useTouchFeedback() {
  const { light, medium, heavy, selection, success, warning, error } = useHaptics();

  const onTap = useCallback((level: HapticPattern = "light") => {
    if (level === "light") light();
    else if (level === "medium") medium();
    else if (level === "heavy") heavy();
  }, [light, medium, heavy]);

  const onPress = useCallback(() => selection(), [selection]);
  const onSuccess = useCallback(() => success(), [success]);
  const onError = useCallback(() => error(), [error]);
  const onWarning = useCallback(() => warning(), [warning]);

  return { onTap, onPress, onSuccess, onError, onWarning };
}

export function light() { if (typeof navigator !== "undefined" && navigator.vibrate) navigator.vibrate(10); }
export function medium() { if (typeof navigator !== "undefined" && navigator.vibrate) navigator.vibrate(20); }
export function heavy() { if (typeof navigator !== "undefined" && navigator.vibrate) navigator.vibrate(40); }
export function selection() { if (typeof navigator !== "undefined" && navigator.vibrate) navigator.vibrate(5); }
export function success() { if (typeof navigator !== "undefined" && navigator.vibrate) navigator.vibrate([15, 50, 15]); }
export function warning() { if (typeof navigator !== "undefined" && navigator.vibrate) navigator.vibrate([30, 20, 30]); }
export function error() { if (typeof navigator !== "undefined" && navigator.vibrate) navigator.vibrate([50, 50, 50]); }