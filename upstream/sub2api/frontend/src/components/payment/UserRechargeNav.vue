<template>
  <div>
    <section class="flex min-h-[92px] flex-col justify-between gap-5 rounded-lg border border-sky-900/80 bg-[#0a1b2f] px-5 py-4 sm:flex-row sm:items-center">
      <div>
        <h2 class="text-sm font-semibold text-white">账户权益</h2>
        <p class="mt-1 text-xs text-[#7698b3]">充值后额度将实时计入当前账户</p>
      </div>
      <div class="flex items-center">
        <div class="min-w-[150px] pr-7">
          <p class="text-xs text-[#7698b3]">可用余额</p>
          <p class="mt-1 text-2xl font-semibold tabular-nums text-white">${{ balance.toFixed(2) }}</p>
        </div>
        <div class="min-w-[150px] border-l border-sky-900/70 pl-7">
          <p class="text-xs text-[#7698b3]">并发上限</p>
          <p class="mt-1 text-2xl font-semibold tabular-nums text-white">{{ concurrency }}<span class="ml-1 text-xs font-normal text-[#7698b3]">路</span></p>
        </div>
      </div>
    </section>

    <div class="mt-3 flex items-end justify-between gap-3 border-b border-sky-900/70">
      <div class="flex" role="tablist" aria-label="充值方式">
        <router-link to="/purchase" role="tab" :aria-selected="active === 'recharge'" class="relative flex h-12 items-center px-6 text-sm font-semibold transition-colors" :class="active === 'recharge' ? activeClass : inactiveClass">
          充值
        </router-link>
        <router-link to="/redeem" role="tab" :aria-selected="active === 'redeem'" class="relative flex h-12 items-center px-6 text-sm font-semibold transition-colors" :class="active === 'redeem' ? activeClass : inactiveClass">
          兑换码
        </router-link>
      </div>
      <router-link to="/orders" class="mb-3 text-sm text-[#8ba9bf] transition-colors hover:text-[#67d4e5]">我的订单 →</router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  active: 'recharge' | 'redeem'
  balance: number
  concurrency?: number
}>(), {
  concurrency: 0,
})

const activeClass = 'text-white after:absolute after:inset-x-4 after:bottom-0 after:h-0.5 after:bg-[#67d4e5]'
const inactiveClass = 'text-[#7698b3] hover:text-white'
</script>
