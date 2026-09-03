import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "next-themes";
import { describe, expect, it } from "vitest";
import { ThemeToggle } from "./theme-toggle";

function renderToggle() {
  return render(
    <ThemeProvider attribute="class" defaultTheme="light">
      <ThemeToggle />
    </ThemeProvider>
  );
}

describe("ThemeToggle", () => {
  it("renders an accessible switch", () => {
    renderToggle();
    expect(screen.getByRole("switch")).toHaveAccessibleName(/theme/i);
  });

  it("toggles theme on click without crashing", async () => {
    const user = userEvent.setup();
    renderToggle();
    await user.click(screen.getByRole("switch"));
    expect(screen.getByRole("switch")).toBeInTheDocument();
  });
});
