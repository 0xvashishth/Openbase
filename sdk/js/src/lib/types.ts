// Shared public types.

export interface User {
  id: string
  email?: string | null
  phone?: string | null
  user_metadata?: Record<string, unknown>
  app_metadata?: Record<string, unknown>
  email_confirmed_at?: string | null
  phone_confirmed_at?: string | null
  banned_until?: string | null
  created_at?: string
  updated_at?: string
  [k: string]: unknown
}

export interface Session {
  access_token: string
  refresh_token: string
  token_type: string
  expires_in: number
  expires_at?: number // unix seconds (client-computed)
  user: User
}

export type AuthChangeEvent =
  | 'SIGNED_IN'
  | 'SIGNED_OUT'
  | 'TOKEN_REFRESHED'
  | 'USER_UPDATED'
  | 'PASSWORD_RECOVERY'
  | 'MFA_CHALLENGE_VERIFIED'

export interface AuthChangeCallback {
  (event: AuthChangeEvent, session: Session | null): void
}

export interface CapabilitySet {
  SupportsForeignKeys?: boolean
  SupportsNativeTriggers?: boolean
  SupportsRealtime?: boolean
  SupportsVectorSearch?: boolean
  SupportsDDL?: boolean
  SupportsRowSecurity?: boolean
  [k: string]: boolean | undefined
}
