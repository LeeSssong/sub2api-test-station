<template>
  <div :class="variant === 'recharge' ? 'space-y-[18px]' : 'space-y-4'">
    <!-- Quick Amount Buttons -->
    <div>
      <label v-if="variant !== 'recharge'" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('payment.quickAmounts') }}
      </label>
      <div :class="variant === 'recharge' ? 'grid grid-cols-2 gap-[10px] sm:grid-cols-4' : 'grid grid-cols-2 gap-2 sm:grid-cols-5'">
        <button
          v-for="amt in filteredAmounts"
          :key="amt"
          type="button"
          :class="[
            'relative min-h-12 border px-3 text-center text-sm font-medium transition-colors',
            variant === 'recharge' ? 'min-h-[88px] rounded-[9px] py-4' : 'rounded-lg py-3',
            isQuickAmountSelected(amt)
              ? 'border-[var(--xq-accent)] bg-[var(--xq-raised)] dark:bg-[linear-gradient(167deg,rgba(31,87,110,0.82),rgba(15,48,69,0.9))] text-[var(--xq-core)] dark:shadow-[0_12px_30px_rgba(27,112,139,0.12)]'
              : 'border-[var(--xq-border)] bg-[var(--xq-depth)] text-[var(--xq-secondary)] hover:border-[var(--xq-accent)] hover:text-[var(--xq-text)]',
          ]"
          :data-testid="`quick-amount-${amt}`"
          :aria-pressed="variant === 'recharge' ? isQuickAmountSelected(amt) : undefined"
          @click="selectAmount(amt)"
        >
          <!-- 促销价签（单行）：仅命中档位的金额显示；红底白字、内圈点线、右侧圆孔、整体旋转 -->
          <span
            v-if="quoteFor(amt).percent > 0"
            class="pointer-events-none absolute -right-2 -top-3 z-10 rotate-12"
            data-testid="quick-amount-bonus-badge"
          >
            <span
              class="relative flex items-center gap-1 whitespace-nowrap rounded bg-red-600 py-0.5 pl-1.5 pr-1 text-[11px] font-extrabold leading-tight tracking-tight text-white shadow-md ring-2 ring-white before:pointer-events-none before:absolute before:inset-[2px] before:rounded-sm before:border before:border-dotted before:border-white/70 dark:bg-red-500 dark:ring-dark-800"
            >
              <span>{{ badgeText(amt) }}</span>
              <span class="h-1 w-1 shrink-0 rounded-full bg-white"></span>
            </span>
          </span>
          <span :class="variant === 'recharge' ? 'block text-[26px] font-semibold leading-8 tabular-nums text-[var(--xq-text)]' : 'block'">{{ variant === 'recharge' ? '$' : '' }}{{ amt }}</span>
          <span v-if="variant === 'recharge'" class="mt-2 block text-[13px] font-normal leading-5 tabular-nums text-[var(--xq-secondary)]" data-testid="quick-amount-pay">{{ rechargePaymentLine(amt) }}</span>
          <span v-if="variant === 'recharge' && isQuickAmountSelected(amt)" class="absolute right-2 top-2 text-[var(--xq-core)]" aria-hidden="true"><svg width="12" height="12" viewBox="0 0 16 16" fill="none"><path d="m3 8 3 3 7-7" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg></span>
          <!-- 配置了优惠阶梯时，所有按钮都显示第二行，保持高度一致：赠金显示到账 USD，折扣显示折后实付 -->
          <span
            v-if="showSecondLine && variant !== 'recharge'"
            :class="[
              'mt-0.5 block text-[11px] font-normal leading-tight',
              quoteFor(amt).percent > 0 ? 'text-red-600 dark:text-red-300' : 'text-gray-400 dark:text-gray-500',
            ]"
            data-testid="quick-amount-credited"
          >{{ secondLine(amt) }}</span>
        </button>
      </div>
    </div>

    <!-- Custom Amount Input -->
    <div v-if="variant === 'recharge'" data-test="custom-amount-frame" class="flex min-h-11 items-center gap-2 rounded-lg border border-[var(--xq-border)] bg-[var(--xq-depth)] px-3 focus-within:border-[var(--xq-accent)] focus-within:outline focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-[var(--xq-accent)]">
      <label for="custom-recharge-amount" class="shrink-0 whitespace-nowrap text-xs text-[var(--xq-secondary)]">其他额度</label>
      <span data-test="custom-recharge-currency" class="shrink-0 text-sm text-[var(--xq-text)]" aria-hidden="true">$</span>
      <input
        id="custom-recharge-amount"
        type="text"
        inputmode="decimal"
        :value="customText"
        :placeholder="placeholderText"
        class="min-h-11 w-0 min-w-0 flex-1 border-0 bg-transparent p-0 text-sm tabular-nums text-[var(--xq-text)] caret-[var(--xq-accent)] outline-none placeholder:text-xs placeholder:text-[var(--xq-muted)] sm:placeholder:text-[13px]"
        aria-describedby="custom-recharge-ratio"
        @input="handleInput"
      />
      <span id="custom-recharge-ratio" class="shrink-0 whitespace-nowrap text-[11px] text-[var(--xq-accent)]">1:1充值</span>
    </div>
    <div v-else>
      <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('payment.customAmount') }}
      </label>
      <div class="relative">
        <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-dark-500">
          $
        </span>
        <input
          type="text"
          inputmode="decimal"
          :value="customText"
          :placeholder="placeholderText"
          class="input w-full py-3 pl-8 pr-4"
          @input="handleInput"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { RechargeBonusTier } from '@/types/payment'
