"use client";

import { useEffect, useRef, useState, useCallback } from "react";

interface SwipeOptions {
  onSwipeLeft?: () => void;
  onSwipeRight?: () => void;
  onSwipeUp?: () => void;
  onSwipeDown?: () => void;
  threshold?: number;
  preventScroll?: boolean;
}

interface SwipeState {
  startX: number;
  startY: number;
  currentX: number;
  currentY: number;
  isSwiping: boolean;
}

export function useSwipeGesture(elementRef: React.RefObject<HTMLElement | null>, options: SwipeOptions) {
  const { onSwipeLeft, onSwipeRight, onSwipeUp, onSwipeDown, threshold = 50, preventScroll = false } = options;
  const stateRef = useRef<SwipeState>({
    startX: 0,
    startY: 0,
    currentX: 0,
    currentY: 0,
    isSwiping: false,
  });

  const handleTouchStart = useCallback((e: TouchEvent) => {
    const touch = e.touches[0];
    stateRef.current = {
      startX: touch.clientX,
      startY: touch.clientY,
      currentX: touch.clientX,
      currentY: touch.clientY,
      isSwiping: true,
    };
  }, []);

  const handleTouchMove = useCallback((e: TouchEvent) => {
    if (!stateRef.current.isSwiping) return;
    const touch = e.touches[0];
    stateRef.current.currentX = touch.clientX;
    stateRef.current.currentY = touch.clientY;

    if (preventScroll) {
      const deltaX = Math.abs(stateRef.current.currentX - stateRef.current.startX);
      const deltaY = Math.abs(stateRef.current.currentY - stateRef.current.startY);
      if (deltaX > 10 || deltaY > 10) e.preventDefault();
    }
  }, [preventScroll]);

  const handleTouchEnd = useCallback(() => {
    if (!stateRef.current.isSwiping) return;
    const { startX, startY, currentX, currentY } = stateRef.current;
    const deltaX = currentX - startX;
    const deltaY = currentY - startY;
    const absX = Math.abs(deltaX);
    const absY = Math.abs(deltaY);

    stateRef.current.isSwiping = false;

    if (absX < threshold && absY < threshold) return;

    if (absX > absY) {
      if (deltaX > 0) onSwipeRight?.();
      else onSwipeLeft?.();
    } else {
      if (deltaY > 0) onSwipeDown?.();
      else onSwipeUp?.();
    }
  }, [onSwipeLeft, onSwipeRight, onSwipeUp, onSwipeDown, threshold]);

  useEffect(() => {
    const el = elementRef.current;
    if (!el) return;

    el.addEventListener("touchstart", handleTouchStart, { passive: !preventScroll });
    el.addEventListener("touchmove", handleTouchMove, { passive: !preventScroll });
    el.addEventListener("touchend", handleTouchEnd, { passive: true });

    return () => {
      el.removeEventListener("touchstart", handleTouchStart);
      el.removeEventListener("touchmove", handleTouchMove);
      el.removeEventListener("touchend", handleTouchEnd);
    };
  }, [elementRef, handleTouchStart, handleTouchMove, handleTouchEnd, preventScroll]);

  return stateRef.current;
}

interface EdgeSwipeOptions {
  onEdgeSwipe?: () => void;
  edgeWidth?: number;
  direction?: "left" | "right";
}

export function useEdgeSwipe(elementRef: React.RefObject<HTMLElement | null>, options: EdgeSwipeOptions) {
  const { onEdgeSwipe, edgeWidth = 24, direction = "left" } = options;
  const [isEdgeSwipe, setIsEdgeSwipe] = useState(false);

  const handleTouchStart = useCallback((e: TouchEvent) => {
    const touch = e.touches[0];
    const rect = elementRef.current?.getBoundingClientRect();
    if (!rect) return;

    const isLeftEdge = touch.clientX - rect.left < edgeWidth;
    const isRightEdge = rect.right - touch.clientX < edgeWidth;

    if ((direction === "left" && isLeftEdge) || (direction === "right" && isRightEdge)) {
      setIsEdgeSwipe(true);
    }
  }, [direction, edgeWidth]);

  const handleTouchMove = useCallback((e: TouchEvent) => {
    if (!isEdgeSwipe) return;
    const touch = e.touches[0];
    const rect = elementRef.current?.getBoundingClientRect();
    if (!rect) return;

    const deltaX = direction === "left" ? touch.clientX - rect.left : rect.right - touch.clientX;

    if (deltaX > edgeWidth * 3) {
      onEdgeSwipe?.();
      setIsEdgeSwipe(false);
    }
  }, [direction, edgeWidth, isEdgeSwipe, onEdgeSwipe]);

  const handleTouchEnd = useCallback(() => {
    setIsEdgeSwipe(false);
  }, []);

  useEffect(() => {
    const el = elementRef.current;
    if (!el) return;

    el.addEventListener("touchstart", handleTouchStart, { passive: true });
    el.addEventListener("touchmove", handleTouchMove, { passive: true });
    el.addEventListener("touchend", handleTouchEnd, { passive: true });

    return () => {
      el.removeEventListener("touchstart", handleTouchStart);
      el.removeEventListener("touchmove", handleTouchMove);
      el.removeEventListener("touchend", handleTouchEnd);
    };
  }, [elementRef, handleTouchStart, handleTouchMove, handleTouchEnd]);

  return isEdgeSwipe;
}

export function usePullToRefresh(onRefresh: () => Promise<void>, options: { threshold?: number; resistance?: number } = {}) {
  const { threshold = 80, resistance = 2.5 } = options;
  const [isPulling, setIsPulling] = useState(false);
  const [pullDistance, setPullDistance] = useState(0);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const startYRef = useRef(0);

  const handleTouchStart = useCallback((e: TouchEvent) => {
    if (isRefreshing) return;
    const scrollTop = window.scrollY || document.documentElement.scrollTop;
    if (scrollTop > 0) return;
    startYRef.current = e.touches[0].clientY;
    setIsPulling(true);
  }, [isRefreshing]);

  const handleTouchMove = useCallback((e: TouchEvent) => {
    if (!isPulling || isRefreshing) return;
    const currentY = e.touches[0].clientY;
    const distance = (currentY - startYRef.current) / resistance;
    if (distance > 0) {
      setPullDistance(Math.min(distance, threshold * 1.5));
      e.preventDefault();
    }
  }, [isPulling, isRefreshing, resistance, threshold]);

  const handleTouchEnd = useCallback(async () => {
    if (!isPulling || isRefreshing) return;
    setIsPulling(false);

    if (pullDistance >= threshold) {
      setIsRefreshing(true);
      try {
        await onRefresh();
      } finally {
        setIsRefreshing(false);
      }
    }
    setPullDistance(0);
  }, [isPulling, isRefreshing, pullDistance, threshold, onRefresh]);

  useEffect(() => {
    window.addEventListener("touchstart", handleTouchStart, { passive: true });
    window.addEventListener("touchmove", handleTouchMove, { passive: false });
    window.addEventListener("touchend", handleTouchEnd, { passive: true });

    return () => {
      window.removeEventListener("touchstart", handleTouchStart);
      window.removeEventListener("touchmove", handleTouchMove);
      window.removeEventListener("touchend", handleTouchEnd);
    };
  }, [handleTouchStart, handleTouchMove, handleTouchEnd]);

  return { isPulling, pullDistance, isRefreshing, progress: Math.min(pullDistance / threshold, 1) };
}