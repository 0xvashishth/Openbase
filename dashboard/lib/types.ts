// Shared types matching the Openbase REST API.

export interface User {
  id: string;
  email: string;
  full_name?: string;
  created_at: string;
  updated_at: string;
}

export interface AuthResponse {
  token: string;
  expires_at: string;
  user: User;
}

export interface SessionView {
  id: string;
  user_agent: string;
  ip: string;
  created_at: string;
  expires_at: string;
  revoked_at?: string;
}

export type OrgRole = "owner" | "admin" | "member";

export interface OrgMember {
  user_id: string;
  email: string;
  full_name?: string;
  role: OrgRole;
  joined_at: string;
}

export interface Invite {
  id: string;
  email: string;
  role: OrgRole;
  invited_by: string;
  expires_at: string;
  accepted_at?: string;
  created_at: string;
}

/**
 * Organization as returned by the API. `role` is the *caller's* role in this
 * org — the server folds it into every org-scoped payload (orgView) so the
 * dashboard can gate UI without a second round-trip.
 */
export interface Organization {
  id: string;
  name: string;
  slug: string;
  created_by: string;
  created_at: string;
  updated_at?: string;
  role?: OrgRole;
}

export interface Project {
  id: string;
  organization_id: string;
  name: string;
  slug: string;
  created_by: string;
  created_at: string;
  updated_at?: string;
}

export type ConnectionMode = "provisioned" | "byodb";
export type ConnectionStatus = "pending" | "connected" | "error";

export interface Connection {
  id: string;
  project_id: string;
  mode: ConnectionMode;
  engine: string;
  container_id?: string;
  encryption_key_id?: string;
  status: ConnectionStatus;
  last_checked_at?: string;
  created_at: string;
}

export interface TestConnectionResult {
  success: boolean;
  engine?: string;
  mode?: string;
  message?: string;
}

/** Endpoint reference returned by /connect-info (server is the source of truth). */
export interface ConnectEndpoints {
  list_tables: string;
  query_rows: string;
  table_schema: string;
  insert_row: string;
  update_row: string;
  delete_row: string;
  realtime: string;
}

/**
 * Everything an external app needs to talk TO Openbase. Never carries database
 * credentials — clients authenticate with an API key against /v1/api/*.
 */
export interface ConnectInfo {
  project_id: string;
  has_connection: boolean;
  engine?: string;
  mode?: ConnectionMode;
  status?: ConnectionStatus;
  api_base_url: string;
  realtime_url: string;
  endpoints: ConnectEndpoints;
  auth_header: string;
  capabilities?: CapabilitySet;
  supports_realtime?: string;
  active_api_keys: number;
}

// Adapter-backed schema/query types.

export interface ColumnInfo {
  name: string;
  data_type: string;
  nullable: boolean;
  is_primary: boolean;
  is_unique: boolean;
  default_val?: string | null;
}

export interface IndexInfo {
  name: string;
  kind: string;
  columns: string[];
}

export interface SchemaInfo {
  collection: string;
  columns: ColumnInfo[];
  indexes?: IndexInfo[];
}

export interface Condition {
  field: string;
  operator: string;
  value: unknown;
}

export interface OrderBy {
  field: string;
  desc: boolean;
}

export interface QueryRowsRequest {
  collection: string;
  conditions?: Condition[];
  order_by?: OrderBy[];
  limit?: number;
  offset?: number;
}

export interface ResultSet {
  columns: string[];
  rows: Record<string, unknown>[];
}

// Schema explorer types.

export interface Relationship {
  from_collection: string;
  from_column: string;
  to_collection: string;
  to_column: string;
}

export interface CapabilitySet {
  supports_relational_joins: boolean;
  supports_foreign_keys: boolean;
  supports_native_triggers: boolean;
  supports_change_streams: boolean;
  supports_realtime: string;
  supports_transactions: boolean;
  supports_full_text_search: boolean;
  supports_vector_search: boolean;
}

export interface FullSchema {
  collections: SchemaInfo[];
  relationships: Relationship[];
  capabilities: CapabilitySet;
}

export interface APIKeyView {
  id: string;
  name: string;
  scopes: string[];
  created_at: string;
  revoked_at?: string;
}

// Triggers + runtime functions (Phase 4).

export type TriggerEvent = "insert" | "update" | "delete";
export type TriggerActionType = "function" | "webhook";

export interface Trigger {
  id: string;
  project_id: string;
  name: string;
  collection: string;
  event: TriggerEvent;
  action_type: TriggerActionType;
  action_target: string;
  enabled: boolean;
  created_at: string;
}

// One webhook delivery attempt record (Phase 8.9 delivery log).
export interface WebhookDelivery {
  id: string;
  project_id: string;
  trigger_id?: string;
  target_url: string;
  collection: string;
  event: string;
  attempts: number;
  status_code?: number | null;
  ok: boolean;
  error?: string;
  duration_ms: number;
  created_at: string;
}

export type FunctionRuntime = "node" | "python";
export interface Function {
  id: string;
  project_id: string;
  name: string;
  runtime: FunctionRuntime;
  source: string;
  created_at: string;
}

// Platform mail settings (Phase 9.2, dashboard-managed BYOC SMTP).
// The password is write-only and never returned.
export type MailProvider = "smtp" | "log";

export interface MailSettingsView {
  provider: MailProvider;
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  password_set: boolean;
  from_address: string;
  from_name: string;
}

export interface UpdateMailSettings {
  provider: MailProvider;
  smtp_host?: string;
  smtp_port?: number;
  smtp_username?: string;
  /** Write-only: omit or empty to keep the stored secret. */
  smtp_password?: string;
  from_address?: string;
  from_name?: string;
}

export interface TestMailResult {
  ok: boolean;
  provider: string;
  error?: string;
}

export interface MailLogEntry {
  id: string;
  to_address: string;
  template: string;
  subject: string;
  ok: boolean;
  error?: string;
  created_at: string;
}