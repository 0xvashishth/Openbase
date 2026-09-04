import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Clock } from "./Clock";

describe("Clock", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("renders time with seconds and ticks every second", () => {
    vi.setSystemTime(new Date("2026-09-04T12:37:07"));
    render(<Clock />);
    const timer = screen.getByRole("timer");
    expect(timer).toHaveTextContent(/12:37:07/);
    // seconds counter advances
    act(() => vi.advanceTimersByTime(2000));
    expect(screen.getByRole("timer")).toHaveTextContent(/12:37:09/);
  });

  it("includes a timezone abbreviation", () => {
    vi.setSystemTime(new Date("2026-09-04T12:37:07"));
    render(<Clock />);
    // e.g. "Sep 4, 12:37:07 PM UTC/GMT/PDT" depending on TZ env
    expect(screen.getByRole("timer")).toHaveTextContent(/[A-Z]{2,5}$/);
  });
});
