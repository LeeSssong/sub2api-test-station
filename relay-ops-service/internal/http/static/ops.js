(() => {
  const status = document.getElementById('ops-status')
  const token = localStorage.getItem('auth_token')
  const acknowledgementQuery = window.location.search
  const notFound = () => window.location.replace('/404')

  if (!token) {
    notFound()
    return
  }

  fetch('/relay-ops/api/ops-view', {
    cache: 'no-store',
    credentials: 'same-origin',
    headers: {
      Accept: 'text/html',
      Authorization: `Bearer ${token}`,
    },
  })
    .then((response) => {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        notFound()
        return null
      }
      if (!response.ok) throw new Error('ops view unavailable')
      return response.text()
    })
    .then((html) => {
      if (html === null) return
      document.open()
      document.write(html)
      document.close()
      if (acknowledgementQuery && window.location.search !== acknowledgementQuery) {
        window.history.replaceState(null, '', `${window.location.pathname}${acknowledgementQuery}${window.location.hash}`)
      }
    })
    .catch(() => {
      if (status) status.textContent = '页面暂时不可用，请刷新重试'
    })
})()
