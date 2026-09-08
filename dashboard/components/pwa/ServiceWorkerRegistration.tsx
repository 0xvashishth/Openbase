"use client";

import * as React from "react";

export function ServiceWorkerRegistration() {
  React.useEffect(() => {
    if (!("serviceWorker" in navigator)) return;
    const registerSW = async () => {
      try {
        const registration = await navigator.serviceWorker.register("/sw.js", {
          scope: "/",
        });
        registration.addEventListener("updatefound", () => {
          const newWorker = registration.installing;
          if (!newWorker) return;
          newWorker.addEventListener("statechange", () => {
            if (newWorker.state === "installed" && navigator.serviceWorker.controller) {
              if (window.confirm("A new version of Openbase is available. Refresh to update?")) {
                window.location.reload();
              }
            }
          });
        });
      } catch (error) {
        console.error("[PWA] Service Worker registration failed:", error);
      }
    };
    registerSW();
  }, []);
  return null;
}
