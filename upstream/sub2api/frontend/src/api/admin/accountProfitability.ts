import { apiClient } from '../client'

export type AccountProfitabilitySource = 'sub2api' | 'newapi' | 'self_purchased' | 'pending' | string
export type AccountProfitabilityStatus = 'known' | 'available' | 'pending' | string

export interface AccountProfitabilityParams {
  start_date: string
  end_date: string
  timezone?: string
}

export interface AccountProfitabilitySummary {
  revenue: number | string
  expense: number | string | null
  profit: number | string | null
  margin: number | string | null
  account_count: number
  pending_count: number
}

export interface AccountProfitabilityRow {
  account_id: number
  name: string
  platform: string
  account_type: string
  source: AccountProfitabilitySource
  status: AccountProfitabilityStatus
  revenue: number | string
  expense: number | string | null
  expense_currency?: 'USD' | 'CNY' | string
  procurement_expense_cny?: number | string | null
  profit: number | string | null
  margin: number | string | null
  expense_status: AccountProfitabilityStatus
  request_count: number
  tokens: number
  cost_basis?: string | null
}

export interface AccountProfitabilityResponse {
  start_date: string
  end_date: string
  generated_at: string
  summary: AccountProfitabilitySummary
  rows: AccountProfitabilityRow[]
}

export async function get(params: AccountProfitabilityParams): Promise<AccountProfitabilityResponse> {
  const { data } = await apiClient.get<AccountProfitabilityResponse>('/admin/operations/account-profitability', { params })
  return data
}

export default { get }
