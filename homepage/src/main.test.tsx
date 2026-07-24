import { describe, expect, it } from 'vitest'
import { selectRuntimePage } from './main'

describe('selectRuntimePage', () => {
  it('selects support only for the external support path', () => {
    expect(selectRuntimePage('/support')).toBe('support')
    expect(selectRuntimePage('/support/')).toBe('support')
    expect(selectRuntimePage('/')).toBe('home')
  })
})
