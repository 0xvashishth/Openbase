import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockUseProject = vi.fn();

vi.mock("@/lib/project-context", () => ({ useProject: () => mockUseProject() }));
vi.mock("@/lib/api", () => ({
  api: { listAPIKeys: vi.fn(), createAPIKey: vi.fn(), getConnectInfo: vi.fn() },
}));
vi.mock("@/components/AuthProvider", () => ({ authToken: () => "tok" }));

import { api } from "@/lib/api";
import { ConnectPanel } from "./ConnectPanel";
import type { ConnectInfo } from "@/lib/types";

const ctx = {
  orgId: "o1",
  projectId: "p1",
  engine: "postgres",
  hasConnection: true,
  supportsRealtime: true,
};

const connectInfo: ConnectInfo = {
  project_id: "p1",
  has_connection: true,
  engine: "postgres",
  mode: "byodb",
  status: "connected",
  api_base_url: "https://api.example.com",
  realtime_url: "wss://api.example.com/v1/realtime",
  endpoints: {
    list_tables: "GET /v1/api/tables",
    query_rows: "GET /v1/api/{collection}?limit=&offset=&order_by=&order=",
    table_schema: "GET /v1/api/{collection}/_schema",
    insert_row: "POST /v1/api/{collection}",
    update_row: "PUT /v1/api/{collection}/{id}",
    delete_row: "DELETE /v1/api/{collection}/{id}",
    realtime: "GET /v1/realtime",
  },
  auth_header: "Authorization: Bearer ob_...",
  supports_realtime: "native",
  active_api_keys: 0,
};

describe("ConnectPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseProject.mockReturnValue(ctx);
    vi.mocked(api.listAPIKeys).mockResolvedValue([]);
    vi.mocked(api.getConnectInfo).mockResolvedValue(connectInfo);
    Object.defineProperty(window, "location", {
      value: { origin: "https://dash.example.com" },
      writable: true,
    });
  });

  it("prefers the server-advertised API + realtime URLs", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText("https://api.example.com")).toBeInTheDocument();
    expect(screen.getByText("wss://api.example.com/v1/realtime")).toBeInTheDocument();
    expect(screen.getByText("p1")).toBeInTheDocument();
  });

  it("falls back to the dashboard origin when connect-info fails", async () => {
    vi.mocked(api.getConnectInfo).mockRejectedValue(new Error("nope"));
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText("https://dash.example.com")).toBeInTheDocument();
    expect(screen.getByText("wss://dash.example.com/v1/realtime")).toBeInTheDocument();
  });

  it("renders the endpoint reference from the server", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText("GET /v1/api/tables")).toBeInTheDocument();
    expect(screen.getByText("PUT /v1/api/{collection}/{id}")).toBeInTheDocument();
    expect(screen.getByText("Authorization: Bearer ob_...")).toBeInTheDocument();
  });

  it("uses a placeholder key until one is created", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/no active keys yet/i)).toBeInTheDocument();
    expect(screen.getByRole("tabpanel")).toHaveTextContent("ob_your_api_key");
  });

  it("reports the server's active key count", async () => {
    vi.mocked(api.getConnectInfo).mockResolvedValue({ ...connectInfo, active_api_keys: 2 });
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/2 active keys/i)).toBeInTheDocument();
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
    expect(screen.getByRole("tabpanel")).toHaveTextContent("wss://api.example.com/v1/realtime");
  });

  it("points at DB Source when the project has no database", async () => {
    vi.mocked(api.getConnectInfo).mockResolvedValue({
      ...connectInfo,
      has_connection: false,
      engine: undefined,
      status: undefined,
      supports_realtime: undefined,
    });
    mockUseProject.mockReturnValue({ ...ctx, hasConnection: false, engine: null });
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/no database attached yet/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /go to db source/i })).toHaveAttribute(
      "href",
      "/orgs/o1/projects/p1/db-source"
    );
  });

  it("flags engines without native realtime using server capabilities", async () => {
    vi.mocked(api.getConnectInfo).mockResolvedValue({ ...connectInfo, supports_realtime: "polling" });
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByText(/no native change stream/i)).toBeInTheDocument();
  });

  it("surfaces key-loading failures without breaking the page", async () => {
    vi.mocked(api.listAPIKeys).mockRejectedValue(new Error("boom"));
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByRole("alert")).toHaveTextContent("boom");
    expect(screen.getByText("https://api.example.com")).toBeInTheDocument();
  });

  it("links to the API Keys tab for management", async () => {
    render(<ConnectPanel projectId="p1" />);
    expect(await screen.findByRole("link", { name: "API Keys" })).toHaveAttribute(
      "href",
      "/orgs/o1/projects/p1/api"
    );
  });
});
