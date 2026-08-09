export function normalizeGroupRPMLimit(value: number | ''): number {
  return value === '' ? 0 : value
}
