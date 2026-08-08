<script setup lang="ts">
import { ArrowUp } from 'lucide-vue-next'
import { onMounted, onUnmounted, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  target?: HTMLElement | null
  threshold?: number
}>(), {
  target: null,
  threshold: 360,
})

const visible = ref(false)

// scrollTarget 获取当前需要监听和滚动的容器，未传入时回退到页面根滚动。
function scrollTarget() {
  return props.target || document.documentElement
}

// updateVisible 根据滚动距离决定是否显示回到顶部按钮。
function updateVisible() {
  const target = scrollTarget()
  visible.value = target.scrollTop > props.threshold
}

// scrollTop 将目标容器平滑滚动到顶部。
function scrollTop() {
  scrollTarget().scrollTo({ top: 0, behavior: 'smooth' })
}

// bind 绑定滚动监听，并立即同步一次显示状态。
function bind(target: HTMLElement | null) {
  ;(target || window).addEventListener('scroll', updateVisible, { passive: true })
  updateVisible()
}

// unbind 解绑旧滚动容器，避免切换页面时残留监听。
function unbind(target: HTMLElement | null) {
  ;(target || window).removeEventListener('scroll', updateVisible)
}

watch(
  () => props.target,
  (next, prev) => {
    unbind(prev)
    bind(next)
  },
)

onMounted(() => bind(props.target))
onUnmounted(() => unbind(props.target))
</script>

<template>
  <button
    v-if="visible"
    class="fixed bottom-8 right-8 z-40 flex size-10 items-center justify-center rounded-full border border-gray-200 bg-white text-gray-600 shadow-lg transition-colors hover:bg-gray-100 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
    title="回到顶部"
    @click="scrollTop"
  >
    <ArrowUp class="size-4" />
  </button>
</template>
