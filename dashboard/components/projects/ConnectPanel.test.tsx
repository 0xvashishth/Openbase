import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockUseProject = vi.fn();

vi.mock("@/lib/project-context", () => ({ useProject: () => mockUseProject() }));
vi.mock("@/lib/api", () => ({
  api: { listAPIKeys: vi.fn(), createAPIKey: vi.fn() },
}));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));

import { api } from "@/lib/api";
import { ConnectPanel } from "./ConnectPanel";

const ctx = {
  orgId: "o1",
  projectId: "p1",
  engine: "postgres",
  hasConnection: true,
  supportsRealtime: true,
};

describe("ConnectPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseProject.mockReturnValue(ctx);
    vi.mocked(api.listAPIKeys).mockResolvedValue([]);
    Object.defineProperty(window, "location", {
      value: { origin: "https://dash.example.com" },
      writable: true,
    });
  });

  it("shows the API + realtime URLs derived from the dashboard origin", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText("https://dash.example.com")).toBeInTheDocument();
    expect(screen.getByText("wss://dash.example.com/v1/realtime")).toBeInTheDocument();
    expect(screen.getByText("p1")).toBeInTheDocument();
  });

  it("uses a placeholder key until one is created", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/no active keys yet/i)).toBeInTheDocument();
    expect(screen.getByRole("tabpanel")).toHaveTextContent("ob_your_api_key");
  });

  it("creates a key and shows it once, inlined into snippets", async () => {
    vi.mocked(api.createAPIKey).mockResolvedValue({ plaintext: "ob_live_key" } as never);
    const user = userEvent.setup();
    render(<ConnectPanel projectId="p1" />);

    await user.click(await screen.findByRole("button", { name: /create api key/i }));

    await waitFor(() => expect(api.createAPIKey).toHaveBeenCalledWith("tok", "p1", "connect-quickstart"));
    expect(await screen.findByText("ob_live_key")).toBeInTheDocument();
    expect(screen.getByRole("tabpanel")).toHaveTextContent("Bearer ob_live_key");
  });

  it("switches snippet languages", async () => {
    const user = userEvent.setup();
    render(<ConnectPanel projectId="p1" />);
    await screen.findByRole("tab", { name: "cURL" });
    expect(screen.getByRole("tabpanel")).toHaveTextContent("curl");

    await user.click(screen.getByRole("tab", { name: "JavaScript" }));
    expect(screen.getByRole("tabpanel")).toHaveTextContent("OPENBASE_URL");

    await user.click(screen.getByRole("tab", { name: "Realtime" }));
    expect(screen.getByRole("tabpanel")).toHaveTextContent("wss://dash.example.com/v1/realtime");
  });

  it("points at DB Source when the project has no database", async () => {
    mockUseProject.mockReturnValue({ ...ctx, hasConnection: false, engine: null });
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/no database attached yet/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /go to db source/i })).toHaveAttribute(
      "href",
      "/orgs/o1/projects/p1/db-source"
    );
  });

  it("flags engines without native realtime", async () => {
    mockUseProject.mockReturnValue({ ...ctx, supportsRealtime: false });
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/no native change stream/i)).toBeInTheDocument();
  });

  it("surfaces key-loading failures without breaking the page", async () => {
    vi.mocked(api.listAPIKeys).mockRejectedValue(new Error("boom"));
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByRole("alert")).toHaveTextContent("boom");
    expect(screen.getByText("https://dash.example.com")).toBeInTheDocument();
  });

  it("links to the API Keys tab for management", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByRole("link", { name: "API Keys" })).toHaveAttribute(
      "href",
      "/orgs/o1/projects/p1/api"
    );
  });
});
