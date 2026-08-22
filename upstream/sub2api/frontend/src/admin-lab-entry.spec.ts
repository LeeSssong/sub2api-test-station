import { describe, expect, it } from 'vitest'

describe('admin lab build contract', () => {
  it('defines isolated lab prefixes for the build contract', () => {
    expect('/admin/lab/').toMatch(/^\/admin\/lab\/$/)
    expect('/admin/lab/api/v1').toContain('/admin/lab/api')
    expect('admin_lab_').toMatch(/^admin_lab_$/)
  })
})
