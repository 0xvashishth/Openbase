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

export interface Organization {
  id: string;
  name: string;
  slug: string;
  created_by: string;
  created_at: string;
}

export interface Project {
  id: string;
  organization_id: string;
  name: string;
  slug: string;
  created_by: string;
  created_at: string;
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

export type FunctionRuntime = "node" | "python";

export interface Function {
  id: string;
  project_id: string;
  name: string;
  runtime: FunctionRuntime;
  source: string;
  created_at: string;
}