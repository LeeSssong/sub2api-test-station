<template><AppLayout><section class="workstation-shell"><p v-if="error" role="alert">{{ error }} <button @click="enter">重试</button></p><p v-else-if="!ready">正在打开生图工作站…</p><iframe v-if="ready" :src="frameUrl" title="生图工作站" class="workstation-frame" /></section></AppLayout></template>
<script setup lang="ts">
import {ref,onMounted} from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import {useAuthStore} from '@/stores/auth'
const auth=useAuthStore(),ready=ref(false),error=ref(''),frameUrl=ref('')
async function enter(){ready.value=false;error.value='';try{const token=auth.token;if(!token)throw new Error('请先登录本站');const r=await fetch('/image-workstation/api/workstation/enter',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({token})});if(!r.ok)throw new Error('工作站暂不可用，请重试');frameUrl.value='/image-workstation/create?entry='+Date.now();ready.value=true}catch(e){error.value=e instanceof Error?e.message:'工作站暂不可用'}}
onMounted(enter)
</script>
<style scoped>.workstation-shell{height:calc(100dvh - 110px);min-height:600px;border-radius:14px;overflow:hidden;background:#f8f3eb}.workstation-frame{width:100%;height:100%;border:0}@media(max-width:700px){.workstation-shell{min-height:calc(100dvh - 110px)}} </style>
