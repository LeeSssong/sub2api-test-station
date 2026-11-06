// Local browser fixture only. Production scripts/clipboard/download remain the real component paths.
import { createApp, h, ref } from 'vue'
import { createPinia } from 'pinia'
import CodexQuickConfigModal from '@/components/keys/CodexQuickConfigModal.vue'
import { i18n, loadLocaleMessages } from '@/i18n'
import '@/style.css'

if (import.meta.env.MODE !== 'browser-test') throw new Error('Local browser-test mode required')
const query = new URLSearchParams(location.search)
const locale = query.get('locale') === 'en' ? 'en' : 'zh'
i18n.global.locale.value = locale
await loadLocaleMessages(locale)
document.documentElement.lang = locale
document.documentElement.classList.add('dark')
const nativeFetch = window.fetch.bind(window)
window.fetch = async (input, init) => {
  const url = new URL(input instanceof Request ? input.url : String(input), location.href)
  if (url.origin === location.origin && url.pathname === '/v1/models') {
    return new Response(JSON.stringify(query.get('catalog') === 'error'
      ? { error: { message: 'Local fixture: optional catalog unavailable', code: 'fixture_error' } }
      : { models: [{ slug: 'preview-model', display_name: 'Local preview model' }] }), {
      status: query.get('catalog') === 'error' ? 503 : 200,
      headers: { 'Content-Type': 'application/json' }
    })
  }
  return nativeFetch(input, init)
}
const App = {
  setup() {
    const show = ref(true)
    return () => h('main', { class: 'user-app-shell min-h-screen p-6' }, [
      h('p', { class: 'mb-3 text-sm' }, '工程组件预览 · 无效演示密钥/本地目录fixture · 复制和下载为真实浏览器操作，请勿运行下载脚本'),
      h('button', { class: 'btn btn-secondary', id: 'preview-reopen', onClick: () => { show.value = true } }, '打开配置弹窗'),
      h(CodexQuickConfigModal, { show: show.value, apiKey: 'sk-preview-invalid-not-a-key', baseUrl: location.origin, platform: 'openai', onClose: () => { show.value = false } })
    ])
  }
}
const app = createApp(App)
app.use(createPinia()).use(i18n)
app.provide('starbridge-user', ref(true))
app.mount('#app')
