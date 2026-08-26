import { describe, expect, it } from 'vitest'

import {
  createSchedulerScenarioPreviews,
  hasValidBusinessPriority,
  recommendedBusinessPriority,
  schedulerPrioritySummary,
} from '../schedulerPolicy'

describe('schedulerPolicy', () => {
  it('uses the agreed GPT-Pro recommendation when legacy priority is invalid', () => {
    expect(hasValidBusinessPriority({ profit: 0, ttft: 0, latency: 0 })).toBe(false)
    expect(recommendedBusinessPriority('GPT-Pro')).toEqual({
      profit: 3,
      ttft: 1,
      latency: 2,
    })
  })

  it('keeps equal priorities together in its operational summary', () => {
    expect(
      schedulerPrioritySummary({ profit: 1, ttft: 1, latency: 1 }, {
        profit: '利润',
        ttft: '首字速度',
        latency: '完整耗时',
      }),
    ).toBe('1：利润、首字速度、完整耗时')
  })

  it('reflects account balance selection in the normal-traffic preview', () => {
    const previews = createSchedulerScenarioPreviews({
      priority: { profit: 1, ttft: 2, latency: 3 },
      operations: {
        balance: 'high',
        peak_protection: 'strict',
        session_continuity: 'standard',
      },
    })

    expect(previews.normal).toContain('优先补齐长期未参与的健康账号')
    expect(previews.peak).toContain('暂停纯覆盖探索')
    expect(previews.session).toContain('及时切换')
  })
})
