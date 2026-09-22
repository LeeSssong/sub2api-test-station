<template>
  <div>
    <label v-if="variant !== 'recharge'" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('payment.paymentMethod') }}
    </label>
    <div
      data-testid="payment-method-grid"
      :class="variant === 'recharge' ? 'grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3' : 'grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4'"
    >
      <button
        v-for="method in sortedMethods"
        :key="method.type"
        type="button"
        :title="methodLabel(method)"
        :disabled="!method.available"
        :class="[
          'relative min-w-0 border transition-all',
          variant === 'recharge'
            ? 'flex h-[68px] w-full max-w-[240px] items-center rounded-[10px] px-[14px] text-left'
            : 'flex h-[60px] flex-col items-center justify-center rounded-lg px-3',
          !method.available
            ? 'cursor-not-allowed border-gray-200 bg-gray-50 opacity-50 dark:border-dark-700 dark:bg-dark-800/50'
            : selected === method.type
              ? selectedClass(method.type)
              : 'border-gray-300 bg-white text-gray-700 hover:border-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-dark-500',
        ]"
        @click="method.available && emit('select', method.type)"
      >
        <template v-if="variant === 'recharge'">
          <span class="flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-lg bg-[#eaf9f9]">
            <img :src="methodIcon(method.type)" :alt="methodLabel(method)" class="h-6 w-6 object-contain" />
          </span>
          <span class="ml-3 flex min-w-0 flex-1 flex-col items-start">
            <span data-testid="payment-method-label" class="block w-full truncate text-sm font-semibold text-[#f1f9f9]">
              {{ methodLabel(method) }}
            </span>
            <span v-if="!isBuiltInAlipayMethod(method.type)" class="mt-0.5 text-[10px] text-[#8cdfc4]">
              {{ method.available ? '当前可用' : '当前不可用' }}
            </span>
          </span>
          <span v-if="selected === method.type" data-testid="payment-method-selected-mark" class="ml-2 text-sm font-bold text-[#8cdfc4]">✓</span>
        </template>
        <span v-else class="flex w-full min-w-0 items-center justify-center gap-2">
          <img :src="methodIcon(method.type)" :alt="methodLabel(method)" class="h-7 w-7 shrink-0 object-contain" />
          <span class="flex min-w-0 flex-col items-start leading-none">
            <span data-testid="payment-method-label" class="block w-full truncate text-base font-semibold">{{ methodLabel(method) }}</span>
            <span v-if="method.fee_rate > 0" class="text-[10px] tracking-wide text-gray-500 dark:text-dark-400">{{ t('payment.fee') }} {{ method.fee_rate }}%</span>
          </span>
        </span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { METHOD_ORDER, isBuiltInAlipayMethod, isBuiltInWxpayMethod } from './providerConfig'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import stripeIcon from '@/assets/icons/stripe.svg'
import airwallexIcon from '@/assets/icons/airwallex.svg'
import paymentIcon from '@/assets/icons/payment.svg'

export interface PaymentMethodOption {
  type: string
  display_name?: string
  fee_rate: number
  available: boolean
}

const props = defineProps<{
  methods: PaymentMethodOption[]
  selected: string
  variant?: 'default' | 'recharge'
}>()

const emit = defineEmits<{
  select: [type: string]
}>()

const { t } = useI18n()

const METHOD_ICONS: Record<string, string> = {
  alipay: alipayIcon,
  wxpay: wxpayIcon,
  stripe: stripeIcon,
  airwallex: airwallexIcon,
  credit_card: paymentIcon,
}

const sortedMethods = computed(() => {
  const order: readonly string[] = METHOD_ORDER
  return [...props.methods].sort((a, b) => {
    const ai = order.indexOf(a.type)
    const bi = order.indexOf(b.type)
    return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
  })
})

function methodIcon(type: string): string {
  if (isBuiltInAlipayMethod(type)) return METHOD_ICONS.alipay
  if (isBuiltInWxpayMethod(type)) return METHOD_ICONS.wxpay
  if (type === 'airwallex') return METHOD_ICONS.airwallex
  return METHOD_ICONS[type] || paymentIcon
}

function methodLabel(method: PaymentMethodOption): string {
  return method.display_name || t(`payment.methods.${method.type}`, method.type)
}

function methodSelectedClass(type: string): string {
  if (isBuiltInAlipayMethod(type)) return 'border-[#02A9F1] bg-blue-50 text-gray-900 shadow-sm dark:bg-blue-950 dark:text-gray-100'
  if (isBuiltInWxpayMethod(type)) return 'border-[#09BB07] bg-green-50 text-gray-900 shadow-sm dark:bg-green-950 dark:text-gray-100'
  if (type === 'stripe') return 'border-[#676BE5] bg-indigo-50 text-gray-900 shadow-sm dark:bg-indigo-950 dark:text-gray-100'
  if (type === 'airwallex') return 'border-[#FF6B3D] bg-orange-50 text-gray-900 shadow-sm dark:border-[#FF8E3C] dark:bg-orange-950 dark:text-gray-100'
  return 'border-primary-500 bg-primary-50 text-gray-900 shadow-sm dark:bg-primary-950 dark:text-gray-100'
}

function selectedClass(type: string): string {
  if (props.variant === 'recharge') {
    return 'border-[rgba(97,201,217,0.46)] bg-[linear-gradient(169deg,rgba(23,69,91,0.72),rgba(11,38,56,0.82))] text-[#f1f9f9]'
  }
  return methodSelectedClass(type)
}
</script>
