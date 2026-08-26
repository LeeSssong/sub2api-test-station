import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { AccountMonitorAccount, AccountMonitorReasonCode } from '@/api/admin/accountMonitor'
import AccountMonitorCard from './AccountMonitorCard.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => ({
        'admin.accountMonitor.status.success': '正常',
        'admin.accountMonitor.status.failed': '不可用',
        'admin.accountMonitor.status.pending': '待确认',
        'admin.accountMonitor.status.paused': '暂停',
        'admin.accountMonitor.status.unavailable': '不可用',
        'admin.accounts.modelDetection.editConnectionProbeModel': '修改连接测试模型',
        'admin.accounts.modelDetection.section': '模型检测',
        'admin.accounts.modelDetection.observedAbnormal': '检测器观察到异常',
        'admin.accounts.modelDetection.viewRecent': '点击查看最近检测结果',
        'admin.accounts.modelDetection.historicalFallback': '当前证据不足，沿用最近一次最终检测结果',
        'admin.accounts.modelDetection.historicalRecordHint': '历史记录：旧格式未保存档位与样本明细',
        'admin.accounts.monitor.historyFallback': '评分沿用最近一次有效结果',
        'admin.accounts.monitor.historyFallbackAt': '历史最终结果时间',
        'admin.accounts.modelDetection.title': '账号模型检测',
        'admin.accounts.modelDetection.close': '关闭',
        'admin.accounts.modelDetection.connectionProbeModel': '连接测试模型',
        'admin.accounts.modelDetection.detectionModel': '检测模型',
        'admin.accounts.modelDetection.detectorUnsupported': '检测器暂不支持',
        'admin.accounts.modelDetection.detectorUnconfigured': '检测服务未接入',
        'admin.accounts.modelDetection.detectorUnavailable': '检测服务暂不可用',
        'admin.accounts.modelDetection.recentStatus': '最近状态',
        'admin.accounts.modelDetection.declaredModel': '申报模型',
        'admin.accounts.modelDetection.requestedModel': '请求模型',
        'admin.accounts.modelDetection.upstreamResponseModel': '上游响应模型',
        'admin.accounts.modelDetection.upstreamModelMissing': '上游未返回 model 字段',
        'admin.accounts.modelDetection.activeResponseUnavailable': '未取得主动响应',
        'admin.accounts.modelDetection.catalogEvidence': '模型目录',
        'admin.accounts.modelDetection.catalogMatch': '已命中请求模型',
        'admin.accounts.modelDetection.catalogMissing': '未命中请求模型',
        'admin.accounts.modelDetection.catalogUnavailable': '未取得模型目录',
        'admin.accounts.modelDetection.catalogReturned': '目录共返回 {count} 个模型：{models}',
        'admin.accounts.modelDetection.fingerprintEvidence': '行为指纹',
        'admin.accounts.modelDetection.fingerprintMatch': '指纹匹配',
        'admin.accounts.modelDetection.fingerprintMismatch': '指纹候选：{candidate}（{similarity}）',
        'admin.accounts.modelDetection.fingerprintUnavailable': '未检测行为指纹',
        'admin.accounts.modelDetection.technicalDetails': '技术详情',
        'admin.accounts.modelDetection.verdict.verified': '模型可信',
        'admin.accounts.modelDetection.verdict.suspected_mapping': '疑似模型映射',
        'admin.accounts.modelDetection.verdict.suspected_replacement': '疑似替换模型',
        'admin.accounts.modelDetection.verdict.high_risk_inconsistent': '高风险不一致',
        'admin.accounts.modelDetection.verdict.insufficient': '证据不足',
        'admin.accounts.modelDetection.juice': 'Juice',
        'admin.accounts.modelDetection.juiceSummary': 'Juice 摘要',
        'admin.accounts.modelDetection.fingerprintCandidate': '行为指纹候选',
        'admin.accounts.modelDetection.fingerprintSimilarity': '相似度',
        'admin.accounts.modelDetection.detectorVersion': '检测器版本',
        'admin.accounts.modelDetection.detectionTime': '检测时间',
        'admin.accounts.modelDetection.error': '错误',
        'admin.accounts.modelDetection.abnormalDisclaimer': '检测器观察到异常；该结果不代表上游确认替换。',
        'admin.accounts.modelDetection.saveModels': '保存模型',
        'admin.accounts.modelDetection.detecting': '已排队…',
        'admin.accounts.modelDetection.detectNow': '立即检测',
        'admin.accounts.modelDetection.status.abnormal': '异常',
        'admin.accounts.modelDetection.status.service_unconfigured': '检测服务未接入',
        'admin.accounts.modelDetection.status.service_unavailable': '检测服务暂不可用',
      }[key] ?? key).replace('{count}', String(params?.count ?? '')).replace('{models}', String(params?.models ?? '')).replace('{candidate}', String(params?.candidate ?? '')).replace('{similarity}', String(params?.similarity ?? '')),
    }),
  }
})

type Deferred = {
  promise: Promise<void>
  resolve: () => void
  reject: (reason?: unknown) => void
}

function deferred(): Deferred {
  let resolve!: () => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<void>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
  await nextTick()
}

const account = {
  account_id: 113,
  name: '93_dowel.paddler@icloud.com',
  platform: 'openai',
  account_type: 'oauth',
  status: 'active',
  schedulable: true,
  management_state: 'enabled',
  service_state: 'available',
  group_eligibility: 'eligible',
  monitor_bucket: 'available',
  priority: 1,
  group_ids: [3],
  group_names: ['GPT-PLUS-内测'],
  model_id: 'gpt-4o-mini',
  latest_status: 'success',
  sample_count: 72,
  success_sample_count: 71,
  ttft_sample_count: 71,
  latency_sample_count: 71,
  success_rate: 0.986,
  ttft_p50_ms: 1018,
  ttft_p95_ms: 1400,
  latency_p95_ms: 1962,
  multiplier: { value: 0.58, source: 'declared', status: 'ok', sample_count: 72 },
  request_count: 72,
  error_count: 1,
  range: '24h',
  base_cost: 18,
  effective_multiplier: 0.48,
  cost_mode: 'multiplier',
  cost_score: 15,
  timeline: Array.from({ length: 24 }, (_, index) => ({
    status: index === 7 ? 'failed' : 'success',
    checked_at: `2026-08-04T04:${String(index).padStart(2, '0')}:00Z`,
    latency_ms: 900 + index * 10,
  })),
  stale: false,
  quality_score: 91,
  score_breakdown: { cost: 12, success: 43.5, ttft: 18, latency: 17.5 },
  evidence_source: 'monitor_probe',
  homepage_url: 'https://upstream.example.com/v1',
  quality_rank: 1,
  quality_rank_total: 3,
  scheduler_rank: 1,
  scheduler_rank_total: 3,
  group_rank: 1,
  eligible: true,
}

