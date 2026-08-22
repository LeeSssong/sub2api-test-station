import { describe, expect, it } from 'vitest'
import { authStorageKey } from './authStorage'

describe('auth storage namespace', () => {
  it('keeps production keys unchanged when no prefix is configured', () => {
    expect(authStorageKey('auth_token')).toBe('auth_token')
  })
})
