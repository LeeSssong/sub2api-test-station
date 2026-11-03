<template>
  <section class="rounded-xl border border-gray-200 p-4 dark:border-dark-600" aria-label="平台工具归类">
    <h3 class="text-sm font-semibold text-gray-900 dark:text-white">工具归类</h3>
    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">按分组平台默认归入 {{ defaultTool?.label || '暂无对应工具' }}；保存分组即可，无需单独关联。</p>
    <p v-if="loading" class="mt-2 text-xs text-gray-500" role="status">正在核对历史例外…</p>
    <div v-if="error" class="mt-2 flex items-center gap-3" role="alert">
      <span class="text-sm text-red-600 dark:text-red-400">历史关联读取失败，暂不能确认是否有例外。</span>
      <button type="button" class="btn btn-secondary btn-sm" @click="load">重试</button>
    </div>
    <p v-if="exceptionLabels.length" class="mt-2 text-xs text-amber-700 dark:text-amber-300" role="status">
      历史例外（只读）：当前仍归入 {{ exceptionLabels.join('、') }}。请交负责人核对后再决定保留或迁移；本次不会清除。
    </p>
  </section>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { apiClient } from '@/api/client'
import { tools } from '@/features/ai-tools/model'
const props=defineProps<{groupId:number;platform:string}>()
const defaultTool=computed(()=>tools.find(tool=>tool.platform===props.platform))
const stored=ref<string[]>([]),loading=ref(false),error=ref(false)
let generation=0
const exceptionLabels=computed(()=>{
 const ids=stored.value
 return ids.length && !(ids.length===1 && ids[0]===defaultTool.value?.id) ? ids.map(id=>tools.find(t=>t.id===id)?.label||id) : []
})
async function load(){
 const current=++generation;loading.value=true;error.value=false;stored.value=[]
 try{const {data}=await apiClient.get<{tool_ids:string[]}>('/admin/groups/'+props.groupId+'/tool-mappings');if(current===generation)stored.value=Array.isArray(data.tool_ids)?data.tool_ids:[]}
 catch{if(current===generation)error.value=true}
 finally{if(current===generation)loading.value=false}
}
watch(()=>props.groupId,load,{immediate:true})
</script>
