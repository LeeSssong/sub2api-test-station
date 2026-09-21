<template>
  <div :class="variant === 'recharge' ? 'space-y-[18px]' : 'space-y-4'">
    <!-- Quick Amount Buttons -->
    <div>
      <label v-if="variant !== 'recharge'" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('payment.quickAmounts') }}
      </label>
      <div :class="variant === 'recharge' ? 'grid grid-cols-2 gap-[10px] sm:grid-cols-5' : 'grid grid-cols-2 gap-2 sm:grid-cols-5'">
        <button
          v-for="amt in filteredAmounts"
          :key="amt"
          type="button"
          :class="[
            'min-h-12 border px-3 text-center text-sm font-medium transition-colors',
            variant === 'recharge' ? 'rounded-[9px]' : 'rounded-lg py-3',
            modelValue === amt
              ? 'border-[#61c9d9] bg-[linear-gradient(167deg,rgba(31,87,110,0.82),rgba(15,48,69,0.9))] text-[#eaf9f9] shadow-[0_12px_30px_rgba(27,112,139,0.12)]'
              : 'border-[rgba(54,111,134,0.44)] bg-[#091a2b] text-[#a1b8c2] hover:border-[#3a6c87] hover:text-white',
          ]"
          @click="selectAmount(amt)"
        >
          ${{ amt }}
        </button>
      </div>
    </div>

    <!-- Custom Amount Input -->
    <div>
      <label :class="variant === 'recharge' ? 'mb-2 block text-xs text-[#a1b8c2]' : 'mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300'">
        {{ variant === 'recharge' ? '自定义额度' : t('payment.customAmount') }}
      </label>
      <div class="relative">
        <span v-if="variant !== 'recharge'" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-dark-500">
          $
        </span>
        <input
          type="text"
          inputmode="decimal"
          :value="customText"
          :placeholder="placeholderText"
          :class="variant === 'recharge' ? 'input h-10 w-full px-[14px] py-2 text-sm' : 'input w-full py-3 pl-8 pr-4'"
          @input="handleInput"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
  variant?: 'default' | 'recharge'
}>(), {
  amounts: () => [20, 50, 100],
  min: 0,
  max: 0,
  variant: 'default',
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const { t } = useI18n()

const customText = ref('')

// 0 = no limit
const filteredAmounts = computed(() =>
  props.amounts.filter((a) => (props.min <= 0 || a >= props.min) && (props.max <= 0 || a <= props.max))
)

const placeholderText = computed(() => {
  if (props.variant === 'recharge') return '输入充值额度（USD）'
  if (props.min > 0 && props.max > 0) return `${props.min} - ${props.max}`
  if (props.min > 0) return `≥ ${props.min}`
  if (props.max > 0) return `≤ ${props.max}`
  return t('payment.enterAmount')
})

const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/

function selectAmount(amt: number) {
  customText.value = String(amt)
  emit('update:modelValue', amt)
}

function handleInput(e: Event) {
  const val = (e.target as HTMLInputElement).value
  if (!AMOUNT_PATTERN.test(val)) return
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
  if (v !== null && String(v) !== customText.value) {
    customText.value = String(v)
  }
}, { immediate: true })
</script>
