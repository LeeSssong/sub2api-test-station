import type { MonitorV4Group } from '@/features/monitor-v4/types'
import type { ApiKey, Group } from '@/types'

export interface DashboardWorkspaceSnapshot {
  userId: string
  groups: Group[]
  keys: ApiKey[]
  rates: Record<number, number>
  metrics: MonitorV4Group[]
  metricsGeneratedAt: string | null
}

let snapshot: DashboardWorkspaceSnapshot | null = null

export function getDashboardWorkspaceSnapshot(userId: string): DashboardWorkspaceSnapshot | null {
  return snapshot?.userId === userId ? snapshot : null
}

export function setDashboardWorkspaceSnapshot(value: DashboardWorkspaceSnapshot): void {
  snapshot = value
}

export function clearDashboardWorkspaceSnapshot(): void {
  snapshot = null
}
