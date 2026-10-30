import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import AmountInput from '../AmountInput.vue'
import { formatPaymentAmount } from '../currency'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params && 'amount' in params ? `${key} ${String(params.amount)}` : key,
  }),
}))
enableAutoUnmount(afterEach)

function mountInput(value: number | null = null) {
  return mount(AmountInput, { props: { modelValue: value } })
}

describe('recharge amount input', () => {
  it.each(['10abc', '10.555', '-10', '1e2'])('restores the accepted amount after rejecting %s', async (value) => {
    const wrapper = mountInput(10)
    const input = wrapper.get('input')
    await input.setValue(value)
    expect((input.element as HTMLInputElement).value).toBe('10')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('restores the last typed amount rather than a stale prop', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')
    await input.setValue('12.50')
    await input.setValue('12.500')
    expect((input.element as HTMLInputElement).value).toBe('12.50')
    expect(wrapper.emitted('update:modelValue')).toEqual([[12.5]])
  })

  it('preserves decimal editing and allows clearing the amount', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')
    for (const value of ['0', '0.', '0.5', '0.50', '']) await input.setValue(value)
    expect(wrapper.emitted('update:modelValue')).toEqual([[null], [null], [0.5], [0.5], [null]])
    expect((input.element as HTMLInputElement).value).toBe('')
  })
})

describe('recharge bonus hints on quick amounts', () => {
  const tiers = [
    { min_amount: 100, bonus_percent: 20 },
    { min_amount: 500, bonus_percent: 30 },
  ]

  it('renders no badge or second line when no tiers are configured', () => {
    const wrapper = mount(AmountInput, { props: { modelValue: null, amounts: [50, 100, 500] } })
    expect(wrapper.find('[data-testid="quick-amount-bonus-badge"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="quick-amount-credited"]').exists()).toBe(false)
  })

  it('bonus mode: price tag only on amounts that hit a tier, credited totals on every button', () => {
    const wrapper = mount(AmountInput, { props: { modelValue: null, amounts: [50, 100, 500], bonusTiers: tiers } })
    const below = wrapper.get('[data-testid="quick-amount-50"]')
    const first = wrapper.get('[data-testid="quick-amount-100"]')
    const second = wrapper.get('[data-testid="quick-amount-500"]')

    expect(below.find('[data-testid="quick-amount-bonus-badge"]').exists()).toBe(false)
    expect(below.get('[data-testid="quick-amount-credited"]').text()).toContain('$50.00')
    expect(first.get('[data-testid="quick-amount-bonus-badge"]').text()).toBe('+20%')
    expect(first.get('[data-testid="quick-amount-credited"]').text()).toContain('$120.00')
    expect(second.get('[data-testid="quick-amount-bonus-badge"]').text()).toBe('+30%')
    expect(second.get('[data-testid="quick-amount-credited"]').text()).toContain('$650.00')
  })

  it('bonus mode: matches tiers by the entered amount but credits by the multiplier', () => {
    const wrapper = mount(AmountInput, {
      props: { modelValue: null, amounts: [1000], bonusTiers: tiers, multiplier: 0.14 },
    })
    const button = wrapper.get('[data-testid="quick-amount-1000"]')
    expect(button.get('[data-testid="quick-amount-bonus-badge"]').text()).toBe('+30%')
    // 1000 × 0.14 = 140 base, +30% = 182
    expect(button.get('[data-testid="quick-amount-credited"]').text()).toContain('$182.00')
  })

  it('discount mode: tag reads N% OFF and the second line shows the discounted payment', () => {
    const wrapper = mount(AmountInput, {
      props: { modelValue: null, amounts: [50, 500], bonusTiers: tiers, bonusMode: 'discount', currency: 'USD' },
    })
    const below = wrapper.get('[data-testid="quick-amount-50"]')
    const hit = wrapper.get('[data-testid="quick-amount-500"]')
    expect(below.find('[data-testid="quick-amount-bonus-badge"]').exists()).toBe(false)
    expect(below.get('[data-testid="quick-amount-credited"]').text()).toContain('50.00')
    expect(hit.get('[data-testid="quick-amount-bonus-badge"]').text()).toBe('30% OFF')
    // 500 × (1 − 30%) = 350
    expect(hit.get('[data-testid="quick-amount-credited"]').text()).toContain('350.00')
  })
})


