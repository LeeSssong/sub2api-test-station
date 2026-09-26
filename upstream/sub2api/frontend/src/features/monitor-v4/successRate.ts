export function successRateTone(rate: number | null | undefined) {
  if (rate == null || !Number.isFinite(rate) || rate < 0 || rate > 100) return 'muted'
  return rate >= 85 ? 'green' : rate >= 50 ? 'amber' : 'red'
}
