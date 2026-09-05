"use client";

import type {
  APIKeyView,
  AuthResponse,
  ConnectInfo,
  Connection,
  FullSchema,
  Function,
  Organization,
  Project,
  QueryRowsRequest,
  ResultSet,
  SchemaInfo,
  TestConnectionResult,
  Trigger,
  TriggerActionType,
  TriggerEvent,
  User,
} from "./types";

// Base URL of the Openbase API. Configure via NEXT_PUBLIC_OPENBASE_API_URL
// (defaults to the local dev server).
export const API_URL = "";

const TOKEN_KEY = "openbase_token";

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  window.localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  window.localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  token?: string
): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch(`${API_URL}${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  if (res.status === 401) {
    clearToken();
  }

  const text = await res.text();
  let data: any = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }

  if (!res.ok) {
    const message = data?.error ?? `Request failed with status ${res.status}`;
    throw new ApiError(res.status, message);
  }

  if (data === null) {
    data = [] as any;
  }

  return data as T;
}

export const api = {
  // Auth.
  register: (email: string, password: string, fullName?: string) =>
    request<AuthResponse>("POST", "/v1/auth/register", {
      email,
      password,
      full_name: fullName,
    }),
  login: (email: string, password: string) =>
    request<AuthResponse>("POST", "/v1/auth/login", { email, password }),
  me: (token: string) => request<User>("GET", "/v1/me", undefined, token),

  // Organizations.
  listOrgs: (token: string) =>
    request<Organization[]>("GET", "/v1/orgs", undefined, token),
  createOrg: (token: string, name: string, slug: string) =>
    request<Organization>("POST", "/v1/orgs", { name, slug }, token),
  getOrg: (token: string, orgId: string) =>
    request<Organization>("GET", `/v1/orgs/${orgId}`, undefined, token),

  // Projects.
  listProjects: (token: string, orgId: string) =>
    request<Project[]>("GET", `/v1/orgs/${orgId}/projects`, undefined, token),
  getProject: (token: string, projectId: string) =>
    request<Project>("GET", `/v1/projects/${projectId}`, undefined, token),
  createProject: (token: string, orgId: string, name: string, slug: string) =>
    request<Project>("POST", `/v1/orgs/${orgId}/projects`, { name, slug }, token),

  // Connections.
  getConnection: (token: string, projectId: string) =>
    request<Connection>("GET", `/v1/projects/${projectId}/connections`, undefined, token),
  testConnection: (token: string, projectId: string, connectionString: string) =>
    request<TestConnectionResult>("POST", `/v1/projects/${projectId}/connections/test`, {
      connection_string: connectionString,
    }, token),
  saveConnection: (
    token: string,
    projectId: string,
    connectionString: string,
    mode?: "byodb" | "provisioned",
    engine?: string
  ) =>
    request<TestConnectionResult>("POST", `/v1/projects/${projectId}/connections`, {
      connection_string: connectionString,
      mode,
      engine,
    }, token),
  deleteConnection: (token: string, projectId: string) =>
    request<{ removed: boolean }>("DELETE", `/v1/projects/${projectId}/connections`, undefined, token),

  // Outbound wiring info for the Connect tab (URLs, endpoints, capabilities).
  getConnectInfo: (token: string, projectId: string) =>
    request<ConnectInfo>("GET", `/v1/projects/${projectId}/connect-info`, undefined, token),

  // Adapter-backed data browsing.
  listCollections: (token: string, projectId: string) =>
    request<{ name: string }[]>("GET", `/v1/projects/${projectId}/collections`, undefined, token),
  getSchema: (token: string, projectId: string, collection: string) =>
    request<SchemaInfo>(`GET`, `/v1/projects/${projectId}/collections/${collection}`, undefined, token),
  queryRows: (token: string, projectId: string, body: QueryRowsRequest) =>
    request<ResultSet>("POST", `/v1/projects/${projectId}/query`, body, token),
  execSQL: (token: string, projectId: string, query: string) =>
    request<ResultSet>("POST", `/v1/projects/${projectId}/sql`, { query }, token),
  getFullSchema: (token: string, projectId: string) =>
    request<FullSchema>("GET", `/v1/projects/${projectId}/schema`, undefined, token),

  // API keys.
  listAPIKeys: (token: string, projectId: string) =>
    request<APIKeyView[]>("GET", `/v1/projects/${projectId}/api-keys`, undefined, token),
  createAPIKey: (token: string, projectId: string, name: string) =>
    request<{ id: string; name: string; plaintext: string; key_hash: string; scopes: string[]; created_at: string }>(
      "POST", `/v1/projects/${projectId}/api-keys`, { name }, token
    ),
  revokeAPIKey: (token: string, projectId: string, keyID: string) =>
    request<{ revoked: boolean }>("DELETE", `/v1/projects/${projectId}/api-keys/${keyID}`, undefined, token),

  // Triggers.
  listTriggers: (token: string, projectId: string) =>
    request<Trigger[]>("GET", `/v1/projects/${projectId}/triggers`, undefined, token),
  createTrigger: (
    token: string,
    projectId: string,
    body: {
      name: string;
      collection: string;
      event: TriggerEvent;
      action_type: TriggerActionType;
      action_target: string;
      enabled?: boolean;
    }
  ) => request<Trigger>("POST", `/v1/projects/${projectId}/triggers`, body, token),
  updateTrigger: (
    token: string,
    projectId: string,
    triggerId: string,
    body: Partial<Omit<Trigger, "id" | "project_id" | "created_at">>
  ) => request<Trigger>("PUT", `/v1/projects/${projectId}/triggers/${triggerId}`, body, token),
  deleteTrigger: (token: string, projectId: string, triggerId: string) =>
    request<{ deleted: boolean }>("DELETE", `/v1/projects/${projectId}/triggers/${triggerId}`, undefined, token),

  // Functions.
  listFunctions: (token: string, projectId: string) =>
    request<Function[]>("GET", `/v1/projects/${projectId}/functions`, undefined, token),
  createFunction: (
    token: string,
    projectId: string,
    body: { name: string; runtime: Function["runtime"]; source: string }
  ) => request<Function>("POST", `/v1/projects/${projectId}/functions`, body, token),
  getFunction: (token: string, projectId: string, fnId: string) =>
    request<Function>("GET", `/v1/projects/${projectId}/functions/${fnId}`, undefined, token),
  deleteFunction: (token: string, projectId: string, fnId: string) =>
    request<{ deleted: boolean }>("DELETE", `/v1/projects/${projectId}/functions/${fnId}`, undefined, token),
};