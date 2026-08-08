import { computed, type Ref, unref } from 'vue'
import type { ControlPlaneResponse, ReadModelFreshness } from '@/api/controlPlane'

export function useReadModelFreshness<T>(response: Ref<ControlPlaneResponse<T> | null> | ControlPlaneResponse<T> | null) {
  const freshness = computed<ReadModelFreshness>(() => unref(response)?.freshness ?? {})
  const degraded = computed(() => Boolean(unref(response)?.degraded) || freshness.value.completeness === 'degraded')
  const generatedAt = computed(() => freshness.value.generated_at ?? '')
  const watermark = computed(() => freshness.value.source_watermark ?? '')
  const freshnessSeconds = computed(() => freshness.value.freshness_seconds ?? -1)
  const completeness = computed(() => freshness.value.completeness ?? 'unknown')
  const calculationVersion = computed(() => freshness.value.calculation_version ?? 'unknown')
  return { freshness, generatedAt, watermark, freshnessSeconds, completeness, calculationVersion, degraded }
}

