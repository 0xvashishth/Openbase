import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api", () => ({
  api: {
    getConnection: vi.fn(),
    saveConnection: vi.fn(),
    deleteConnection: vi.fn(),
    listCollections: vi.fn(),
    getSchema: vi.fn(),
    queryRows: vi.fn(),
  },
}));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));
// TableBrowser has its own suite; stub it so these tests stay on the panel.
vi.mock("@/components/data/TableBrowser", () => ({
  TableBrowser: () => <div data-testid="table-browser" />,
}));

import { api } from "@/lib/api";
import { DBSourcePanel } from "./DBSourcePanel";
import type { Connection } from "@/lib/types";

const connected: Connection = {
  id: "c1",
  project_id: "p1",
  mode: "byodb",
  engine: "postgres",
  status: "connected",
  created_at: new Date().toISOString(),
};

function notFound() {
  return Object.assign(new Error("no connection configured for this project"), { status: 404 });
}

describe("DBSourcePanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getConnection).mockRejectedValue(notFound());
  });

  it("frames itself as the inbound direction and names the Connect tab", async () => {
    render(<DBSourcePanel projectId="p1" />);
    expect(await screen.findByText(/points openbase/i)).toBeInTheDocument();
    expect(screen.getByText(/to point an app at openbase/i)).toBeInTheDocument();
  });

  it("shows an empty state when no database is attached", async () => {
    render(<DBSourcePanel projectId="p1" />);
    expect(await screen.findByText("No database attached")).toBeInTheDocument();
    expect(screen.queryByTestId("table-browser")).not.toBeInTheDocument();
  });

  it("saves a BYODB connection string and reports the detected engine", async () => {
    vi.mocked(api.saveConnection).mockResolvedValue({ success: true, engine: "postgres" });
    const user = userEvent.setup();
    render(<DBSourcePanel projectId="p1" />);

    await user.type(
      await screen.findByLabelText("Connection string"),
      "postgres://u:p@host:5432/db"
    );
    await user.click(screen.getByRole("button", { name: "Save connection" }));

    await waitFor(() =>
      expect(api.saveConnection).toHaveBeenCalledWith(
        "tok",
        "p1",
        "postgres://u:p@host:5432/db",
        "byodb",
        undefined
      )
    );
    expect(await screen.findByText(/detected engine: postgres/i)).toBeInTheDocument();
  });

  it("treats an empty connection string as an explicit Postgres auto-provision", async () => {
    vi.mocked(api.saveConnection).mockResolvedValue({ success: true, mode: "provisioned" });
    const user = userEvent.setup();
    render(<DBSourcePanel projectId="p1" />);

    await user.click(await screen.findByRole("button", { name: /auto-provision postgres/i }));

    await waitFor(() =>
      expect(api.saveConnection).toHaveBeenCalledWith("tok", "p1", "", "provisioned", "postgres")
    );
  });

  it("provisions the chosen engine", async () => {
    vi.mocked(api.saveConnection).mockResolvedValue({ success: true });
    const user = userEvent.setup();
    render(<DBSourcePanel projectId="p1" />);

    await user.click(await screen.findByRole("button", { name: "New provisioned DB" }));
    await user.click(screen.getByRole("button", { name: /ferretdb/i }));
    await user.click(screen.getByRole("button", { name: "Provision database" }));

    await waitFor(() =>
      expect(api.saveConnection).toHaveBeenCalledWith("tok", "p1", "", "provisioned", "ferretdb")
    );
  });

  it("surfaces a failed save without clearing the input", async () => {
    vi.mocked(api.saveConnection).mockResolvedValue({ success: false, message: "connection refused" });
    const user = userEvent.setup();
    render(<DBSourcePanel projectId="p1" />);

    const input = await screen.findByLabelText("Connection string");
    await user.type(input, "postgres://bad");
    await user.click(screen.getByRole("button", { name: "Save connection" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("connection refused");
    expect(input).toHaveValue("postgres://bad");
  });

  it("shows the attached database and the data browser once connected", async () => {
    vi.mocked(api.getConnection).mockResolvedValue(connected);
    render(<DBSourcePanel projectId="p1" />);

    expect(await screen.findByText("postgres")).toBeInTheDocument();
    expect(screen.getByText("BYODB")).toBeInTheDocument();
    expect(screen.getByTestId("table-browser")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /replace database/i })).toBeInTheDocument();
  });

  it("removes the connection after confirmation", async () => {
    vi.mocked(api.getConnection).mockResolvedValue(connected);
    vi.mocked(api.deleteConnection).mockResolvedValue({ removed: true });
    const user = userEvent.setup();
    render(<DBSourcePanel projectId="p1" />);

    await user.click(await screen.findByRole("button", { name: "Remove" }));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Remove database" })
    );

    await waitFor(() => expect(api.deleteConnection).toHaveBeenCalledWith("tok", "p1"));
    expect(await screen.findByText("No database attached")).toBeInTheDocument();
  });

  it("keeps the connection when removal is cancelled", async () => {
    vi.mocked(api.getConnection).mockResolvedValue(connected);
    const user = userEvent.setup();
    render(<DBSourcePanel projectId="p1" />);

    await user.click(await screen.findByRole("button", { name: "Remove" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(api.deleteConnection).not.toHaveBeenCalled();
  });

  /**
   * BYODB and provisioned have opposite blast radii: one forgets credentials,
   * the other destroys the customer's data (no persistent volumes yet — see
   * PHASES 18.1). The dialog copy has to distinguish them.
   */
  it("warns that a provisioned database is unrecoverable", async () => {
    vi.mocked(api.getConnection).mockResolvedValue({ ...connected, mode: "provisioned" });
    const user = userEvent.setup();
    render(<DBSourcePanel projectId="p1" />);

    await user.click(await screen.findByRole("button", { name: "Remove" }));
    expect(
      within(screen.getByRole("dialog")).getByText(/not recoverable — there is no backup/i)
    ).toBeInTheDocument();
  });

  it("tells a BYODB user their database is left untouched", async () => {
    vi.mocked(api.getConnection).mockResolvedValue(connected);
    const user = userEvent.setup();
    render(<DBSourcePanel projectId="p1" />);

    await user.click(await screen.findByRole("button", { name: "Remove" }));
    expect(
      within(screen.getByRole("dialog")).getByText(/your database itself is left untouched/i)
    ).toBeInTheDocument();
  });
});
