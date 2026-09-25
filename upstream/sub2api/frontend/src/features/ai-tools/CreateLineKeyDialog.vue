<template>
  <BaseDialog :show="show" title="创建线路密钥" panel-class="xq-key-dialog" :close-on-escape="!submitting" :show-close-button="!submitting" @close="close">
    <form id="xq-create-line-key" class="compact-key-form" @submit.prevent="submit">
      <label class="field">名称<input v-model="name" name="name" class="xq-control" placeholder="我的 API 密钥" maxlength="100" required /></label>
      <div class="field route-select">
        <span id="xq-line-label">线路</span>
        <LineSelect v-model="groupId" :groups="groups" :rates="rates" :metrics="metrics" :linked-counts="linkedCounts" :tool-id="resolvedToolId" :search-placeholder="`搜索${toolName}线路`" />
      </div>
      <section class="key-option"><label class="toggle-row">自定义密钥<input v-model="customOn" name="customOn" type="checkbox" role="switch" /></label><input v-if="customOn" v-model="customKey" name="customKey" type="password" class="xq-control" placeholder="至少 16 位字母、数字、下划线或横线" autocomplete="off" /></section>
      <section class="key-option"><label class="toggle-row">IP 限制<input v-model="ipOn" name="ipOn" type="checkbox" role="switch" /></label><div v-if="ipOn" class="expanded-fields"><label class="field">IP 白名单<textarea v-model="whitelist" name="whitelist" class="xq-control" rows="2" placeholder="每行一个 IP 或 CIDR，也可用逗号分隔" /></label><label class="field">IP 黑名单<textarea v-model="blacklist" name="blacklist" class="xq-control" rows="2" placeholder="每行一个 IP 或 CIDR，也可用逗号分隔" /></label></div></section>
      <label class="field quota-field">额度限制<div class="unit-input"><input v-model.number="quota" name="quota" type="number" min="0" step="0.01" class="xq-control" /><span>USD</span></div><small class="hint">设置此密钥可消费的最大金额。0 = 无限制。</small></label>
      <section class="key-option"><label class="toggle-row">速率限制<input v-model="rateOn" name="rateOn" type="checkbox" role="switch" /></label><div v-if="rateOn" class="expanded-fields"><label class="field">5 小时消费上限（USD）<input v-model.number="rate5h" name="rate5h" type="number" min="0" step="0.01" class="xq-control" /></label><label class="field">1 天消费上限（USD）<input v-model.number="rate1d" name="rate1d" type="number" min="0" step="0.01" class="xq-control" /></label><label class="field">7 天消费上限（USD）<input v-model.number="rate7d" name="rate7d" type="number" min="0" step="0.01" class="xq-control" /></label><small class="hint">各时间窗口的 0 均表示无限制。</small></div></section>
      <section class="key-option"><label class="toggle-row">密钥有效期<input v-model="expiresOn" name="expiresOn" type="checkbox" role="switch" /></label><input v-if="expiresOn" v-model="expires" name="expires" aria-label="到期日期" type="date" class="xq-control" :min="tomorrow" /></section>
      <p v-if="error" role="alert" class="form-error">{{ error }}</p>
    </form>
    <template #footer><button type="button" class="xq-button" data-test="cancel" :disabled="submitting" @click="close">取消</button><button type="submit" form="xq-create-line-key" class="xq-button primary" :disabled="submitting || !lineOptions.length">{{ submitting ? '创建中…' : '创建' }}</button></template>
  </BaseDialog>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LineSelect from '@/components/keys/LineSelect.vue'
