import overview from './overview'
import channels from './channels'
import accounts from './accounts'
import resources from './resources'
import ops from './ops'
import settings from './settings'
import audit from './audit'
import promptAudit from './promptAudit'

export default {
  ...overview,
  ...channels,
  ...accounts,
  ...resources,
  ...ops,
  ...settings,
  ...audit,
  ...promptAudit,
  accountMonitor: {
    title: '账号监控',
    description: '只监控当前启用且可调度的上游账号，集中查看探测质量、倍率和使用窗口。',
    monitoredCount: '监控中 {count} 个账号',
    interval: '全局间隔',
    intervalMinutes: '{count} 分钟',
    intervalSeconds: '{count} 秒',
    lastObserved: '观测于 {time}',
    loadError: '账号监控数据加载失败',
    actions: {
      refreshAll: '立即刷新全部',
      running: '运行中...',
      refreshOne: '立即刷新此账号',
      settings: '监控设置',
      history: '查看历史',
    },
    status: {
      success: '正常',
      failed: '失败',
      balance_exhausted: '余额不足',
      stale: '数据过期',
      unavailable: '暂无结果',
      noHistory: '暂无历史',
    },
    metrics: {
      successRate: '成功率',
      ttft: 'TTFT P50',
      latency: '延迟 P95',
      multiplier: '倍率',
    },
    today: {
      title: '今日调用',
      requests: '请求 {count} 次',
      errors: '错误 {count} 次',
    },
    card: {
      noGroups: '未加入分组',
      usageWindows: '官方使用窗口',
      checkedAt: '检查于 {time}',
    },
    filters: {
      searchPlaceholder: '搜索账号、平台、模型或分组',
      platform: '平台',
      status: '状态',
    },
    settings: {
      title: '账号监控设置',
      intervalLabel: '全局刷新间隔（秒）',
      intervalHint: '范围 15–3600 秒；所有账号共用此间隔。',
    },
    empty: {
      filtered: '没有符合当前筛选条件的账号。',
      pool: '当前没有启用且可调度的账号。',
    },
    messages: {
      refreshAllSuccess: '全部账号刷新完成',
      refreshFailed: '账号刷新失败',
      settingsSaved: '监控间隔已保存',
      settingsFailed: '监控间隔保存失败',
    },
    history: {
      title: '监控历史',
      checkedAt: '检查时间',
      status: '状态',
      ttft: 'TTFT',
      latency: '延迟',
      error: '错误',
      empty: '暂无监控历史',
      loadError: '监控历史加载失败',
    },
  },
}
