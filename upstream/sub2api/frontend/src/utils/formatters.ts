/**
 * 格式化缓存 token 数量（1K/1M 缩写）
 */
export function formatCacheTokens(tokens: number): string {
  if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(1)}M`
  if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}K`
  return tokens.toLocaleString()
}

/**
 * 自适应精度格式化倍率：保留至多 4 位小数并去掉末尾多余的 0，
 * 但至少保留 2 位小数（0.035 -> "0.035"，0.3 -> "0.30"，1 -> "1.00"）
 */
export function formatMultiplier(val: number): string {
  if (val < 0.0001) return val.toPrecision(2)
  return val.toFixed(4).replace(/(\.\d{2}\d*?)0+$/, '$1')
}

/** Display-only label for a configured multiplier; never substitute missing data. */
export function formatMultiplierLabel(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '倍率暂不可用'
  const displayValue = Number(value.toPrecision(15))
  return `${Number.isInteger(displayValue) ? displayValue.toFixed(1) : String(displayValue)}x倍率`
}
