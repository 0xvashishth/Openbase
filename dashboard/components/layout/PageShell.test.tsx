import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PageShell } from "./PageShell";

describe("PageShell", () => {
  it("pins content to the 1200px design-system shell", () => {
    render(
      <PageShell data-testid="shell">
        <p>content</p>
      </PageShell>
    );
    const shell = screen.getByTestId("shell");
    expect(shell.className).toMatch(/max-w-shell/);
    expect(screen.getByText("content")).toBeInTheDocument();
  });

  /**
   * Guards against the nesting bug this component was introduced to fix:
   * pages used to declare their own container AND ProjectGuard re-declared one,
   * doubling the padding on every blocked or loading tool page.
   */
  it("lets a caller override padding without losing the width cap", () => {
    render(
      <PageShell className="p-0" data-testid="shell">
        <p>content</p>
      </PageShell>
    );
    const cls = screen.getByTestId("shell").className;
    expect(cls).toMatch(/max-w-shell/);
    expect(cls).toMatch(/(^|\s)p-0(\s|$)/);
    expect(cls).not.toMatch(/py-6/);
  });
});
