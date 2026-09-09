// Typed errors. Convention: HTTP failures never throw — async SDK methods
// resolve `{ data, error }`. Only programmer errors (bad URL/key, use of
// service_role in a browser) throw synchronously.

export type ErrorCode =
  | 'validation'
  | 'network'
  | 'timeout'
  | 'http'
  | 'capability'
  | 'storage_unavailable'
  | 'functions_unavailable'
  | 'auth'
  | 'unknown'

export class OpenbaseError extends Error {
  code: ErrorCode
  status?: number
  details?: string
  hint?: string

  constructor(message: string, opts?: { code?: ErrorCode; status?: number; details?: string; hint?: string }) {
    super(message)
    this.name = 'OpenbaseError'
    this.code = opts?.code ?? 'unknown'
    this.status = opts?.status
    this.details = opts?.details
    this.hint = opts?.hint
  }
}

/** PostgREST-style error shape returned inside `{ data, error }` results. */
export interface PostgrestErrorShape {
  message: string
  code: string
  details: string | null
  hint: string | null
  status?: number
}

/** Capability-gated failure: the engine honestly cannot do this. Never a wrong answer. */
export class CapabilityError extends OpenbaseError {
  engine?: string
  capability?: string
  constructor(message: string, opts?: { engine?: string; capability?: string; status?: number }) {
    super(message, { code: 'capability', status: opts?.status ?? 422 })
    this.name = 'CapabilityError'
    this.engine = opts?.engine
    this.capability = opts?.capability
  }
}

export function toPostgrestError(input: {
  message?: string
  code?: string
  details?: unknown
  hint?: unknown
  status?: number
}): PostgrestErrorShape {
  return {
    message: input.message ?? 'Request failed',
    code: String(input.code ?? ''),
    details: input.details == null ? null : String(input.details),
    hint: input.hint == null ? null : String(input.hint),
    status: input.status,
  }
}

/** Parse a JSON error body of shape `{ error: string }` or PostgREST shape. */
export function parseErrorBody(body: unknown, status: number): PostgrestErrorShape {
  if (body && typeof body === 'object') {
    const b = body as Record<string, unknown>
    if (typeof b['error'] === 'string') {
      return { message: b['error'], code: String(status), details: null, hint: null, status }
    }
    if (typeof b['message'] === 'string') {
      return toPostgrestError({
        message: b['message'],
        code: typeof b['code'] === 'string' ? b['code'] : undefined,
        details: b['details'],
        hint: b['hint'],
        status,
      })
    }
  }
  return { message: `Request failed with status ${status}`, code: String(status), details: null, hint: null, status }
}
