<template>
  <span class="iq-drawing-ribbon" :class="`is-${state}`" :title="judgment?.reason || label" role="status" :aria-label="`图片判定：${label}`">{{ label }}</span>
</template>
<script setup lang="ts">
import { computed } from 'vue';
import type { DrawingJudgment } from '@/api/intelligenceTests';
const props = defineProps<{ judgment?: DrawingJudgment }>();
const state = computed(() => props.judgment?.status || 'unjudged');
const label = computed(() => ({ passed: '通过', incorrect: '未通过', pending: '待判定', unknown: '无法判断', error: '判定异常', unjudged: '未判定' })[state.value]);
</script>
<style scoped>
.iq-drawing-ribbon { position:absolute; z-index:2; top:22px; right:-37px; width:150px; padding:7px 0; transform:rotate(45deg); text-align:center; font-size:12px; line-height:18px; font-weight:600; background:#526273; color:#fff; pointer-events:auto; }
.iq-drawing-ribbon.is-passed { background:#236d50; }
.iq-drawing-ribbon.is-incorrect { background:#a43146; }
.iq-drawing-ribbon.is-pending { background:#68571e; }
</style>
