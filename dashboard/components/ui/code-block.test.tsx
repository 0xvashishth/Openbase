import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CodeBlock, CopyField } from "./code-block";

const writeText = vi.fn();

/**
 * userEvent.setup() installs its own navigator.clipboard stub, so the spy has
 * to be attached *after* setup to win.
 */
function setupWithClipboard() {
  const user = userEvent.setup();
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
  return user;
}

describe("CodeBlock", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    writeText.mockResolvedValue(undefined);
  });

  it("renders the code and a labelled copy button", () => {
    render(<CodeBlock code="curl example.com" label="cURL example" language="bash" />);
    expect(screen.getByText("curl example.com")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy cURL example" })).toBeInTheDocument();
    expect(screen.getByText("bash")).toBeInTheDocument();
  });

  it("copies to the clipboard and confirms", async () => {
    const user = setupWithClipboard();
    render(<CodeBlock code="curl example.com" label="cURL example" />);
    await user.click(screen.getByRole("button", { name: "Copy cURL example" }));
    expect(writeText).toHaveBeenCalledWith("curl example.com");
    expect(await screen.findByText("Copied")).toBeInTheDocument();
  });

  it("stays usable when the clipboard is blocked", async () => {
    writeText.mockRejectedValue(new Error("denied"));
    const user = setupWithClipboard();
    render(<CodeBlock code="curl example.com" label="cURL example" />);
    await user.click(screen.getByRole("button", { name: "Copy cURL example" }));
    await waitFor(() => expect(writeText).toHaveBeenCalled());
    expect(screen.queryByText("Copied")).not.toBeInTheDocument();
    expect(screen.getByText("curl example.com")).toBeInTheDocument();
  });
});

describe("CopyField", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    writeText.mockResolvedValue(undefined);
  });

  it("shows label, value and optional hint", () => {
    render(<CopyField label="API URL" value="https://api.example.com" hint="same-origin default" />);
    expect(screen.getByText("API URL")).toBeInTheDocument();
    expect(screen.getByText("https://api.example.com")).toBeInTheDocument();
    expect(screen.getByText("same-origin default")).toBeInTheDocument();
  });

  it("copies the value", async () => {
    const user = setupWithClipboard();
    render(<CopyField label="API URL" value="https://api.example.com" />);
    await user.click(screen.getByRole("button", { name: "Copy API URL" }));
    expect(writeText).toHaveBeenCalledWith("https://api.example.com");
  });
});
