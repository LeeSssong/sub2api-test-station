import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const componentSource = readFileSync('src/views/admin/ops/components/OpsRequestDetailsModal.vue', 'utf8')

describe('OpsRequestDetailsModal lifecycle projection', () => {
  it('renders the logical terminal and attempt diagnostics for administrators', () => {
    expect(componentSource).toContain('data-test="terminal-kind"')
    expect(componentSource).toContain('data-test="attempt-count"')
    expect(componentSource).toContain('data-test="upstream-error-count"')
    expect(componentSource).toContain('data-test="failover-count"')
  })
})
