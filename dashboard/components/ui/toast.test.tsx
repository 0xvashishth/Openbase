import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ToastProvider, useToast } from "./toast";

function Harness() {
  const { success, error } = useToast();
  return (
    <div>
      <button onClick={() => success("Saved", "Organization updated")}>save</button>
      <button onClick={() => error("Failed", "slug already in use")}>fail</button>
    </div>
  );
}

describe("ToastProvider", () => {
  it("renders nothing until a toast is pushed", () => {
    render(
      <ToastProvider>
        <Harness />
      </ToastProvider>
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("shows a success toast with title and description", async () => {
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <Harness />
      </ToastProvider>
    );
    await user.click(screen.getByRole("button", { name: "save" }));
    const toast = await screen.findByRole("status");
    expect(toast).toHaveTextContent("Saved");
    expect(toast).toHaveTextContent("Organization updated");
    expect(toast).toHaveAttribute("data-variant", "success");
  });

  it("uses role=alert for errors so failures are announced assertively", async () => {
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <Harness />
      </ToastProvider>
    );
    await user.click(screen.getByRole("button", { name: "fail" }));
    const toast = await screen.findByRole("alert");
    expect(toast).toHaveTextContent("Failed");
    expect(toast).toHaveAttribute("data-variant", "error");
  });

  it("dismisses a toast when the close button is clicked", async () => {
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <Harness />
      </ToastProvider>
    );
    await user.click(screen.getByRole("button", { name: "save" }));
    await screen.findByRole("status");
    await user.click(screen.getByRole("button", { name: "Dismiss notification" }));
    await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  });
});