function mountCard(overrides: Record<string, unknown> = {}) {
  return mount(AccountMonitorCard, {
    attachTo: document.body,
    props: {
      account,
      concurrency: { account_id: 113, current: 3, limit: 10, delayed: false },
      ...overrides,
    },
    global: { stubs: { Icon: true } },
  })
}

afterEach(() => {
  document.body.replaceChildren()
})

	describe('AccountMonitorCard', () => {
	it('mirrors the explainable ranking response contract', () => {
		const reasonCode: AccountMonitorReasonCode = 'strategy'
		const ranking: Pick<AccountMonitorAccount, 'quality_rank' | 'quality_rank_total' | 'scheduler_rank' | 'scheduler_rank_total' | 'quality_explanation' | 'scheduler_explanation'> = {
			quality_rank: 2,
			quality_rank_total: 5,
			scheduler_rank: 1,
			scheduler_rank_total: 4,
			quality_explanation: { window: '24h', sample_count: 12, source: 'hybrid', observed_at: '2026-08-26T00:00:00Z' },
			scheduler_explanation: { eligible: true, policy_label: '利润优先', primary_reason_code: reasonCode },
		}

		expect(ranking.quality_rank).toBe(2)
		expect(ranking.scheduler_explanation?.primary_reason_code).toBe('strategy')
	})

	it('renders both rankings and expands readable server-provided explanations', async () => {
		const wrapper = mountCard({
			account: {
				...account,
				quality_rank: 3,
				quality_rank_total: 12,
				scheduler_rank: 1,
				scheduler_rank_total: 9,
				quality_explanation: {
					score: 91,
					rank: 3,
					rank_total: 12,
					breakdown: {
						cost: { score: 12, max: 25 },
						success: { score: 43.5, max: 45 },
						ttft: { score: 18, max: 20 },
						latency: { score: 17.5, max: 20 },
					},
					window: '24h',
					sample_count: 72,
					source: 'monitor_probe',
					observed_at: '2026-08-26T00:00:00Z',
				},
				scheduler_explanation: {
					eligible: true,
					policy_label: '利润优先',
					effective_weights: { quality: 60, cost: 40 },
					candidate_total: 9,
					candidate_scope: '当前分组',
					snapshot_at: '2026-08-26T00:01:00Z',
					primary_reason_code: 'strategy',
					primary_reason_label: '当前分组策略与质量排序目标不同',
				},
			},
		})

		expect(wrapper.get('[data-test="quality-rank"]').text()).toContain('第 3 / 12')
		expect(wrapper.get('[data-test="scheduler-rank"]').text()).toContain('第 1 / 9')
		expect(wrapper.get('[data-test="ranking-reason"]').text()).toContain('当前分组策略与质量排序目标不同')
		const toggle = wrapper.get('[data-test="ranking-explanation-toggle"]')
		expect(toggle.attributes('aria-expanded')).toBe('false')
		const panelID = toggle.attributes('aria-controls')
		expect(panelID).toBeTruthy()
		expect(wrapper.find(`[data-test="ranking-explanation"]`).exists()).toBe(false)

		await toggle.trigger('click')

		expect(toggle.attributes('aria-expanded')).toBe('true')
		const explanation = wrapper.get('[data-test="ranking-explanation"]')
		expect(explanation.attributes('id')).toBe(panelID)
		expect(explanation.text()).toContain('利润优先')
		expect(explanation.text()).toContain('成本 12 / 25')
		expect(explanation.text()).toContain('候选数 9')
		expect(explanation.text()).toContain('符合调度条件')
		expect(explanation.text()).toContain('2026')
	})

	it('keeps the row safely stacked before the wide desktop breakpoint', () => {
		const wrapper = mountCard()
		const row = wrapper.get('[data-test="monitor-card-header"]')

		expect(row.classes()).toEqual(expect.arrayContaining(['grid', 'min-w-0', 'gap-x-4']))
		expect(row.classes()).toContain('xl:grid-cols-[minmax(15rem,1.45fr)_minmax(10rem,.9fr)_minmax(11rem,1fr)_minmax(15rem,1.35fr)_minmax(13rem,1.1fr)_auto]')
		expect(row.classes()).not.toContain('lg:grid-cols-[minmax(15rem,1.45fr)_minmax(10rem,.9fr)_minmax(11rem,1fr)_minmax(15rem,1.35fr)_minmax(13rem,1.1fr)_auto]')
		expect(wrapper.find('[data-test="ranking-explanation"]').exists()).toBe(false)
		expect(wrapper.get('[data-test="account-actions"]').classes()).not.toContain('lg:flex-col')
		expect(wrapper.get('[data-test="account-actions"]').classes()).toContain('xl:flex-col')
	})

	it('uses legacy group_rank for selected-group quality display without changing scheduler rank', () => {
		const wrapper = mountCard({
			rankingScope: 'group',
			account: { ...account, quality_rank: undefined, quality_rank_total: undefined, group_rank: 7, scheduler_rank: 2, scheduler_rank_total: 9 },
		})

		expect(wrapper.get('[data-test="quality-rank"]').text()).toContain('第 7')
		expect(wrapper.get('[data-test="scheduler-rank"]').text()).toContain('第 2')
	})

	const recommendation = {
		status: 'recommended',
		target: 'gpt_pro',
		target_name: 'GPT-Pro',
		action: 'migrate',
		reason_codes: ['codex_auth_default_pro'],
		sample_count: 72,
		observed_at: '2026-08-10T00:00:00Z',
		source: 'monitor_probe',
	}
	const notRecommendedRecommendation = {
		status: 'not_recommended',
		target: '',
		target_name: '',
		action: 'none',
		reason_codes: ['success_rate_below_special'],
		sample_count: 72,
		observed_at: '2026-08-10T00:00:00Z',
		source: 'monitor_probe',
	}

	it('exposes native account entry points without changing the compact card projection', async () => {
		const accountInfo = vi.fn()
		const accountEdit = vi.fn()
		const accountDelete = vi.fn()
		const accountMore = vi.fn()
		const wrapper = mountCard({ onAccountInfo: accountInfo, onAccountEdit: accountEdit, onAccountDelete: accountDelete, onAccountMore: accountMore })

		expect(wrapper.get('[data-test="account-info"]').text()).toContain('账号信息')
		expect(wrapper.get('[data-test="account-edit"]').text()).toContain('编辑')
		expect(wrapper.get('[data-test="account-delete"]').text()).toContain('删除')
		expect(wrapper.get('[data-test="account-more"]').text()).toContain('更多')

		await wrapper.get('[data-test="account-info"]').trigger('click')
		await wrapper.get('[data-test="account-edit"]').trigger('click')
		await wrapper.get('[data-test="account-delete"]').trigger('click')
		await wrapper.get('[data-test="account-more"]').trigger('click')

		expect(accountInfo).toHaveBeenCalledWith(account)
		expect(accountEdit).toHaveBeenCalledWith(account)
		expect(accountDelete).toHaveBeenCalledWith(account)
		expect(accountMore).toHaveBeenCalledWith(account, expect.any(MouseEvent))
	})

	it('keeps platform, current group, schedulable state, and recommendation in one compact metadata row', () => {
		const wrapper = mountCard({ account: { ...account, group_names: ['GPT-测试分组'], group_recommendation: recommendation } })
		const metadata = wrapper.get('[data-test="account-metadata"]')
		expect(metadata.text()).toContain('平台 openai')
		expect(metadata.text()).toContain('当前分组 GPT-测试分组')
		expect(metadata.text()).toContain('调度状态 可调度')
		expect(metadata.text()).toContain('推荐：GPT-Pro')
	})

	it.each([
		['observe', 'keep', '继续观察'],
		['blocked', 'hold', '暂缓迁入'],
		['not_recommended', 'none', '暂不建议入组'],
	])('shows the test-group %s state as %s', (status, action, label) => {
		const wrapper = mountCard({
			account: {
				...account,
				group_names: ['GPT-测试分组'],
				group_recommendation: { ...recommendation, status, action, target: status === 'blocked' ? 'gpt_pro' : '', target_name: status === 'blocked' ? 'GPT-Pro' : '' },
			},
		})
		expect(wrapper.get('[data-test="group-recommendation"]').text()).toContain(label)
		expect(wrapper.find('[data-test="recommendation-warning"]').exists()).toBe(false)
	})

	it('keeps the not-recommended label compact and hides its reason by default', () => {
		const wrapper = mountCard({
			account: { ...account, group_names: ['GPT-测试分组'], group_recommendation: notRecommendedRecommendation },
		})
		const metadata = wrapper.get('[data-test="account-metadata"]')

		expect(wrapper.get('[data-test="group-recommendation"]').text()).toContain('暂不建议入组')
		expect(wrapper.find('[data-test="recommendation-reason-button"]').exists()).toBe(false)
		expect(metadata.text()).not.toContain('原因：')
	})

	it('does not expose recommendation reasons or probe evidence', () => {
		const knownWrapper = mountCard({
			account: { ...account, group_names: ['GPT-测试分组'], group_recommendation: notRecommendedRecommendation },
		})
		expect(knownWrapper.text()).not.toContain('探测成功率未达到特惠门槛')

		const emptyWrapper = mountCard({
			account: {
				...account,
				group_names: ['GPT-测试分组'],
				group_recommendation: { ...notRecommendedRecommendation, reason_codes: [] },
			},
		})
		expect(emptyWrapper.text()).not.toContain('主动探测质量不满足目标')

		const unknownWrapper = mountCard({
			account: {
				...account,
				group_names: ['GPT-测试分组'],
				group_recommendation: { ...notRecommendedRecommendation, reason_codes: ['unknown_reason'] },
			},
		})
		expect(unknownWrapper.text()).not.toContain('主动探测质量不满足目标')
	})

	it('does not open a not-recommended reason tooltip', async () => {
		const wrapper = mountCard({
			account: { ...account, group_names: ['GPT-测试分组'], group_recommendation: notRecommendedRecommendation },
		})

		expect(wrapper.find('[data-test="recommendation-reason-trigger"]').exists()).toBe(false)
	})

	it('does not expose a not-recommended reason action', async () => {
		const wrapper = mountCard({
			account: { ...account, group_names: ['GPT-测试分组'], group_recommendation: notRecommendedRecommendation },
		})
		expect(wrapper.find('[data-test="recommendation-reason-button"]').exists()).toBe(false)
	})

	it('keeps recommendation labels accessible without reason copy', async () => {
		const wrapper = mountCard({
			account: { ...account, group_names: ['GPT-测试分组'], group_recommendation: notRecommendedRecommendation },
		})
		expect(wrapper.get('[data-test="group-recommendation"]').text()).toContain('暂不建议入组')
		expect(wrapper.text()).not.toContain('探测成功率未达到特惠门槛')
	})

	it('keeps the other recommendation states unchanged', () => {
		const recommendedWrapper = mountCard({ account: { ...account, group_names: ['GPT-测试分组'], group_recommendation: recommendation } })
		const observeWrapper = mountCard({ account: { ...account, group_names: ['GPT-测试分组'], group_recommendation: { ...recommendation, status: 'observe', action: 'keep' } } })
		const blockedWrapper = mountCard({ account: { ...account, group_names: ['GPT-测试分组'], group_recommendation: { ...recommendation, status: 'blocked', action: 'hold' } } })
		const migrationWrapper = mountCard({ account: { ...account, group_names: ['GPT-Plus'], group_recommendation: recommendation } })
		const notRecommendedWrapper = mountCard({ account: { ...account, group_names: ['GPT-测试分组'], group_recommendation: notRecommendedRecommendation } })

		expect(recommendedWrapper.get('[data-test="group-recommendation"]').text()).toContain('推荐：GPT-Pro')
		expect(observeWrapper.get('[data-test="group-recommendation"]').text()).toContain('继续观察')
		expect(blockedWrapper.get('[data-test="group-recommendation"]').text()).toContain('暂缓迁入')
		expect(migrationWrapper.get('[data-test="recommendation-tooltip-trigger"]').exists()).toBe(true)
		expect(notRecommendedWrapper.find('[data-test="recommendation-reason-trigger"]').exists()).toBe(false)
		expect(recommendedWrapper.find('[data-test="recommendation-reason-trigger"]').exists()).toBe(false)
		expect(observeWrapper.find('[data-test="recommendation-reason-trigger"]').exists()).toBe(false)
		expect(blockedWrapper.find('[data-test="recommendation-reason-trigger"]').exists()).toBe(false)
		expect(migrationWrapper.find('[data-test="recommendation-reason-trigger"]').exists()).toBe(false)
	})

	it('shows an accessible warning tooltip only for an explicit formal-group migration', async () => {
		const wrapper = mountCard({ account: { ...account, group_names: ['GPT-Plus'], group_recommendation: recommendation } })
		expect(wrapper.get('[data-test="group-recommendation"]').text()).toContain('推荐：GPT-Pro')
		const warning = wrapper.get<HTMLElement>('[data-test="recommendation-warning"]')
		expect(warning.attributes('title')).toContain('推荐迁移至 GPT-Pro')
		expect(warning.attributes('aria-label')).toContain('推荐迁移至 GPT-Pro')

		warning.element.focus()
		await nextTick()
		const content = document.body.querySelector('[data-test="group-recommendation-tooltip"]')
		expect(content?.textContent).toContain('推荐迁移至 GPT-Pro')
    expect(content?.textContent).not.toContain('主动探测')
    expect(content?.textContent).not.toContain('72 次')
		expect(content?.closest('[role="tooltip"]')?.getAttribute('style')).not.toContain('display: none')
	})

	it('normalizes test-group case and ASCII spaces before deciding whether to show a formal warning', () => {
		const wrapper = mountCard({ account: { ...account, group_names: ['  gPt - 测试 分组  '], group_recommendation: recommendation } })
		expect(wrapper.get('[data-test="group-recommendation"]').text()).toContain('推荐：GPT-Pro')
		expect(wrapper.find('[data-test="recommendation-warning"]').exists()).toBe(false)
	})

	it('keeps formal matching or legacy rows free of recommendation text and warning icons', () => {
		const wrapper = mountCard({ account: { ...account, group_names: ['GPT-Pro'] } })
		expect(wrapper.find('[data-test="group-recommendation"]').exists()).toBe(false)
		expect(wrapper.find('[data-test="recommendation-warning"]').exists()).toBe(false)
		expect(wrapper.get('[data-test="account-metadata"]').text()).not.toContain('继续观察')
	})

  it('shows only the projected equivalent site multiplier in the cost evidence area', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        equivalent_site_multiplier: 0.05,
        cost_guard: {
          upstream_multiplier: 0.2,
          equivalent_site_multiplier: 0.3,
          group_multiplier: 0.1,
          model: 'legacy-model',
          sample_count: 6,
          required_sample_count: 6,
          status: 'loss_confirmed',
        },
      },
    })

    const evidence = wrapper.get('[data-test="equivalent-cost-multiplier"]')
    expect(evidence.text()).toContain('成本折合本站倍率')
    expect(evidence.text()).toContain('0.05×')
    expect(evidence.text()).not.toContain('上游原生倍率')
    expect(evidence.text()).not.toContain('当前分组倍率')
    expect(evidence.text()).not.toContain('有效样本')
    expect(evidence.text()).not.toContain('成本状态')
    expect(wrapper.find('[data-test="cost-inversion-alert"]').exists()).toBe(false)
  })

  it('shows unavailable when the equivalent site multiplier cannot be priced', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        equivalent_site_multiplier: null,
      },
    })

    expect(wrapper.get('[data-test="equivalent-cost-multiplier"]').text()).toContain('--')
  })

  it('emits editCost and removes inline cost editors', async () => {
    const editCost = vi.fn()
    const wrapper = mountCard({ onEditCost: editCost })

    await wrapper.get('[data-test="edit-cost"]').trigger('click')
    expect(editCost).toHaveBeenCalledWith(account)
    expect(wrapper.find('[data-test="cost-input"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="multiplier-input"]').exists()).toBe(false)
  })

  it('shows a valid API Key balance and marks a failed refresh as delayed', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'apikey',
        balance: { value_usd: 12.5, status: 'failed', source: 'newapi' },
      },
    })

    expect(wrapper.get('[data-test="balance-metric"]').text()).toContain('$12.50')
    expect(wrapper.get('[data-test="balance-metric"]').text()).toContain('数据延迟')
  })

  it('does not render a balance placeholder for non-API-Key accounts', () => {
    const wrapper = mountCard({ account: { ...account, account_type: 'oauth', balance: undefined } })
    expect(wrapper.find('[data-test="balance-metric"]').exists()).toBe(false)
  })

  it('renders the V3 service, rank, cost, and concurrency information without operations labels', () => {
    const wrapper = mountCard()

    expect(wrapper.get('[data-test="monitor-card"]').text()).toContain('93_dowel.paddler@icloud.com')
    expect(wrapper.text()).toContain('#113')
    expect(wrapper.get('[data-test="status-badge"]').text()).toContain('正常')
    expect(wrapper.get('[data-test="score-metric"]').text()).toContain('91')
    expect(wrapper.get('[data-test="rank-metric"]').text()).toContain('第 1')
    expect(wrapper.get('[data-test="priority-control"]').text()).toContain('1')
    expect(wrapper.get('[data-test="success-rate-metric"]').text()).toContain('98.6%')
    expect(wrapper.get('[data-test="success-rate-metric"]').text()).not.toContain('探测样本')
    expect(wrapper.get('[data-test="success-rate-metric"]').text()).not.toContain('失败')
    expect(wrapper.get('[data-test="ttft-metric"]').text()).toContain('1018 ms')
    expect(wrapper.get('[data-test="latency-metric"]').text()).toContain('1962 ms')
    expect(wrapper.get('[data-test="cost-metric"]').text()).toContain('成本待确认')
    expect(wrapper.get('[data-test="concurrency-metric"]').text()).toContain('3 / 10')

    for (const label of ['分组倍率', '营收', '利润', '对账', '账务', '上游真实扣费', '用户实际计费']) {
      expect(wrapper.text()).not.toContain(label)
    }
  })

  it('keeps a retained score and rank visible while current status is unavailable or stale', () => {
    const unavailable = mountCard({
      account: { ...account, availability_status: 'unavailable', service_state: 'unavailable', score_status: 'eligible', quality_score: 82, quality_rank: 3, scheduler_rank: 3, group_rank: 3, eligible: true },
    })
    expect(unavailable.get('[data-test="status-badge"]').text()).toContain('不可用')
    expect(unavailable.get('[data-test="score-metric"]').text()).toContain('82')
    expect(unavailable.get('[data-test="rank-metric"]').text()).toContain('第 3')

    const stale = mountCard({
      account: { ...account, availability_status: 'stale', service_state: 'pending', stale: true, score_status: 'eligible', quality_score: 79, quality_rank: 4, scheduler_rank: 4, group_rank: 4, eligible: true },
    })
    expect(stale.get('[data-test="status-badge"]').text()).toContain('待确认')
    expect(stale.get('[data-test="score-metric"]').text()).toContain('79')
    expect(stale.get('[data-test="rank-metric"]').text()).toContain('第 4')
  })

  it('uses probe evidence wording, links the account homepage, and exposes score composition', () => {
    const wrapper = mountCard()
    expect(wrapper.get('[data-test="score-metric"]').text()).not.toContain('主动探测')
    expect(wrapper.get('[data-test="account-homepage-link"]').attributes()).toMatchObject({ href: 'https://upstream.example.com/v1', target: '_blank', rel: 'noopener noreferrer' })
    expect(wrapper.get('[data-test="score-metric"]').attributes('title')).toContain('当前服务评分 91 / 100')
    expect(wrapper.get('[data-test="score-metric"]').attributes('title')).not.toContain('探测成功率')
  })

  it('shows the historical source when the score is carried from the last valid result', () => {
    const wrapper = mountCard({ account: { ...account, evidence_source: 'historical_final', checked_at: '2026-08-25T08:00:00Z', quality_score: 88 } })
    expect(wrapper.get('[data-test="score-metric"]').text()).toContain('评分沿用最近一次有效结果')
    expect(wrapper.get('[data-test="score-metric"]').text()).toContain('历史最终结果时间')
  })

  it('shows the score breakdown in an application tooltip on click', async () => {
    const wrapper = mountCard()
    await wrapper.get('[data-test="score-tooltip-trigger"]').trigger('click')
    await nextTick()
    const tooltip = document.body.querySelector('[data-test="score-breakdown-tooltip"]')
    expect(tooltip?.textContent).toContain('当前服务评分 91 / 100')
    expect(tooltip?.textContent).not.toContain('探测成功率')
    expect(tooltip?.textContent).not.toContain('主动探测')
  })

  it('opens score and cost details by click so later cards remain usable after scrolling', async () => {
    const wrapper = mountCard({ account: { ...account, account_type: 'apikey', multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 } } })

    await wrapper.get('[data-test="score-tooltip-trigger"]').trigger('click')
    await nextTick()
    expect(document.body.querySelector('[data-test="score-breakdown-tooltip"]')?.textContent).toContain('当前服务评分 91 / 100')

    await wrapper.get('[data-test="cost-tooltip-trigger"]').trigger('click')
    await nextTick()
    expect(document.body.querySelector('[data-test="cost-source-tooltip"]')?.textContent).toContain('手工维护')
  })

  it('marks manually maintained cost with a warning and explains its source', () => {
    const wrapper = mountCard({ account: { ...account, account_type: 'apikey', multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 } } })
    expect(wrapper.get('[data-test="cost-tooltip-trigger"] button').attributes('title')).toContain('手工维护')
  })

  it('shows the cost source in an application tooltip on click', async () => {
    const wrapper = mountCard({ account: { ...account, account_type: 'apikey', multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 } } })
    await wrapper.get('[data-test="cost-tooltip-trigger"]').trigger('click')
    await nextTick()
    expect(document.body.querySelector('[data-test="cost-source-tooltip"]')?.textContent).toContain('手工维护')
  })

  it('exposes the native billing cost source tooltip', () => {
    const wrapper = mountCard({ account: { ...account, account_type: 'apikey', multiplier: { value: 0.08, source: 'declared', status: 'ok', sample_count: 72 } } })
    expect(wrapper.get('[data-test="cost-tooltip-trigger"] button').attributes('title')).toContain('上游原生')
  })

  it('restores the rejected V3 green service card shell, five colored metrics, probe bars, and service-only footer', async () => {
    const refresh = vi.fn()
    const wrapper = mountCard({
      statisticsCutoff: '2026-08-04T04:20:42Z',
      onRefresh: refresh,
    })

    const card = wrapper.get('[data-test="monitor-card"]')
    expect(card.classes()).toEqual(expect.arrayContaining(['rounded-lg', 'w-full']))
    expect(card.classes()).not.toContain('border-l-4')
    expect(card.classes()).not.toContain('card')
    expect(wrapper.get('[data-test="monitor-card-header"]').classes()).toEqual(expect.arrayContaining(['px-[18px]', 'py-4']))
    expect(wrapper.get('[data-test="score-metric"]').classes()).toEqual(expect.arrayContaining(['min-h-[121px]', 'p-[14px]']))
    expect(wrapper.findAll('.service-metric')).toHaveLength(5)
    expect(wrapper.get('[data-test="success-rate-metric"]').classes()).toContain('bg-emerald-50')
    expect(wrapper.get('[data-test="ttft-metric"]').classes()).toContain('bg-blue-50')
    expect(wrapper.get('[data-test="latency-metric"]').classes()).toContain('bg-amber-50')
    expect(wrapper.get('[data-test="cost-metric"]').classes()).toContain('bg-violet-50')
    expect(wrapper.get('[data-test="concurrency-metric"]').classes()).toContain('bg-gray-50')
    expect(wrapper.findAll('[data-test="probe-bar"]')).toHaveLength(24)
    expect(wrapper.find('[data-test="probe-summary"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="probe-section"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="timeline-section"]').text()).not.toContain('探测失败')
    expect(wrapper.get('[data-test="timeline-section"]').text()).not.toContain('失败')
    expect(wrapper.get('[data-test="timeline-section"]').html()).not.toContain('探测失败')
    expect(wrapper.get('[data-test="timeline-section"]').html()).not.toContain('evidence_source')
    expect(wrapper.get('[data-test="calls-disclosure"]').text()).toContain('24 小时调用')
    expect(wrapper.get('[data-test="calls-disclosure"]').text()).toContain('72 次请求 · 1 次失败')
    expect(wrapper.get('[data-test="card-footer"]').text()).toContain('检查于')
    expect(wrapper.get('[data-test="card-footer"]').text()).toContain('统计截止')

    await wrapper.get('[data-test="refresh-account"]').trigger('click')
    expect(refresh).toHaveBeenCalledWith(113)

    for (const label of ['营收', '利润', '经营', '账务', '对账', '流水', '历史', '异常', '调整']) {
      expect(wrapper.text()).not.toContain(label)
    }
  })

  it('keeps metric milliseconds on one line, wraps a mobile identity safely, and stacks the short cutoff footer', () => {
    const wrapper = mountCard({
      account: { ...account, name: 'a-very-long-account-name-that-must-not-be-truncated@example.com' },
      statisticsCutoff: '2026-08-04T04:20:42Z',
    })

    expect(wrapper.get('[data-test="ttft-metric"]').text()).toContain('1018 ms')
    expect(wrapper.get('[data-test="ttft-metric-value"]').classes()).toContain('whitespace-nowrap')
    expect(wrapper.get('[data-test="latency-metric-value"]').classes()).toContain('whitespace-nowrap')
    const identity = wrapper.get('[data-test="account-identity"]')
    expect(identity.classes()).toContain('break-words')
    expect(identity.classes()).not.toContain('truncate')
    expect(identity.text()).toContain('a-very-long-account-name-that-must-not-be-truncated@example.com')
    expect(identity.text()).toContain('#113')
    expect(wrapper.get('[data-test="card-footer"]').text()).toContain('统计截止 12:20')
    expect(wrapper.get('[data-test="card-footer"]').text()).not.toContain('统计截止 2026')
    expect(wrapper.get('[data-test="card-footer"]').classes()).toContain('max-[430px]:flex-col')
  })

  it('keeps the call disclosure aligned with the selected window instead of optional account range or probe totals', () => {
    const wrapper = mountCard({
      account: { ...account, range: '24h', request_count: 426, error_count: 8 },
      selectedRange: '7d',
    })

    const disclosure = wrapper.get('[data-test="calls-disclosure"]').text()
    expect(disclosure).toContain('7 天调用')
    expect(disclosure).toContain('426 次请求 · 8 次失败')
    expect(disclosure).not.toContain('24 次请求')
  })

  it('renders procurement cost with its expected usable quota and derived multiplier', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        procurement_cost_cny: 120,
        estimated_usable_quota_usd: 120,
        cost_mode: 'procurement',
        effective_multiplier: 0.48,
      },
    })

    const cost = wrapper.get('[data-test="cost-metric"]').text()
    expect(cost).toContain('¥120.00')
    expect(cost).toContain('预计可用额度 120 USD')
    expect(cost).toContain('预计成本倍率 1.00×')
    expect(cost).not.toContain('0.58×')
  })

  it('shows a saved manual multiplier for an OpenAI API Key account', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'apikey',
        multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 },
      },
    })

    expect(wrapper.get('[data-test="cost-metric"]').text()).toContain('0.08×')
    expect(wrapper.get('[data-test="cost-detail"]').text()).toContain('手工录入倍率')
  })

  it('renders a saved manual multiplier when the API Key type uses the underscored wire spelling', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'api_key',
        multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 },
      },
    })

    expect(wrapper.get('[data-test="cost-metric"]').text()).toContain('0.08×')
    expect(wrapper.get('[data-test="cost-detail"]').text()).toContain('手工录入倍率')
  })

  it('updates the visible cost when a post-save range reload replaces the account snapshot', async () => {
    const wrapper = mountCard({
      account: { ...account, account_type: 'apikey', multiplier: { value: 0.58, source: 'declared', status: 'ok', sample_count: 72 } },
    })

    await wrapper.setProps({
      account: { ...account, account_type: 'apikey', multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 } },
    })

    expect(wrapper.get('[data-test="cost-metric"]').text()).toContain('0.08×')
    expect(wrapper.get('[data-test="cost-detail"]').text()).toContain('手工录入倍率')
  })

  it('uses the OpenAI API Key multiplier even when stale procurement fields exist', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'apikey',
        procurement_cost_cny: 120,
        estimated_usable_quota_usd: 120,
        multiplier: { value: 0.08, source: 'manual', status: 'ok', sample_count: 72 },
      },
    })

    const cost = wrapper.get('[data-test="cost-metric"]').text()
    expect(cost).toContain('0.08×')
    expect(cost).not.toContain('¥120.00')
  })

  it('does not fall back to stale procurement fields when an OpenAI API Key multiplier is unavailable', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'api_key',
        procurement_cost_cny: 120,
        estimated_usable_quota_usd: 120,
        multiplier: { value: null, source: 'manual', status: 'failed', sample_count: 0 },
      },
    })

    const cost = wrapper.get('[data-test="cost-metric"]').text()
    expect(cost).toContain('--')
    expect(cost).not.toContain('¥120.00')
  })

  it('uses procurement as the sole source for OpenAI non-API-Key accounts', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        account_type: 'oauth',
        procurement_cost_cny: null,
        estimated_usable_quota_usd: null,
        multiplier: { value: 0.08, source: 'declared', status: 'ok', sample_count: 72 },
      },
    })

    const cost = wrapper.get('[data-test="cost-metric"]').text()
    expect(cost).toContain('--')
    expect(cost).toContain('成本待确认')
    expect(cost).not.toContain('0.08×')
  })

  it('keeps procurement cost actions below the metric detail so the V3 five-tile label does not collapse', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        procurement_cost_cny: 120,
        procurement_cost_effective_at: '2026-08-04T00:00:00Z',
        expires_at: '2026-09-01T00:00:00Z',
        cost_mode: 'procurement',
      },
    })

    const detail = wrapper.get('[data-test="cost-detail"]')
    const actions = wrapper.get('[data-test="cost-actions"]')
    expect(detail.element.compareDocumentPosition(actions.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
  })

  it('marks procurement cost as pending when expected usable quota is missing', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        procurement_cost_cny: 120,
        estimated_usable_quota_usd: null,
        cost_mode: 'procurement',
        effective_multiplier: null,
      },
    })

    expect(wrapper.get('[data-test="cost-metric"]').text()).toContain('成本待确认')
  })

  it('rejects priority values below one or with a fraction before asking the parent to save', async () => {
    const updatePriority = vi.fn()
    const wrapper = mountCard({ onUpdatePriority: updatePriority })

    await wrapper.get('[data-test="edit-priority"]').trigger('click')
    const input = wrapper.get<HTMLInputElement>('[data-test="priority-input"]')
    await input.setValue('1.5')
    await wrapper.get('[data-test="save-priority"]').trigger('click')

    expect(updatePriority).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="priority-error"]').text()).toContain('请输入大于或等于 1 的整数')
    expect((wrapper.get<HTMLInputElement>('[data-test="priority-input"]').element).value).toBe('1.5')
  })

  it('keeps priority editing until the parent confirms the save, then shows the saved value', async () => {
    const pending = deferred()
    const wrapper = mountCard({
      onUpdatePriority: (_id: number, _value: number, completion: { resolve: () => void, reject: (reason?: unknown) => void }) => {
        pending.promise.then(completion.resolve, completion.reject)
      },
    })

    await wrapper.get('[data-test="edit-priority"]').trigger('click')
    await wrapper.get<HTMLInputElement>('[data-test="priority-input"]').setValue('2')
    const saveButton = wrapper.get<HTMLButtonElement>('[data-test="save-priority"]')
    saveButton.element.focus()
    await saveButton.trigger('click')

    expect(wrapper.find('[data-test="priority-input"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="save-priority"]').attributes('disabled')).toBeDefined()

    pending.resolve()
    await flushAsyncWork()

    expect(wrapper.find('[data-test="priority-input"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="priority-control"]').text()).toContain('2')
  })

  it('preserves priority draft, focus, and local error when the parent rejects the save', async () => {
    const wrapper = mountCard({
      onUpdatePriority: (_id: number, _value: number, completion: { reject: (reason?: unknown) => void }) => completion.reject(new Error('保存失败')),
    })

    await wrapper.get('[data-test="edit-priority"]').trigger('click')
    await wrapper.get<HTMLInputElement>('[data-test="priority-input"]').setValue('2')
    await wrapper.get('[data-test="save-priority"]').trigger('click')
    await flushAsyncWork()

    const input = wrapper.get<HTMLInputElement>('[data-test="priority-input"]')
    expect(input.element.value).toBe('2')
    expect(wrapper.get('[data-test="priority-error"]').text()).toContain('保存失败')
    expect(document.activeElement).toBe(input.element)
  })

  it('retains the last concurrency snapshot and marks it delayed', () => {
    const wrapper = mountCard({ concurrency: { account_id: 113, current: 3, limit: 10, delayed: true } })

    expect(wrapper.get('[data-test="concurrency-metric"]').text()).toContain('3 / 10')
    expect(wrapper.get('[data-test="concurrency-metric"]').text()).toContain('数据延迟')
  })

  it('keeps model detection details collapsed and uses cautious abnormal wording', async () => {
    const detectorAccount = {
      ...account,
      connection_probe_model: 'gpt-5.6-sol',
      model_detection: {
        status: 'abnormal',
        settings: { account_id: 113, connection_probe_model: 'gpt-5.6-sol', model_detection_model: 'gpt-5.6-sol' },
        model_options: [{ id: 'gpt-5.6-sol', supported: true, selected: true }],
        recent: {
          status: 'abnormal',
          claimed_model: 'gpt-5.6-sol',
          juice_status: 'mismatch',
          juice_summary: { score: 0.9 },
          fingerprint_candidate: 'gpt-5.6-luna',
          fingerprint_similarity: { 'gpt-5.6-luna': 0.98 },
          detector_version: 'test',
          error_code: 'fingerprint_mismatch',
          error_message: '证据不一致',
          finished_at: '2026-08-17T10:06:00+08:00',
        },
      },
    }
    const wrapper = mountCard({ account: detectorAccount })
    expect(wrapper.find('[data-test="model-detection-dialog"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="model-detection-status-row"]').text()).toContain('检测器观察到异常')
    await wrapper.get('[data-test="model-detection-status-row"]').trigger('click')
    const dialog = wrapper.get('[data-test="model-detection-dialog"]')
    expect(dialog.text()).toContain('不代表上游确认替换')
    expect(dialog.get('[data-test="model-detection-juice-summary"]').text()).toContain('{"score":0.9}')
    expect(dialog.get('[data-test="model-detection-fingerprint-similarity"]').text()).toContain('{"gpt-5.6-luna":0.98}')
    expect(dialog.get('[data-test="model-detection-finished-at"]').text()).toContain('2026')
    expect(dialog.get('[data-test="model-detection-error"]').text()).toContain('fingerprint_mismatch')
    expect(dialog.get('[data-test="model-detection-error"]').text()).toContain('证据不一致')
  })

  it('labels model detection details when the visible evidence is historical', async () => {
    const wrapper = mountCard({
      account: {
        ...account,
        model_detection: {
          status: 'insufficient',
          settings: { account_id: 113, connection_probe_model: 'gpt-5.6-sol', model_detection_model: 'gpt-5.6-sol' },
          model_options: [{ id: 'gpt-5.6-sol', supported: true, selected: true }],
          recent: { status: 'normal', source: 'historical_final', detector_version: 'detector-1', finished_at: '2026-08-25T08:00:00Z' },
          current: { status: 'insufficient', source: 'current', finished_at: '2026-08-25T09:00:00Z' },
        },
      },
    })
    expect(wrapper.get('[data-test="model-detection-status-row"]').text()).toContain('当前证据不足')
    await wrapper.get('[data-test="model-detection-status-row"]').trigger('click')
    expect(wrapper.get('[data-test="model-detection-history-fallback"]').text()).toContain('当前证据不足')
    expect(wrapper.get('[data-test="model-detection-history-fallback"]').text()).toContain('2026')
  })

  it('does not label a legacy detection row as a fresh normal result', () => {
    const wrapper = mountCard({
      account: {
        ...account,
        model_detection: {
          status: 'normal',
          settings: { account_id: 113, connection_probe_model: 'gpt-5.6-sol', model_detection_model: 'gpt-5.6-sol' },
          model_options: [{ id: 'gpt-5.6-sol', supported: true, selected: true }],
          recent: { status: 'normal', source: 'historical', finished_at: '2026-08-25T08:00:00Z' },
        },
      },
    })
    expect(wrapper.get('[data-test="model-detection-status-row"]').text()).toContain('历史记录')
    expect(wrapper.get('[data-test="model-detection-status-row"]').text()).not.toContain('正常')
  })

  it('shows mapping evidence without confusing the catalog with the upstream response model', async () => {
    const wrapper = mountCard({
      account: {
        ...account,
        model_detection: {
          status: 'abnormal',
          settings: { account_id: 113, connection_probe_model: 'gpt-5.6-sol', model_detection_model: 'gpt-5.6-sol' },
          model_options: [{ id: 'gpt-5.6-sol', supported: true, selected: true }],
          recent: {
            status: 'abnormal',
            claimed_model: 'gpt-5.6-sol',
            juice_status: 'suspected_mapping',
            juice_summary: {
              evidence_version: 'model-detection-evidence-v1',
              requested_model: 'gpt-5.6-sol',
              catalog: { status: 'missing', returned_count: 2, returned_models: ['gpt-5.4', 'gpt-5.6-terra'] },
              active_response: { status: 'match', returned_model: 'gpt-5.6-sol' },
              fingerprint: { status: 'unavailable', candidate: '', similarity: 0 },
              verdict: 'suspected_mapping',
            },
            detector_version: 'native-2',
            error_code: 'model_not_advertised',
          },
        },
      },
    })

    await wrapper.get('[data-test="model-detection-status-row"]').trigger('click')
    const dialog = wrapper.get('[data-test="model-detection-dialog"]')
    expect(dialog.get('[data-test="model-detection-verdict"]').text()).toContain('疑似模型映射')
    expect(dialog.get('[data-test="model-detection-requested-model"]').text()).toContain('gpt-5.6-sol')
    expect(dialog.get('[data-test="model-detection-response-model"]').text()).toContain('gpt-5.6-sol')
    expect(dialog.get('[data-test="model-detection-catalog"]').text()).toContain('目录共返回 2 个模型：gpt-5.4、gpt-5.6-terra')
    expect(dialog.get('[data-test="model-detection-fingerprint"]').text()).toContain('未检测行为指纹')
  })

  it('writes the upstream returned model when an active response indicates replacement', async () => {
    const wrapper = mountCard({
      account: {
        ...account,
        model_detection: {
          status: 'abnormal',
          settings: { account_id: 113, connection_probe_model: 'gpt-5.6-sol', model_detection_model: 'gpt-5.6-sol' },
          model_options: [{ id: 'gpt-5.6-sol', supported: true, selected: true }],
          recent: {
            status: 'abnormal',
            claimed_model: 'gpt-5.6-sol',
            juice_summary: {
              evidence_version: 'model-detection-evidence-v1',
              requested_model: 'gpt-5.6-sol',
              catalog: { status: 'match', returned_count: 2, returned_models: ['gpt-5.4', 'gpt-5.6-sol'] },
              active_response: { status: 'mismatch', returned_model: 'gpt-5.4' },
              fingerprint: { status: 'unavailable', candidate: '', similarity: 0 },
              verdict: 'suspected_replacement',
            },
            error_code: 'response_or_fingerprint_mismatch',
          },
        },
      },
    })

    await wrapper.get('[data-test="model-detection-status-row"]').trigger('click')
    const dialog = wrapper.get('[data-test="model-detection-dialog"]')
    expect(dialog.get('[data-test="model-detection-verdict"]').text()).toContain('疑似替换模型')
    expect(dialog.get('[data-test="model-detection-requested-model"]').text()).toContain('gpt-5.6-sol')
    expect(dialog.get('[data-test="model-detection-response-model"]').text()).toContain('gpt-5.4')
  })

  it('states when the active response omitted model and does not borrow a catalog candidate', async () => {
    const wrapper = mountCard({
      account: {
        ...account,
        model_detection: {
          status: 'insufficient',
          settings: { account_id: 113, connection_probe_model: 'gpt-5.6-sol', model_detection_model: 'gpt-5.6-sol' },
          model_options: [{ id: 'gpt-5.6-sol', supported: true, selected: true }],
          recent: {
            status: 'insufficient',
            claimed_model: 'gpt-5.6-sol',
            juice_summary: {
              evidence_version: 'model-detection-evidence-v1',
              requested_model: 'gpt-5.6-sol',
              catalog: { status: 'match', returned_count: 2, returned_models: ['gpt-5.4', 'gpt-5.6-sol'] },
              active_response: { status: 'missing', returned_model: '' },
              fingerprint: { status: 'unavailable', candidate: '', similarity: 0 },
              verdict: 'insufficient',
            },
            error_code: 'response_model_missing',
          },
        },
      },
    })

    await wrapper.get('[data-test="model-detection-status-row"]').trigger('click')
    const responseRow = wrapper.get('[data-test="model-detection-response-model"]')
    expect(responseRow.text()).toContain('上游未返回 model 字段')
    expect(responseRow.text()).not.toContain('gpt-5.4')
  })

  it('shows both returned model and fingerprint candidate when evidence conflicts', async () => {
    const wrapper = mountCard({
      account: {
        ...account,
        model_detection: {
          status: 'abnormal',
          settings: { account_id: 113, connection_probe_model: 'gpt-5.6-sol', model_detection_model: 'gpt-5.6-sol' },
          model_options: [{ id: 'gpt-5.6-sol', supported: true, selected: true }],
          recent: {
            status: 'abnormal',
            claimed_model: 'gpt-5.6-sol',
            fingerprint_candidate: 'gpt-5.6-terra',
            fingerprint_similarity: { 'gpt-5.6-terra': 0.98 },
            juice_summary: {
              evidence_version: 'model-detection-evidence-v1',
              requested_model: 'gpt-5.6-sol',
              catalog: { status: 'missing', returned_count: 1, returned_models: ['gpt-5.4'] },
              active_response: { status: 'mismatch', returned_model: 'gpt-5.4' },
              fingerprint: { status: 'mismatch', candidate: 'gpt-5.6-terra', similarity: 0.98 },
              verdict: 'high_risk_inconsistent',
            },
            error_code: 'evidence_inconsistent',
          },
        },
      },
    })

    await wrapper.get('[data-test="model-detection-status-row"]').trigger('click')
    const dialog = wrapper.get('[data-test="model-detection-dialog"]')
    expect(dialog.get('[data-test="model-detection-verdict"]').text()).toContain('高风险不一致')
    expect(dialog.get('[data-test="model-detection-response-model"]').text()).toContain('gpt-5.4')
    expect(dialog.get('[data-test="model-detection-fingerprint"]').text()).toContain('gpt-5.6-terra')
    expect(dialog.get('[data-test="model-detection-fingerprint"]').text()).toContain('98%')
  })

  it('exposes an explicit connection probe model entry next to recent probes', async () => {
    const editConnectionProbeModel = vi.fn()
    const wrapper = mountCard({ onEditConnectionProbeModel: editConnectionProbeModel })
    await wrapper.get('[data-test="edit-connection-probe-model"]').trigger('click')
    expect(editConnectionProbeModel).toHaveBeenCalledWith(account)
  })

  it.each([
    ['unconfigured', '检测服务未接入'],
    ['unavailable', '检测服务暂不可用'],
  ])('does not mislabel %s detector state as unsupported', async (state, label) => {
    const wrapper = mountCard({
      account: { ...account, model_detection: { status: state === 'unconfigured' ? 'service_unconfigured' : 'service_unavailable', settings: { account_id: 113, connection_probe_model: 'gpt-4o', model_detection_model: '' }, model_options: [] } },
      modelDetectionModels: { account_id: 113, detector_state: state, connection_probe_model: 'gpt-4o', model_detection_model: '', connection_models: [{ id: 'gpt-4o', supported: true, selected: true }], detection_models: [{ id: 'gpt-4o', supported: false, selected: false, reason: `detector_${state}` }] },
    })
    await wrapper.get('[data-test="model-detection-status-row"]').trigger('click')
    const dialog = wrapper.get('[data-test="model-detection-dialog"]')
    expect(dialog.get('[data-test="detector-availability"]').text()).toContain(label)
    expect(dialog.text()).not.toContain('检测器暂不支持')
    expect(dialog.get('[data-test="connection-model-select"]').attributes('disabled')).toBeUndefined()
    expect(dialog.get('[data-test="model-detection-run"]').attributes('disabled')).toBeDefined()
  })

  it('uses unsupported copy only for a ready catalog that omits the native model', async () => {
    const wrapper = mountCard({
      account: { ...account, model_detection: { status: 'unsupported', settings: { account_id: 113, connection_probe_model: 'gpt-4o', model_detection_model: '' }, model_options: [] } },
      modelDetectionModels: { account_id: 113, detector_state: 'ready', connection_probe_model: 'gpt-4o', model_detection_model: '', connection_models: [{ id: 'gpt-4o', supported: true, selected: true }], detection_models: [{ id: 'gpt-4o', supported: false, selected: false, reason: 'detector_unsupported' }] },
    })
    await wrapper.get('[data-test="model-detection-status-row"]').trigger('click')
    const dialog = wrapper.get('[data-test="model-detection-dialog"]')
    expect(dialog.text()).toContain('检测器暂不支持')
    expect(dialog.find('[data-test="detector-availability"]').exists()).toBe(false)
  })
})
