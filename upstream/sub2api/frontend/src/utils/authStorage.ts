export const AUTH_STORAGE_PREFIX = String(import.meta.env.VITE_AUTH_STORAGE_PREFIX || '').trim()

export function authStorageKey(key: string): string {
  return AUTH_STORAGE_PREFIX ? `${AUTH_STORAGE_PREFIX}${key}` : key
}

export function authStorageGet(key: string): string | null {
  return localStorage.getItem(authStorageKey(key))
}

export function authStorageSet(key: string, value: string): void {
  localStorage.setItem(authStorageKey(key), value)
}

export function authStorageRemove(key: string): void {
  localStorage.removeItem(authStorageKey(key))
}
