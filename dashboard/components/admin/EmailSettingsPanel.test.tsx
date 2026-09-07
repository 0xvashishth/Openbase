import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { EmailSettingsPanel } from "./EmailSettingsPanel";

vi.mock("@/lib/api", async () => {
  class ApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  }
  return {
    ApiError,
    api: {
      getMailSettings: vi.fn(),
      updateMailSettings: vi.fn(),
      testMailSend: vi.fn(),
      listMailLog: vi.fn(),
    },
  };
});

vi.mock("@/components/AuthProvider", () => ({ authToken: () => "test-token" }));

import { api } from "@/lib/api";
import { ToastProvider } from "@/components/ui/toast";

function renderPanel() {
  render(
    <ToastProvider>
      <EmailSettingsPanel />
    </ToastProvider>
  );
}

const smtpSettings = {
  provider: "smtp",
  smtp_host: "smtp.example.com",
  smtp_port: 587,
  smtp_username: "op",
  password_set: true,
  from_address: "noreply@example.com",
  from_name: "Openbase",
};

describe("EmailSettingsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listMailLog).mockResolvedValue([]);
  });

  it("loads stored settings and marks the password as stored", async () => {
    vi.mocked(api.getMailSettings).mockResolvedValue(smtpSettings as never);
    renderPanel();
    await waitFor(() =>
      expect(screen.getByLabelText(/Host/)).toHaveValue("smtp.example.com")
    );
    expect(screen.getByLabelText(/Password/)).toHaveValue("");
    expect(screen.getByText(/leave blank to keep/i)).toBeInTheDocument();
  });

  it("shows the owner-only message on 403", async () => {
    const { ApiError } = await import("@/lib/api");
    vi.mocked(api.getMailSettings).mockRejectedValue(new ApiError(403, "owner role required"));
    renderPanel();
    await waitFor(() =>
      expect(screen.getByText(/owner access required/i)).toBeInTheDocument()
    );
  });

  it("saves trimmed values and omits an empty password", async () => {
    vi.mocked(api.getMailSettings).mockResolvedValue({ ...smtpSettings, provider: "log" } as never);
    vi.mocked(api.updateMailSettings).mockImplementation(async (_t, body) => ({
      ...(smtpSettings as object),
      ...(body as object),
      password_set: true,
    }) as never);
    renderPanel();
    await waitFor(() => expect(screen.getByLabelText(/Host/)).toBeInTheDocument());

    await userEvent.click(screen.getByRole("button", { name: /SMTP \(your own\)/ }));
    await userEvent.clear(screen.getByLabelText(/Host/));
    await userEvent.type(screen.getByLabelText(/Host/), "  smtp.example.com  ");
    await userEvent.click(screen.getByRole("button", { name: /save email settings/i }));

    await waitFor(() => expect(api.updateMailSettings).toHaveBeenCalled());
    const body = vi.mocked(api.updateMailSettings).mock.calls[0][1] as unknown as Record<string, unknown>;
    expect(body["smtp_host"]).toBe("smtp.example.com");
    expect(body).not.toHaveProperty("smtp_password");
  });

  it("sends a test email and shows the inline result", async () => {
    vi.mocked(api.getMailSettings).mockResolvedValue(smtpSettings as never);
    vi.mocked(api.testMailSend).mockResolvedValue({ ok: true, provider: "smtp" } as never);
    renderPanel();
    await waitFor(() => expect(screen.getByLabelText(/Recipient/)).toBeInTheDocument());

    await userEvent.type(screen.getByLabelText(/Recipient/), "me@example.com");
    await userEvent.click(screen.getByRole("button", { name: /send test/i }));

    await waitFor(() => expect(api.testMailSend).toHaveBeenCalledWith("test-token", "me@example.com"));
    await waitFor(() =>
      expect(screen.getByText(/check the inbox/i)).toBeInTheDocument()
    );
  });

  it("lists delivery log entries", async () => {
    vi.mocked(api.getMailSettings).mockResolvedValue(smtpSettings as never);
    vi.mocked(api.listMailLog).mockResolvedValue([
      {
        id: "d1",
        to_address: "me@example.com",
        template: "test (via smtp)",
        subject: "Openbase — Openbase test email",
        ok: true,
        created_at: new Date().toISOString(),
      },
    ] as never);
    renderPanel();
    await waitFor(() =>
      expect(screen.getByText("me@example.com")).toBeInTheDocument()
    );
    const section = screen.getByText("me@example.com").closest("section") ?? document.body;
    expect(within(section as HTMLElement).getByText(/sent/i)).toBeInTheDocument();
  });
});
