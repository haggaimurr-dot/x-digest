<template>
  <!-- 分页条：上一页 / 页码（首尾 + 当前页附近，其余省略号）/ 下一页 / 每页条数 / 总数。X 日报与油管、B站共用。 -->
  <nav class="x-pager" aria-label="分页">
    <IconBtn class="btn-sm" icon="chevron-left" label="上一页" :disabled="page <= 1" @click="emit('update:page', page - 1)" />
    <template v-for="(it, ii) in items" :key="'pg' + ii">
      <span v-if="it === '…'" class="x-pager-gap">…</span>
      <button v-else class="btn btn-sm" :class="{ 'btn-primary': it === page }" type="button" :aria-current="it === page ? 'page' : undefined" @click="emit('update:page', it)">{{ it }}</button>
    </template>
    <IconBtn class="btn-sm" icon="chevron-right" label="下一页" :disabled="page >= pageCount" @click="emit('update:page', page + 1)" />
    <label class="x-pager-size">每页
      <select class="field" :value="pageSize" @change="emit('update:pageSize', Number(($event.target as HTMLSelectElement).value))">
        <option v-for="n in sizes" :key="n" :value="n">{{ n }} 条</option>
      </select>
    </label>
    <span class="x-pager-info">共 {{ total }} 条</span>
  </nav>
</template>

<script setup lang="ts">
import IconBtn from '~/components/IconBtn.vue'

const props = defineProps<{ page: number, pageCount: number, pageSize: number, total: number, sizes: number[] }>()
const emit = defineEmits<{ 'update:page': [n: number], 'update:pageSize': [n: number] }>()

const items = computed<Array<number | '…'>>(() => {
  const n = props.pageCount; const c = props.page
  const keep = [...new Set([1, n, c - 1, c, c + 1].filter(x => x >= 1 && x <= n))].sort((a, b) => a - b)
  const out: Array<number | '…'> = []
  keep.forEach((x, i) => { if (i && x - keep[i - 1] > 1) out.push('…'); out.push(x) })
  return out
})
</script>
