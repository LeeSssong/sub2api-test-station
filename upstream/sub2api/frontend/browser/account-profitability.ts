import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createI18n } from 'vue-i18n'
import AccountProfitabilityView from '../src/views/admin/AccountProfitabilityView.vue'
import { adminAPI } from '../src/api/admin'
import '../src/style.css'

const amount = (value: number) => ({ requests: 2, tokens: 10, cost: value, user_cost: -value, profit: value * 2, margin: null })
const report = {
  generated_at: '2026-08-15T10:00:00Z', range: 'today' as const, currency: 'USD' as const,
  summary: amount(123456789012345.6789),
  accounts: [{ id: 7, name: 'Native', type: 'api_key', platform: 'sub', historical: false, amounts: amount(123456789012345.6789) }],
  groups: [{ id: 10, name: 'Pro', unassigned: false, historical: false, amounts: amount(123456789012345.6789), accounts: [{ id: 7, name: 'Native', type: 'api_key', platform: 'sub', historical: false, amounts: amount(123456789012345.6789) }] }],
  user_unconsumed_balance_cny: 90,
}

adminAPI.accountFinancial.getReport = async () => report

const messages = {
  'admin.accountProfitability.title': 'Account profitability',
  'admin.accountProfitability.description': 'Native usage metrics',
  'admin.accountProfitability.ranges.today': 'Today',
  'admin.accountProfitability.ranges.24h': '24 hours',
  'admin.accountProfitability.ranges.7d': '7 days',
  'admin.accountProfitability.ranges.31d': '31 days',
  'admin.accountProfitability.loading': 'Loading',
  'admin.accountProfitability.refreshing': 'Refreshing',
  'admin.accountProfitability.empty': 'No usage',
  'admin.accountProfitability.loadError': 'Load failed',
  'admin.accountProfitability.retry': 'Retry',
  'admin.accountProfitability.scope.label': 'Scope',
  'admin.accountProfitability.scope.all': 'All',
  'admin.accountProfitability.scope.unassigned': 'Unassigned',
  'admin.accountProfitability.scope.groupSummary': 'Group summary',
  'admin.accountProfitability.scope.accountCount': '{count} accounts',
  'admin.accountProfitability.summary.requests': 'Requests',
  'admin.accountProfitability.summary.tokens': 'Tokens',
  'admin.accountProfitability.summary.accountCost': 'Account cost',
  'admin.accountProfitability.summary.userCost': 'User cost',
  'admin.accountProfitability.summary.profit': 'Profit',
  'admin.accountProfitability.summary.margin': 'Margin',
  'admin.accountProfitability.summary.unconsumedBalance': 'Balance',
  'admin.accountProfitability.columns.account': 'Account',
  'admin.accountProfitability.columns.requests': 'Requests',
  'admin.accountProfitability.columns.tokens': 'Tokens',
  'admin.accountProfitability.columns.accountCost': 'Account cost',
  'admin.accountProfitability.columns.userCost': 'User cost',
  'admin.accountProfitability.columns.profit': 'Profit',
  'admin.accountProfitability.columns.margin': 'Margin',
  'common.refresh': 'Refresh',
}

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: messages } })
window.addEventListener('error', (event) => {
  const output = document.createElement('pre'); output.id = 'browser-result'; output.textContent = JSON.stringify({ pass: false, error: event.error?.stack ?? event.message }); document.body.append(output)
})
window.addEventListener('unhandledrejection', (event) => {
  const output = document.createElement('pre'); output.id = 'browser-result'; output.textContent = JSON.stringify({ pass: false, error: String(event.reason) }); document.body.append(output)
})
const AppLayout = { setup: (_props: unknown, context: { slots: { default?: () => unknown } }) => () => h('div', context.slots.default?.()) }
const app = createApp(AccountProfitabilityView)
app.use(createPinia())
app.use(createRouter({ history: createMemoryHistory(), routes: [] }))
app.use(i18n)
app.component('AppLayout', AppLayout)
document.documentElement.style.width = '390px'
document.documentElement.style.minHeight = '844px'
document.body.style.width = '390px'
document.body.style.margin = '0'
app.mount('#app')

function rect(value: Element) {
  const r = (value as HTMLElement).getBoundingClientRect()
  return { left: r.left, right: r.right, top: r.top, bottom: r.bottom, width: r.width, height: r.height, clientWidth: (value as HTMLElement).clientWidth, scrollWidth: (value as HTMLElement).scrollWidth }
}

function inspect(selector: string) {
  const card = document.querySelector(selector) as HTMLElement | null
  if (!card) throw new Error(`missing ${selector}`)
  const value = card.lastElementChild as HTMLElement
  const cardRect = rect(card)
  const valueRect = rect(value)
  return { text: value.textContent ?? '', card: cardRect, value: valueRect, overflow: value.scrollWidth > value.clientWidth, outside: valueRect.left < cardRect.left || valueRect.right > cardRect.right }
}

function finish() {
  const summary = [inspect('[data-test="summary-cost"]'), inspect('[data-test="summary-user-cost"]')]
  document.querySelector('[data-test="scope-group-10"]')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  setTimeout(() => {
    const scoped = [inspect('[data-test="group-summary-10"] .card:nth-child(3)'), inspect('[data-test="group-summary-10"] .card:nth-child(4)')]
    const cards = [...document.querySelectorAll('[data-test="group-summary-10"] .card')].map(rect)
    const adjacentOverlap = cards.some((card, index) => cards.slice(index + 1).some(next => card.right > next.left && next.right > card.left && Math.abs(card.top - next.top) < 2))
    const all = [...summary, ...scoped]
    const expected = '$123,456,789,012,345.67'
    const valueNodes = [...document.querySelectorAll('[data-test="summary-cost"] > div:last-child, [data-test="summary-user-cost"] > div:last-child, [data-test="group-summary-10"] .card:nth-child(3) > div:last-child, [data-test="group-summary-10"] .card:nth-child(4) > div:last-child')] as HTMLElement[]
    const result = { viewport: { width: 390, height: 844, browserInnerWidth: innerWidth, browserInnerHeight: innerHeight }, all, cards, adjacentOverlap, completeText: all.every(item => item.text.includes(expected) || item.text.includes('-$123,456,789,012,345.67')), ellipsisOrTruncate: all.some(item => /…/.test(item.text)) || valueNodes.some(node => getComputedStyle(node).textOverflow === 'ellipsis' || node.className.includes('truncate')), pass: all.every(item => !item.overflow && !item.outside) && !adjacentOverlap && all.every(item => item.text.includes(expected) || item.text.includes('-$123,456,789,012,345.67')) && valueNodes.every(node => !getComputedStyle(node).textOverflow.includes('ellipsis') && !node.className.includes('truncate')) }
    const output = document.createElement('pre'); output.id = 'browser-result'; output.textContent = JSON.stringify(result); document.body.append(output); window.stop()
  }, 50)
}

setTimeout(finish, 100)
