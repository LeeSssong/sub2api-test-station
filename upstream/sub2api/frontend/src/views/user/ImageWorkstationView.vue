<template>
  <AppLayout><section class="workstation-shell">
    <div v-if="error" class="workstation-message" role="alert"><h2>暂时无法打开生图工作站</h2><p>{{ error }}</p><button class="btn btn-primary" :disabled="opening" @click="enter">重新打开</button></div>
    <div v-else-if="!ready" class="workstation-message" role="status"><p>正在验证本站会话并打开工作站…</p></div>
    <iframe v-if="ready" ref="frame" :src="frameUrl" title="生图工作站" class="workstation-frame" @load="sendTheme" />
  </section></AppLayout>
</template>
<script setup lang="ts">
import {ref,onMounted,onBeforeUnmount} from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import {useAuthStore} from '@/stores/auth'
import {getAuthToken} from '@/api/auth'
const auth=useAuthStore(),ready=ref(false),opening=ref(false),error=ref(''),frameUrl=ref(''),frame=ref<HTMLIFrameElement|null>(null)
let observer:MutationObserver|undefined
function theme(){return document.documentElement.classList.contains('dark')?'dark':'light'}
function sendTheme(){frame.value?.contentWindow?.postMessage({type:'xingqiao-theme',theme:theme()},location.origin)}
async function enter(){if(opening.value)return;opening.value=true;ready.value=false;error.value='';try{
 await auth.refreshUser()
 const token=getAuthToken()||auth.token;if(!token)throw new Error('本站会话已过期，请重新登录本站。')
 const r=await fetch('/image-workstation/api/workstation/enter',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({token})})
 if(!r.ok){const body=await r.json().catch(()=>({}));throw new Error(body.error||`工作站入口返回 ${r.status}，请重试。`)}
 frameUrl.value='/image-workstation/create?theme='+theme()+'&entry='+Date.now();ready.value=true
}catch(e){error.value=e instanceof Error?e.message:'工作站暂不可用，请稍后重试。'}finally{opening.value=false}}
onMounted(()=>{void enter();observer=new MutationObserver(sendTheme);observer.observe(document.documentElement,{attributes:true,attributeFilter:['class']})})
onBeforeUnmount(()=>observer?.disconnect())
</script>
<style scoped>
.workstation-shell{height:calc(100dvh - 110px);min-height:600px;border-radius:14px;overflow:hidden;background:#f8f3eb;color:#302319}.workstation-frame{width:100%;height:100%;border:0}.workstation-message{padding:48px;max-width:720px;margin:0 auto;display:flex;flex-direction:column;align-items:flex-start;gap:18px;line-height:1.6}.workstation-message h2{font-size:22px;font-weight:600}:global(.dark) .workstation-shell{background:#0b1c29;color:#e6f0f4}:global(.dark) .workstation-message p{color:#b8ceda}@media(max-width:700px){.workstation-shell{min-height:calc(100dvh - 110px)}.workstation-message{padding:28px}}
</style>
