import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../AppLayout.vue'),
  'utf8',
)

describe('AppLayout regular user shell', () => {
  it('keeps the full header for admins and a mobile menu trigger for users', () => {
    expect(source).toContain('<AppHeader v-if="isAdmin" />')
    expect(source).toContain('v-else class="sticky top-0')
    expect(source).toContain('@click="appStore.toggleMobileSidebar()"')
    expect(source).toContain('lg:hidden')
  })
})
