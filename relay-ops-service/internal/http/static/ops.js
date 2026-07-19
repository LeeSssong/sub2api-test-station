(() => {
  const status = document.getElementById('ops-status')
  const token = localStorage.getItem('auth_token')
  const loginURL = '/login?redirect=%2Fops'

  if (!token) {
    window.location.replace(loginURL)
    return
  }

  fetch('/relay-ops/api/ops-view', {
    credentials: 'same-origin',
    headers: {
      Accept: 'text/html',
      Authorization: `Bearer ${token}`,
    },
  })
    .then((response) => {
      if (response.status === 401 || response.status === 403) {
        window.location.replace(loginURL)
        return null
      }
      if (!response.ok) {
        throw new Error('ops view unavailable')
      }
      return response.text()
    })
    .then((html) => {
      if (html === null) return
      document.open()
      document.write(html)
      document.close()
    })
    .catch(() => {
      if (status) status.textContent = '运维数据暂时不可用，请刷新重试'
    })
})()
