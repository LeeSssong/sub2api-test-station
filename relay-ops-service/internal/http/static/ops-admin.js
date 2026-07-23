(() => {
  const refreshStatus = document.getElementById('refresh-status')
  const token = localStorage.getItem('auth_token')
  const notFound = () => window.location.replace('/404')

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
  window.setTimeout(refresh, 30000)
})()