describe('quick amount badge ownership', () => {
  it.each(['default', 'recharge'] as const)('anchors multiple qualifying badges to their own %s buttons and submits the clicked amount', async (variant) => {
    const wrapper = mount(AmountInput, { props: {
      modelValue: null, variant, amounts: [10, 30, 50],
      bonusTiers: [{ min_amount: 10, bonus_percent: 5 }, { min_amount: 30, bonus_percent: 10 }],
    } })
    for (const [amount, bonus] of [[10, '+5%'], [30, '+10%'], [50, '+10%']] as const) {
      const button = wrapper.get(`[data-testid="quick-amount-${amount}"]`)
      const badge = button.get('[data-testid="quick-amount-bonus-badge"]')
      expect(button.classes()).toContain('relative')
      expect(badge.classes()).toContain('absolute')
      expect(badge.element.parentElement).toBe(button.element)
      expect(badge.text()).toBe(bonus)
      await button.trigger('click')
      expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([amount])
    }
  })
})


describe('confirmed recharge amount selection', () => {
  it('shows four credit/pay choices and keeps selection and custom editing connected', async () => {
    const wrapper = mount(AmountInput, { props: { modelValue: null, variant: 'recharge', amounts: [10, 20, 50, 100], currency: 'CNY' } })
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('')
    expect(wrapper.findAll('[aria-pressed="true"]')).toHaveLength(0)
    expect(wrapper.findAll('button')).toHaveLength(4)
    for (const amount of [10, 20, 50, 100]) {
      const choice = wrapper.get(`[data-testid="quick-amount-${amount}"]`)
      expect(choice.text()).toContain(`$${amount}`)
      expect(choice.get('[data-testid="quick-amount-pay"]').text()).toBe(formatPaymentAmount(amount, 'CNY'))
      await choice.trigger('click')
      expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([amount])
      await wrapper.setProps({ modelValue: amount })
      expect(choice.attributes('aria-pressed')).toBe('true')
      expect((wrapper.get('input').element as HTMLInputElement).value).toBe('')
    }
    const frame = wrapper.get('[data-test="custom-amount-frame"]')
    expect(frame.get('label').text()).toBe('其他额度')
    expect(frame.text()).toContain('1:1充值')
    const input = frame.get('input')
    expect(input.attributes('placeholder')).toBe('请输入额度')
    await input.setValue('12.34')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([12.34])
    await input.setValue('')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([null])
  })

  it('switches between quick and custom choices without echoing quick amounts or selecting matching custom values', async () => {
    const wrapper = mount(AmountInput, { props: { modelValue: null, variant: 'recharge', amounts: [10, 20, 50, 100] } })
    const input = wrapper.get('input')
    const choice = wrapper.get('[data-testid="quick-amount-20"]')
    await input.setValue('12.34')
    await wrapper.setProps({ modelValue: 12.34 })
    await choice.trigger('click')
    await wrapper.setProps({ modelValue: 20 })
    expect((input.element as HTMLInputElement).value).toBe('')
    expect(choice.attributes('aria-pressed')).toBe('true')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([20])

    await input.setValue('20')
    expect((input.element as HTMLInputElement).value).toBe('20')
    expect(choice.attributes('aria-pressed')).toBe('false')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([20])
    await input.setValue('')
    await wrapper.setProps({ modelValue: null })
    expect(wrapper.findAll('[aria-pressed="true"]')).toHaveLength(0)
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([null])
  })

  it.each([
    { bonusMode: 'bonus' as const, multiplier: 2, feeRate: 0, expected: 100 },
    { bonusMode: 'bonus' as const, multiplier: 1, feeRate: 1, expected: 101 },
    { bonusMode: 'discount' as const, multiplier: 1, feeRate: 1, expected: 80.8 },
  ])('shows actual payment without changing the selected input: %j', async ({ bonusMode, multiplier, feeRate, expected }) => {
    const wrapper = mount(AmountInput, { props: {
      modelValue: null, variant: 'recharge', amounts: [100], currency: 'CNY', feeRate, multiplier, bonusMode,
      bonusTiers: [{ min_amount: 100, bonus_percent: 20 }],
    } })
    const choice = wrapper.get('[data-testid="quick-amount-100"]')
    expect(choice.get('[data-testid="quick-amount-pay"]').text()).toBe(formatPaymentAmount(expected, 'CNY'))
    await choice.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[100]])
  })
})