import { buildLineOptions } from '@/components/keys/lineOptions'
import { keysAPI } from '@/api/keys'
import type { ApiKey, Group } from '@/types'
import type { MonitorV4Group } from '@/features/monitor-v4/types'
import { tools } from './model'
const props = defineProps<{show: boolean; toolName: string; toolId?: string; groups: Group[]; metrics: Map<number, MonitorV4Group>; linkedCounts: Map<number, number> | null; rates: Record<number, number>; initialGroupId?: number}>()
const emit = defineEmits<{close: []; created: [key: ApiKey]}>()
const name = ref(''), groupId = ref<number | null>(null), customOn = ref(false), customKey = ref(''), ipOn = ref(false), whitelist = ref(''), blacklist = ref(''), quota = ref(0), rateOn = ref(false), rate5h = ref(0), rate1d = ref(0), rate7d = ref(0), expiresOn = ref(false), expires = ref(''), error = ref(''), submitting = ref(false)
const resolvedToolId = computed(() => props.toolId || tools.find(tool => tool.label === props.toolName)?.id)
const lineOptions = computed(() => buildLineOptions(props.groups, props.rates, props.metrics, props.linkedCounts, resolvedToolId.value))
const selected = computed(() => lineOptions.value.find(option => option.value === groupId.value)?.group)
const tomorrow = computed(() => { const date = new Date(); date.setDate(date.getDate() + 1); return `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}` })
const close = () => { if (!submitting.value) emit('close') }
watch(() => props.show, show => { if (!show) return; name.value = ''; groupId.value = props.initialGroupId ?? null; customOn.value = false; customKey.value = ''; ipOn.value = false; whitelist.value = ''; blacklist.value = ''; quota.value = 0; rateOn.value = false; rate5h.value = rate1d.value = rate7d.value = 0; expiresOn.value = false; expires.value = ''; error.value = '' }, { immediate: true })
async function submit() {
  if (submitting.value) return
  error.value = ''
  if (!name.value.trim()) { error.value = '请输入名称'; return }
  if (!selected.value) { error.value = '请选择线路'; return }
  if (customOn.value && !/^[A-Za-z0-9_-]{16,}$/.test(customKey.value)) { error.value = '自定义密钥需为 至少 16 位字母、数字、下划线或横线'; return }
  if (![quota.value, ...(rateOn.value ? [rate5h.value, rate1d.value, rate7d.value] : [])].every(value => Number.isFinite(Number(value)) && Number(value) >= 0)) { error.value = '请输入有效的非负金额'; return }
  let days: number | undefined
  if (expiresOn.value) { days = Math.ceil((new Date(`${expires.value}T00:00:00`).getTime() - Date.now()) / 86400000); if (!expires.value || !Number.isFinite(days) || days <= 0) { error.value = '请选择未来的到期日期'; return } }
  const parseIPs = (text: string) => text.split(/[\n,，]+/).map(value => value.trim()).filter(Boolean)
  submitting.value = true
  try {
    const key = await keysAPI.create(name.value.trim(), selected.value.id, customOn.value ? customKey.value : undefined, ipOn.value ? parseIPs(whitelist.value) : [], ipOn.value ? parseIPs(blacklist.value) : [], Number(quota.value) || 0, days, {rate_limit_5h: rateOn.value ? Number(rate5h.value) || 0 : 0, rate_limit_1d: rateOn.value ? Number(rate1d.value) || 0 : 0, rate_limit_7d: rateOn.value ? Number(rate7d.value) || 0 : 0})
    emit('created', key)
  } catch { error.value = '创建失败，输入已保留。请检查设置后重试。' } finally { submitting.value = false }
}
</script>
<style scoped>
.compact-key-form{display:flex;flex-direction:column;gap:8px;color:var(--xq-text);font-size:12px}.field{display:flex;flex-direction:column;gap:5px;font-weight:500}.xq-control{width:100%;min-height:38px;padding:8px 12px;border:1px solid var(--xq-border);border-radius:8px;background:var(--xq-depth);color:inherit;font:inherit}.xq-control:focus-visible,.route-option:focus-visible{outline:2px solid var(--xq-accent);outline-offset:2px}.xq-control::placeholder,.hint{color:var(--xq-muted)}.hint{font-size:12px;font-weight:400;line-height:1.5}.key-option{display:flex;flex-direction:column;gap:10px}.toggle-row{display:flex;align-items:center;justify-content:space-between;min-height:34px;cursor:pointer}.toggle-row input{appearance:none;width:34px;height:20px;border-radius:12px;background:var(--xq-raised);position:relative;cursor:pointer;border:1px solid var(--xq-border)}.toggle-row input:before{content:'';display:block;width:14px;height:14px;background:var(--xq-text);border-radius:50%;position:absolute;left:2px;top:2px;transition:transform .15s}.toggle-row input:checked{background:var(--xq-accent);border-color:var(--xq-accent)}.toggle-row input:checked:before{transform:translateX(14px);background:var(--xq-depth)}.expanded-fields{display:flex;flex-direction:column;gap:10px}.unit-input{position:relative}.unit-input input{padding-right:55px}.unit-input>span{position:absolute;right:12px;top:11px;color:var(--xq-muted);font-size:12px}.select-trigger{display:flex;justify-content:space-between;align-items:center;text-align:left;cursor:pointer}.selected-line{display:flex;align-items:center;gap:8px;min-width:0}.selected-line small{color:var(--xq-secondary)}.selected-line img,.option-top img{width:20px;height:20px;object-fit:contain}.route-select{position:relative}.route-options{position:absolute;z-index:30;top:100%;left:0;right:0;margin-top:5px;border:1px solid var(--xq-border);padding:8px;border-radius:8px;background:var(--xq-depth)}.route-option-list{max-height:210px;overflow:auto;margin-top:6px}.route-option{width:100%;padding:10px 8px;border:0;border-radius:6px;background:transparent;color:inherit;text-align:left;cursor:pointer}.route-option:hover,.route-option[aria-selected=true]{background:var(--xq-raised)}.option-top{display:flex;gap:8px;align-items:center;font-size:12px}.option-top strong{font-size:14px;flex:1;min-width:0;overflow-wrap:anywhere}.option-top small{white-space:nowrap;color:var(--xq-secondary)}.option-metrics{display:block;margin:5px 0 0 28px;color:var(--xq-muted);font-size:12px}.form-error{color:var(--xq-danger);font-size:13px;margin:0}.quota-field{margin:1px 0}.xq-button:disabled{opacity:.5;cursor:wait}textarea{resize:vertical}@media(prefers-reduced-motion:reduce){.toggle-row input:before{transition:none}}
</style>
