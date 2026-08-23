export function resolveAdminLabAccessRedirect(
  enabled: boolean,
  path: string,
  isAuthenticated: boolean,
  isAdmin: boolean,
): '/login' | null {
  if (!enabled || path === '/login') {
    return null
  }
  return isAuthenticated && isAdmin ? null : '/login'
}
