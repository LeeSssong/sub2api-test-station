import { describe, expect, it } from 'vitest'
import { resolveAdminLabAccessRedirect } from '@/router/adminLabAccess'

describe('admin lab route access', () => {
  it('keeps only the login page public', () => {
    expect(resolveAdminLabAccessRedirect(true, '/login', false, false)).toBeNull()
    expect(resolveAdminLabAccessRedirect(true, '/home', false, false)).toBe('/login')
    expect(resolveAdminLabAccessRedirect(true, '/admin/dashboard', false, false)).toBe('/login')
  })

  it('rejects normal lab users from every non-login route', () => {
    expect(resolveAdminLabAccessRedirect(true, '/dashboard', true, false)).toBe('/login')
    expect(resolveAdminLabAccessRedirect(true, '/home', true, false)).toBe('/login')
  })

  it('allows lab administrators and does not affect production builds', () => {
    expect(resolveAdminLabAccessRedirect(true, '/admin/dashboard', true, true)).toBeNull()
    expect(resolveAdminLabAccessRedirect(false, '/home', false, false)).toBeNull()
  })
})
