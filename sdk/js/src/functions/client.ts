// Edge/HTTP functions — `POST /v1/functions/{name}` (Phase 15).
// Until the backend lands the endpoint 404s; surface a typed error.

import { Transport, type FetchImpl } from '../lib/fetch.js'
import { OpenbaseError } from '../lib/errors.js'

export type FunctionsResponseType = 'json' | 'text' | 'blob' | 'arrayBuffer'

export interface InvokeOptions {
  body?: unknown
  headers?: Record<string, string>
  responseType?: FunctionsResponseType
}

export class FunctionsClient {
  constructor(
    private transport: Transport,
    private fetchImpl?: FetchImpl,
  ) {}

  async invoke<T = unknown>(functionName: string, options: InvokeOptions = {}) {
    if (!functionName || functionName.trim() === '') {
      return {
        data: null,
        error: { message: 'functions.invoke(): function name is required', code: 'validation', details: null, hint: null },
      }
    }
    const res = await this.transport.request<T>(`/v1/functions/${encodeURIComponent(functionName)}`, {
      method: 'POST',
      headers: options.headers,
      body: options.body,
      fetch: this.fetchImpl,
    })
    if (res.error) {
      if (res.status === 404) {
        const err = new OpenbaseError(`Function "${functionName}" not found (Phase 15 backend not deployed or no such function)`, {
          code: 'functions_unavailable',
          status: 404,
        })
        return { data: null, error: { message: err.message, code: 'functions_unavailable', details: null, hint: null } }
      }
      return { data: null, error: res.error }
    }
    void options.responseType
    return { data: res.data, error: null }
  }
}
