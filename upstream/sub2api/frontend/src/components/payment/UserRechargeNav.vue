<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-3">
      <div class="inline-flex min-h-12 w-full max-w-xl rounded-lg border border-sky-900 bg-[#071426] p-1" role="tablist" aria-label="充值方式">
        <router-link to="/purchase" role="tab" :aria-selected="active === 'recharge'" class="flex flex-1 items-center justify-center rounded-md px-4 text-sm font-semibold transition-colors" :class="active === 'recharge' ? activeClass : inactiveClass">
          充值
        </router-link>
        <router-link to="/redeem" role="tab" :aria-selected="active === 'redeem'" class="flex flex-1 items-center justify-center rounded-md px-4 text-sm font-semibold transition-colors" :class="active === 'redeem' ? activeClass : inactiveClass">
          兑换
        </router-link>
      </div>
      <router-link to="/orders" class="btn btn-secondary shrink-0">我的订单</router-link>
    </div>

    <div class="flex min-h-24 items-center justify-between rounded-lg border border-[#173a5e] bg-[#0a162c] px-5 py-4">
      <div>
        <p class="text-xs font-medium text-[#94bbd9]">可用余额</p>
        <p class="mt-1 text-2xl font-bold tabular-nums text-[#ffca48]">${{ balance.toFixed(2) }}</p>
      </div>
      <span class="h-8 w-8 rotate-45 border border-sky-700 bg-[#071426]" aria-hidden="true"></span>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  active: 'recharge' | 'redeem'
  balance: number
}>()

const activeClass = 'bg-[#0a2440] text-sky-300 shadow-[inset_0_-2px_#ffca48]'
const inactiveClass = 'text-[#7f9bb2] hover:text-white'
</script>
