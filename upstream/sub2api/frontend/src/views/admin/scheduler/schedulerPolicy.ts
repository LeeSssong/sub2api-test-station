import type {
  OpenAISchedulerBusinessPriority,
  OpenAISchedulerOperations,
} from '@/api/admin/settings'

export interface SchedulerBusinessPolicy {
  priority: OpenAISchedulerBusinessPriority
  operations: OpenAISchedulerOperations
}

export interface SchedulerMetricLabels {
  profit: string
  ttft: string
  latency: string
}

export interface SchedulerScenarioPreviews {
  normal: string
  peak: string
  session: string
}

export const DEFAULT_SCHEDULER_OPERATIONS: OpenAISchedulerOperations = {
  balance: 'standard',
  peak_protection: 'strict',
  session_continuity: 'standard',
}

export function recommendedBusinessPriority(groupName?: string): OpenAISchedulerBusinessPriority {
  const name = String(groupName ?? '').trim()
  if (name === 'GPT-特惠') return { profit: 1, ttft: 2, latency: 3 }
  if (name === 'GPT-Pro' || name === '【专属】GPT-PRO') {
    return { profit: 3, ttft: 1, latency: 2 }
  }
  return { profit: 1, ttft: 1, latency: 1 }
}

export function hasValidBusinessPriority(
  priority: Partial<OpenAISchedulerBusinessPriority> | undefined | null,
): priority is OpenAISchedulerBusinessPriority {
  return [priority?.profit, priority?.ttft, priority?.latency].every(
    (value) => Number.isInteger(value) && value! >= 1 && value! <= 3,
  )
}

export function schedulerPrioritySummary(
  priority: OpenAISchedulerBusinessPriority,
  labels: SchedulerMetricLabels,
): string {
  const levels = [1, 2, 3]
    .map((level) => ({
      level,
      labels: (Object.keys(labels) as Array<keyof SchedulerMetricLabels>)
        .filter((key) => priority[key] === level)
        .map((key) => labels[key]),
    }))
    .filter((entry) => entry.labels.length > 0)

  return levels.map((entry) => `${entry.level}：${entry.labels.join('、')}`).join('  →  ')
}

export function createSchedulerScenarioPreviews(
  policy: SchedulerBusinessPolicy,
): SchedulerScenarioPreviews {
  const balance = policy.operations.balance === 'low'
    ? '只分散高负载账号'
    : policy.operations.balance === 'high'
      ? '优先补齐长期未参与的健康账号'
      : '标准覆盖健康但长期未参与的账号'
  const peak = policy.operations.peak_protection === 'strict'
    ? '严格保护时暂停纯覆盖探索'
    : policy.operations.peak_protection === 'open'
      ? '保留覆盖时，只要有明显余量就继续补覆盖'
      : '标准保护时保留少量覆盖探索'
  const session = policy.operations.session_continuity === 'keep'
    ? '原账号健康且有余量时尽量保持'
    : policy.operations.session_continuity === 'switch'
      ? '对原账号等待容忍更低，更快切换'
      : '正常保持，遇到拥塞或风险及时切换'

  return {
    normal: `先过滤不可用和满并发账号，再按经营优先级选择；${balance}。`,
    peak: `先把流量从接近并发上限的账号分散出去；${peak}。`,
    session: `${session}；满并发、冷却或异常时始终自动切换。`,
  }
}
