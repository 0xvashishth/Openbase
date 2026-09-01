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