import { formatRechargeBonusNumber, quoteRechargeBonus, type RechargeBonusMode } from '@/utils/rechargeBonus'
import { formatPaymentAmount } from './currency'

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
  /** 充值优惠阶梯（按 min_amount 升序）；为空时不显示价签与第二行 */
  bonusTiers?: RechargeBonusTier[]
  /** 阶梯模式：bonus 赠金 / discount 折扣 */
  bonusMode?: RechargeBonusMode
  /** 充值倍率（1 支付币种 = multiplier USD），用于计算到账金额 */
  multiplier?: number
  /** 支付币种（折扣模式第二行实付金额的币种与精度） */
  currency?: string
  feeRate?: number
  variant?: 'default' | 'recharge'
}>(), {
  amounts: () => [20, 50, 100],
  min: 0,
  max: 0,
  bonusTiers: () => [],
  bonusMode: 'bonus',
  multiplier: 1,
  currency: undefined,
  feeRate: 0,
  variant: 'default',
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const { t } = useI18n()

const customText = ref('')
const selectedQuickAmount = ref<number | null>(null)

// 0 = no limit
const filteredAmounts = computed(() =>
  props.amounts.filter((a) => (props.min <= 0 || a >= props.min) && (props.max <= 0 || a <= props.max))
)

const showSecondLine = computed(() => props.bonusTiers.length > 0)

function currencyDigits(): number {
  if (!props.currency) return 2
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency: props.currency }).resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

function quoteFor(amt: number) {
  return quoteRechargeBonus(props.bonusTiers, amt, {
    multiplier: props.multiplier,
    mode: props.bonusMode,
    currencyDigits: currencyDigits(),
  })
}

// 仅增加金额展示：报价与充值页原始手续费取整完全一致，不改变 modelValue 或提交额。
function rechargePaymentLine(amt: number): string {
  const base = quoteFor(amt).payBase
  const fee = props.feeRate > 0 && base > 0 ? Math.ceil(((base * props.feeRate) / 100) * 100) / 100 : 0
  const total = props.feeRate > 0 && base > 0 ? Math.round((base + fee) * 100) / 100 : base
  return formatPaymentAmount(total, props.currency)
}

// 价签文案：赠金「+20%」，折扣「20% OFF」
function badgeText(amt: number): string {
  const percent = formatRechargeBonusNumber(quoteFor(amt).percent)
  return props.bonusMode === 'discount' ? `${percent}% OFF` : `+${percent}%`
}

function secondLine(amt: number): string {
  const quote = quoteFor(amt)
  if (props.bonusMode === 'discount') {
    return t('payment.rechargeBonus.payShort', { amount: formatPaymentAmount(quote.payBase, props.currency) })
  }
  return t('payment.rechargeBonus.creditedShort', { amount: '$' + quote.credited.toFixed(2) })
}

const placeholderText = computed(() => {
  if (props.variant === 'recharge') return '请输入额度'
  if (props.min > 0 && props.max > 0) return `${props.min} - ${props.max}`
  if (props.min > 0) return `≥ ${props.min}`
  if (props.max > 0) return `≤ ${props.max}`
  return t('payment.enterAmount')
})

const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/

function isQuickAmountSelected(amt: number): boolean {
  return props.modelValue === amt && (props.variant !== 'recharge' || selectedQuickAmount.value === amt)
}

function selectAmount(amt: number) {
  selectedQuickAmount.value = amt
  customText.value = props.variant === 'recharge' ? '' : String(amt)
  emit('update:modelValue', amt)
}

function handleInput(e: Event) {
  const input = e.target as HTMLInputElement
  const val = input.value
  if (!AMOUNT_PATTERN.test(val)) {
    input.value = customText.value
    return
  }
  selectedQuickAmount.value = null
  customText.value = val
  if (val === '') {
    emit('update:modelValue', null)
    return
  }
  const num = parseFloat(val)
  if (!isNaN(num) && num > 0) {
    emit('update:modelValue', num)
  } else {
    emit('update:modelValue', null)
  }
}

watch(() => props.modelValue, (v) => {
  if (props.variant === 'recharge') {
    // A quick choice updates the payable amount without filling the custom field.
    if (selectedQuickAmount.value !== null && v === selectedQuickAmount.value) return
    selectedQuickAmount.value = null
  }
  if (v !== null && String(v) !== customText.value) {
    customText.value = String(v)
  }
}, { immediate: true })
</script>
