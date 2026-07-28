(() => {
  const refreshStatus = document.getElementById('refresh-status')
  const acknowledgementStatus = document.getElementById('incident-ack-status')
  const token = localStorage.getItem('auth_token')
  const notFound = () => window.location.replace('/404')

  const acknowledgeIncident = async () => {
    const query = new URLSearchParams(window.location.search)
    const incidentKey = query.get('ack_incident')
    const occurrenceValue = query.get('ack_occurrence')
    if (incidentKey === null && occurrenceValue === null) return

    window.history.replaceState(null, '', `${window.location.pathname}${window.location.hash}`)
    if (!incidentKey || !/^[1-9][0-9]*$/.test(occurrenceValue || '') || !acknowledgementStatus) {
      if (acknowledgementStatus) acknowledgementStatus.textContent = '确认链接无效，请返回告警卡片重试'
      return
    }
    if (!token) {
      notFound()
      return
    }
    try {
      const response = await fetch('/relay-ops/api/incidents/ack', {
        method: 'POST',
        cache: 'no-store',
        credentials: 'same-origin',
        headers: {
          Accept: 'application/json',
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          incident_key: incidentKey,
          occurrence_no: Number(occurrenceValue),
        }),
      })
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        notFound()
        return
      }
      if (response.status === 409) {
        acknowledgementStatus.textContent = '该事件已恢复或告警轮次已变化，无需重复确认'
        return
      }
      if (!response.ok) throw new Error('acknowledgement unavailable')
      acknowledgementStatus.textContent = '已确认并接手，后续升级提醒已停止'
    } catch (_) {
      acknowledgementStatus.textContent = '确认失败，请返回告警卡片重试'
    }
  }

  const hasValidModelocReport = (config) => {
    if (!config || config.version !== 1 || !Array.isArray(config.thirdPartyReports)) return false
    return config.thirdPartyReports.some((report) => {
      if (!report || typeof report !== 'object') return false
      if (typeof report.id !== 'string' || !report.id.trim()) return false
      if (typeof report.provider !== 'string' || report.provider.trim().toUpperCase() !== 'MODELOC') return false
      if (typeof report.title !== 'string' || !report.title.trim()) return false
      if (typeof report.status !== 'string' || !['verified', 'reference', 'archived'].includes(report.status.trim())) return false
      try {
        if (typeof report.url !== 'string') return false
        const url = new URL(report.url.trim())
        return url.protocol === 'https:'
      } catch (_) {
        return false
      }
    })
  }

  const updateModelocReminder = async () => {
    const reminder = document.getElementById('modeloc-reminder')
    if (!reminder) return
    try {
      const response = await fetch('/home-assets/site-config.json', {
        cache: 'no-store',
        credentials: 'same-origin',
        headers: { Accept: 'application/json' },
      })
      if (!response.ok) return
      const config = await response.json()
      if (hasValidModelocReport(config)) reminder.hidden = true
    } catch (_) {
      // Keep the reminder visible when public evidence cannot be verified.
    }
  }

  const refresh = async () => {
    if (!token) {
      notFound()
      return
    }
    try {
      const response = await fetch('/relay-ops/api/ops-view', {
        cache: 'no-store',
        credentials: 'same-origin',
        headers: {
          Accept: 'text/html',
          Authorization: `Bearer ${token}`,
        },
      })
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        notFound()
        return
      }
      if (!response.ok) throw new Error('refresh unavailable')
      const html = await response.text()
      document.open()
      document.write(html)
      document.close()
    } catch (_) {
      if (refreshStatus) refreshStatus.textContent = '自动更新暂时失败，将继续重试'
      window.setTimeout(refresh, 30000)
    }
  }

  updateModelocReminder()
  acknowledgeIncident()
  window.setTimeout(refresh, 30000)
})()
