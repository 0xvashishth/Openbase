import { describe, expect, it } from 'vitest'
import { MemoryStorage, LocalStorageAdapter } from '../src/lib/storage.js'

describe('MemoryStorage', () => {
  it('get/set/remove round-trips', async () => {
    const s = new MemoryStorage()
    expect(await s.getItem('k')).toBeNull()
    await s.setItem('k', 'v')
    expect(await s.getItem('k')).toBe('v')
    await s.removeItem('k')
    expect(await s.getItem('k')).toBeNull()
  })
})

describe('LocalStorageAdapter', () => {
  it('no-ops outside a browser without throwing', async () => {
    const s = new LocalStorageAdapter()
    expect(await s.getItem('k')).toBeNull()
    await s.setItem('k', 'v')
    await s.removeItem('k')
  })
})
