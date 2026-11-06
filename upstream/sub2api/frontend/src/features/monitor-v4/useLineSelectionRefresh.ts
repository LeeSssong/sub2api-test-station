import { onBeforeUnmount, onMounted } from 'vue'

/** Match route details: re-read the mode while a selector is open or regains focus. */
export function useLineSelectionRefresh(active: () => boolean, reload: () => Promise<void>) {
  let timer: ReturnType<typeof setInterval> | undefined
  let pending = false
  async function refresh() {
    if (document.hidden || !active() || pending) return
    pending = true
    try { await reload() } finally { pending = false }
  }
  function visible() { void refresh() }
  onMounted(() => {
    timer = setInterval(visible, 30000)
    window.addEventListener('focus', visible)
    document.addEventListener('visibilitychange', visible)
  })
  onBeforeUnmount(() => {
    clearInterval(timer)
    window.removeEventListener('focus', visible)
    document.removeEventListener('visibilitychange', visible)
  })
}
