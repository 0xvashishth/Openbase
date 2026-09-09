import { describe, expect, it } from 'vitest'
import { normalizeBaseUrl, normalizeKey, realtimeUrl } from '../src/lib/helpers.js'
import { OpenbaseError } from '../src/lib/errors.js'

describe('normalizeBaseUrl', () => {
  it('trims trailing slashes', () => {
    expect(normalizeBaseUrl('http://localhost:8080/')).toBe('http://localhost:8080')
    expect(normalizeBaseUrl('https://api.example.com///')).toBe('https://api.example.com')
  })
  it('rejects empty / non-URL / non-http', () => {
    expect(() => normalizeBaseUrl('')).toThrowError(OpenbaseError)
    expect(() => normalizeBaseUrl('not-a-url')).toThrowError(/valid URL/)
    expect(() => normalizeBaseUrl('ftp://x.com')).toThrowError(/http\(s\)/)
  })
})

describe('normalizeKey', () => {
  it('accepts ob_ keys and JWTs', () => {
    expect(normalizeKey('ob_abcdef123456')).toBe('ob_abcdef123456')
    expect(normalizeKey('a.b.c')).toBe('a.b.c')
  })
  it('rejects empty / truncated / unknown formats', () => {
    expect(() => normalizeKey('')).toThrowError(OpenbaseError)
    expect(() => normalizeKey('ob_x')).toThrowError(/truncated/)
    expect(() => normalizeKey('random')).toThrowError(/ob_/)
  })
})

describe('realtimeUrl', () => {
  it('maps http->ws and https->wss with /v1/realtime path', () => {
    expect(realtimeUrl('http://localhost:8080')).toBe('ws://localhost:8080/v1/realtime')
    expect(realtimeUrl('https://api.example.com/')).toBe('wss://api.example.com/v1/realtime')
  })
})
