import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDialog, Dialog, DialogContent, DialogDescription, DialogTitle } from "./dialog";

describe("Dialog (shadcn/Radix)", () => {
  it("renders nothing when closed", () => {
    render(
      <Dialog open={false} onOpenChange={() => {}}>
        <DialogContent>
          <DialogTitle>Hidden</DialogTitle>
        </DialogContent>
      </Dialog>
    );
    expect(screen.queryByText("Hidden")).not.toBeInTheDocument();
  });

  it("renders content in a portal with accessible dialog role", () => {
    render(
      <Dialog open onOpenChange={() => {}}>
        <DialogContent>
          <DialogTitle>Delete project?</DialogTitle>
          <DialogDescription>This cannot be undone.</DialogDescription>
        </DialogContent>
      </Dialog>
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Delete project?")).toBeInTheDocument();
  });

  it("calls onOpenChange(false) on Escape", async () => {
    const onOpenChange = vi.fn();
    const user = userEvent.setup();
    render(
      <Dialog open onOpenChange={onOpenChange}>
        <DialogContent>
          <DialogTitle>Delete project?</DialogTitle>
        </DialogContent>
      </Dialog>
    );
    await user.keyboard("{Escape}");
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it("ConfirmDialog confirms with destructive action and cancels", async () => {
    const onConfirm = vi.fn();
    const onOpenChange = vi.fn();
    const user = userEvent.setup();
    render(
      <ConfirmDialog
        open
        onOpenChange={onOpenChange}
        title="Remove connection?"
        description="Provisioned DB will be destroyed."
        confirmLabel="Remove"
        onConfirm={onConfirm}
      />
    );
    await user.click(screen.getByRole("button", { name: "Remove" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("ConfirmDialog keeps confirm disabled until confirmPhrase matches exactly", async () => {
    const onConfirm = vi.fn();
    const user = userEvent.setup();
    render(
      <ConfirmDialog
        open
        onOpenChange={() => {}}
        title="Delete organization?"
        confirmLabel="Delete organization"
        confirmPhrase="acme"
        onConfirm={onConfirm}
      />
    );
    const confirm = screen.getByRole("button", { name: "Delete organization" });
    expect(confirm).toBeDisabled();

    const input = screen.getByRole("textbox");
    // A partial match must not arm the button.
    await user.type(input, "acm");
    expect(confirm).toBeDisabled();

    // Case must match too — a slug is a literal.
    await user.clear(input);
    await user.type(input, "ACME");
    expect(confirm).toBeDisabled();

    await user.clear(input);
    await user.type(input, "acme");
    expect(confirm).toBeEnabled();
    await user.click(confirm);
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("ConfirmDialog renders extra body content and inline errors", () => {
    render(
      <ConfirmDialog
        open
        onOpenChange={() => {}}
        title="Delete project?"
        confirmPhrase="web"
        onConfirm={() => {}}
        error={<p>Delete these projects first.</p>}
      >
        <p>2 API keys, 1 trigger</p>
      </ConfirmDialog>
    );
    expect(screen.getByText("2 API keys, 1 trigger")).toBeInTheDocument();
    expect(screen.getByText("Delete these projects first.")).toBeInTheDocument();
  });
});
