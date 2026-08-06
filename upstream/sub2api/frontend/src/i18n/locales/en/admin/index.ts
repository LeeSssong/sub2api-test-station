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
    title: 'Account Monitor',
    description: 'Monitor only active, schedulable upstream accounts and review quality, multipliers, and usage windows in one place.',
    monitoredCount: '{count} accounts monitored',
    interval: 'Global interval',
    intervalMinutes: '{count} min',
    intervalSeconds: '{count} sec',
    lastObserved: 'Observed {time}',
    loadError: 'Failed to load account monitor data',
    actions: {
      refreshAll: 'Refresh all now',
      running: 'Running...',
      refreshOne: 'Refresh this account',
      settings: 'Monitor settings',
      history: 'View history',
    },
    status: {
      success: 'Healthy',
      failed: 'Failed',
      balance_exhausted: 'Balance exhausted',
      stale: 'Stale',
      unavailable: 'No result',
      noHistory: 'No history',
    },
    metrics: {
      successRate: 'Success rate',
      ttft: 'TTFT P50',
      latency: 'Latency P95',
      multiplier: 'Multiplier',
    },
    multiplier: {
      declared: 'Declared upstream',
      measured: 'Measured from quota',
      stale: 'Multiplier expired',
      unsupported: 'Not declared upstream',
      failed: 'Measurement failed',
      unavailable: 'No multiplier probe',
    },
    costGuard: {
      title: 'Cost and group multiplier',
      upstreamMultiplier: 'Native upstream multiplier',
      multiplierSource: 'Multiplier source',
      equivalentSiteMultiplier: 'Equivalent site multiplier',
      costSource: 'Cost source',
      groupMultiplier: 'Current group multiplier',
      status: 'Cost status',
      model: 'Trigger model',
      samples: 'Valid samples',
      observedAt: 'Evidence updated',
      multiplierSources: {
        upstreamDeclared: 'Declared upstream',
        upstreamPricing: 'Upstream pricing',
        quotaMeasurement: 'Quota measurement',
        manual: 'Manual configuration',
        unknown: 'Pending confirmation',
      },
      costSources: {
        reconciledBill: 'Reconciled bill',
        upstreamPricing: 'Upstream pricing estimate',
        quotaMeasurement: 'Quota measurement',
        unknown: 'Pending confirmation',
      },
      statuses: {
        lossConfirmed: 'Confirmed loss',
        lossObserving: 'Loss observing {count}/{required}',
        pricingRisk: 'Possible loss',
        zeroMargin: 'No margin',
        costCovered: 'Cost covered',
        insufficientSamples: 'Insufficient samples',
        unknown: 'Cost pending confirmation',
      },
      alerts: {
        inversion: 'Cost inversion',
        aboveBy: 'above by {gap}',
        observingEvidence: '{count}/{required} valid billed samples so far',
        pricingEvidence: 'Pricing estimate only; per-request actual charge is not confirmed',
        zeroMargin: 'Cost is effectively equal to the group multiplier',
      },
    },
    today: {
      title: 'Today',
      requests: '{count} requests',
      errors: '{count} errors',
    },
    card: {
      noGroups: 'No groups',
      usageWindows: 'Official usage windows',
      checkedAt: 'Checked {time}',
    },
    filters: {
      searchPlaceholder: 'Search account, platform, model, or group',
      platform: 'Platform',
      status: 'Status',
      group: 'Group',
      allGroups: 'All groups',
    },
    settings: {
      title: 'Account monitor settings',
      intervalLabel: 'Global refresh interval (seconds)',
      intervalHint: 'Range: 15–3600 seconds. All accounts use this interval.',
    },
    empty: {
      filtered: 'No accounts match the current filters.',
      pool: 'There are no active, schedulable accounts.',
    },
    messages: {
      refreshAllSuccess: 'All account checks completed',
      refreshFailed: 'Account refresh failed',
      settingsSaved: 'Monitor interval saved',
      settingsFailed: 'Failed to save monitor interval',
    },
    history: {
      title: 'Monitor history',
      checkedAt: 'Checked at',
      status: 'Status',
      ttft: 'TTFT',
      latency: 'Latency',
      error: 'Error',
      empty: 'No monitor history',
      loadError: 'Failed to load monitor history',
    },
  },
}
