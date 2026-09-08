"use client";

import { useEffect, useState } from "react";

export interface SafeAreaInsets {
  top: number;
  right: number;
  bottom: number;
  left: number;
}

export function useSafeArea(): SafeAreaInsets {
  const [insets, setInsets] = useState<SafeAreaInsets>({
    top: 0,
    right: 0,
    bottom: 0,
    left: 0,
  });

  useEffect(() => {
    const updateInsets = () => {
      const style = getComputedStyle(document.documentElement);
      const getInset = (varName: string) => {
        const value = style.getPropertyValue(varName).trim();
        return value ? parseInt(value.replace("px", ""), 10) : 0;
      };

      setInsets({
        top: getInset("--safe-top"),
        right: getInset("--safe-right"),
        bottom: getInset("--safe-bottom"),
        left: getInset("--safe-left"),
      });
    };

    updateInsets();
    window.addEventListener("resize", updateInsets);
    window.addEventListener("orientationchange", updateInsets);
    return () => {
      window.removeEventListener("resize", updateInsets);
      window.removeEventListener("orientationchange", updateInsets);
    };
  }, []);

  return insets;
}

export function useVisualViewport(): {
  width: number;
  height: number;
  offsetTop: number;
  offsetLeft: number;
  scale: number;
} {
  const [viewport, setViewport] = useState({
    width: window.innerWidth,
    height: window.innerHeight,
    offsetTop: 0,
    offsetLeft: 0,
    scale: 1,
  });

  useEffect(() => {
    const vp = window.visualViewport;
    if (!vp) return;

    const update = () => {
      setViewport({
        width: vp.width,
        height: vp.height,
        offsetTop: vp.offsetTop,
        offsetLeft: vp.offsetLeft,
        scale: vp.scale,
      });
    };

    update();
    vp.addEventListener("resize", update);
    vp.addEventListener("scroll", update);
    return () => {
      vp.removeEventListener("resize", update);
      vp.removeEventListener("scroll", update);
    };
  }, []);

  return viewport;
}