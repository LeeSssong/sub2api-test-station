<template>
  <div>
    <section class="flex min-h-[92px] flex-col justify-between gap-5 rounded-[14px] border border-[rgba(61,124,148,0.38)] bg-[linear-gradient(156deg,rgba(16,49,71,0.82),rgba(13,40,59,0.72)_50%,rgba(8,28,43,0.62))] px-[22px] py-[18px] sm:flex-row sm:items-center">
      <div>
        <h2 class="text-[15px] font-semibold leading-6 text-[#f1f9f9]">账户权益</h2>
        <p class="text-[11px] leading-[22px] text-[#708c9e]">当前账户状态</p>
      </div>
      <div data-test="account-metrics" class="grid w-full min-w-0 grid-cols-2 sm:w-auto">
        <div data-test="account-metric" class="min-w-0 px-3 sm:min-w-[170px] sm:px-7">
          <p class="text-[11px] leading-[18px] text-[#708c9e]">可用额度</p>
          <p class="mt-0.5 text-[22px] font-semibold leading-[28px] tabular-nums text-[#f1f9f9]">${{ balance.toFixed(2) }}</p>
        </div>
        <div data-test="account-metric" class="min-w-0 border-l border-[rgba(63,119,140,0.28)] px-3 sm:min-w-[170px] sm:px-7">
          <p class="text-[11px] leading-[18px] text-[#708c9e]">并发上限</p>
          <p class="mt-0.5 text-[22px] font-semibold leading-[28px] tabular-nums text-[#f1f9f9]">{{ concurrency }}<span class="ml-1 text-[11px] font-normal text-[#708c9e]">路</span></p>
        </div>
      </div>
    </section>

    <div class="mt-[14px] flex min-w-0 items-center justify-between gap-2 border-b border-[rgba(55,107,128,0.35)] sm:gap-4">
      <div class="flex" role="tablist" aria-label="充值方式">
        <router-link v-if="paymentEnabled" data-test="recharge-tab" to="/purchase" role="tab" :aria-selected="active === 'recharge'" class="relative flex h-12 items-center px-3 text-sm font-semibold transition-colors sm:px-6" :class="active === 'recharge' ? activeClass : inactiveClass">
          充值
        </router-link>
        <span v-else data-test="recharge-tab" role="tab" aria-disabled="true" aria-selected="false" class="flex h-12 items-center px-3 text-sm text-[#708c9e] sm:px-6">充值</span>
        <router-link to="/redeem" role="tab" :aria-selected="active === 'redeem'" class="relative flex h-12 items-center px-3 text-sm font-semibold transition-colors sm:px-6" :class="active === 'redeem' ? activeClass : inactiveClass">
          兑换码
        </router-link>
      </div>
      <router-link v-if="paymentEnabled" data-test="orders-link" to="/orders" class="flex h-[34px] shrink-0 items-center whitespace-nowrap px-1 text-xs text-[#a1b8c2] transition-colors hover:text-[#61c9d9]">我的订单 →</router-link>
    </div>
    <p v-if="!paymentEnabled" data-test="recharge-unavailable" class="mt-3 text-xs text-[#a1b8c2]">充值暂不可用，仍可使用兑换码。</p>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  active: 'recharge' | 'redeem'
  balance: number
  concurrency?: number
  paymentEnabled?: boolean
}>(), {
  concurrency: 0,
  paymentEnabled: true,
})

const activeClass = 'text-[#eaf9f9] after:absolute after:inset-x-0 after:bottom-0 after:h-[3px] after:bg-[#61c9d9]'
const inactiveClass = 'text-[#a1b8c2] hover:text-white'
</script>
