import '@testing-library/jest-dom/vitest'
import { vi } from 'vitest'

class IntersectionObserverStub implements IntersectionObserver {
  readonly root = null
  readonly rootMargin = '0px'
  readonly scrollMargin = '0px'
  readonly thresholds = [0]

  disconnect() {}
  observe() {}
  takeRecords(): IntersectionObserverEntry[] { return [] }
  unobserve() {}
}

globalThis.IntersectionObserver = IntersectionObserverStub

const storage = new Map<string, string>()
const localStorageStub: Storage = {
  get length() { return storage.size },
  clear() { storage.clear() },
  getItem(key) { return storage.get(key) ?? null },
  key(index) { return Array.from(storage.keys())[index] ?? null },
  removeItem(key) { storage.delete(key) },
  setItem(key, value) { storage.set(key, String(value)) },
}

Object.defineProperty(globalThis, 'localStorage', {
  configurable: true,
  value: localStorageStub,
})
Object.defineProperty(window, 'localStorage', {
  configurable: true,
  value: localStorageStub,
})

Object.defineProperty(HTMLCanvasElement.prototype, 'getContext', {
  configurable: true,
  value: vi.fn(() => null),
})
