<template>
  <!-- 图形按钮：只显示图标，文字放在 aria-label（读屏/测试用）和 data-tip 悬浮提示里。
       样式沿用 .btn / .btn-sm / .btn-primary / .btn-danger，颜色全部走主题变量。 -->
  <button type="button" class="btn btn-icon" :class="{ 'is-busy': busy, 'has-badge': badge !== undefined && badge !== '' }"
    :aria-label="label" :data-tip="label">
    <span class="btn-icon-glyph">
      <UiIcon :name="icon" :size="size" />
      <span v-if="plus" class="btn-icon-plus" aria-hidden="true"><UiIcon name="plus" :size="8" :stroke="3.2" /></span>
    </span>
    <span v-if="badge !== undefined && badge !== ''" class="btn-icon-badge" aria-hidden="true">{{ badge }}</span>
  </button>
</template>

<script setup lang="ts">
import UiIcon from '~/components/UiIcon.vue'

withDefaults(defineProps<{
  icon: string
  label: string
  /** 数字角标（如已选数量、失败条数） */
  badge?: string | number
  /** 图标右下角加一个小「+」，表示「新增 xx」 */
  plus?: boolean
  /** 进行中：图标旋转 */
  busy?: boolean
  size?: number
}>(), { size: 16 })
</script>
