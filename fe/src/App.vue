<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Copy, X } from 'lucide-vue-next'
import { API_ERROR_EVENT } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { useColorMode } from '@/composables/useColorMode'

const store = useAppStore()
const route = useRoute()
const colorMode = useColorMode()
const isNonPcScreen = ref(false)
const currentLink = computed(() => window.location.href)
const shouldShowNonPcDialog = computed(
  () => route.name !== 'share-reader' && isNonPcScreen.value
)

watch(
  () => store.personalizationSettings.themeMode,
  (mode) => {
    colorMode.preference.value = mode
  },
  { immediate: true },
)

function handleApiError(event: Event) {
  const custom = event as CustomEvent<{ message?: string }>
  store.notifyError(custom.detail?.message || '网络请求失败，请检查后端服务是否正常')
}

// updateScreenType 根据视口和输入能力判断当前是否不是电脑端屏幕。
function updateScreenType() {
  const width = window.innerWidth
  const height = window.innerHeight
  isNonPcScreen.value = width < 768 || (width < 900 && height < 520)
}

// copyCurrentLink 复制当前页面链接，方便用户转到电脑端打开。
async function copyCurrentLink() {
  try {
    await navigator.clipboard.writeText(currentLink.value)
    store.notifyInfo('链接已复制')
  } catch {
    const input = document.createElement('textarea')
    input.value = currentLink.value
    input.style.position = 'fixed'
    input.style.opacity = '0'
    document.body.appendChild(input)
    input.select()
    document.execCommand('copy')
    document.body.removeChild(input)
    store.notifyInfo('链接已复制')
  }
}

onMounted(() => {
  updateScreenType()
  window.addEventListener('resize', updateScreenType)
  window.addEventListener(API_ERROR_EVENT, handleApiError)
})
onUnmounted(() => {
  window.removeEventListener('resize', updateScreenType)
  window.removeEventListener(API_ERROR_EVENT, handleApiError)
})
</script>

<template>
  <router-view />
  <div
    v-if="shouldShowNonPcDialog"
    class="fixed inset-0 z-[90] flex items-center justify-center bg-black/50 px-4"
  >
    <div
      class="w-full max-w-sm rounded-xl bg-white p-5 shadow-2xl dark:bg-gray-900"
    >
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        请在电脑端打开
      </h2>
      <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-gray-400">
        本网站仅做电脑端适配，建议复制下面链接在电脑端打开
      </p>
      <div
        class="mt-4 break-all rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-700 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-200"
      >
        {{ currentLink }}
      </div>
      <button
        class="mt-4 flex w-full items-center justify-center gap-2 rounded-lg bg-gray-900 px-4 py-2.5 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
        @click="copyCurrentLink"
      >
        <Copy class="size-4" />
        复制链接
      </button>
    </div>
  </div>
  <div
    v-if="store.errorToast"
    class="fixed left-1/2 top-6 z-[100] flex max-w-md -translate-x-1/2 items-start gap-3 rounded-lg border px-4 py-3 text-sm shadow-lg"
    :class="store.toastKind === 'error'
      ? 'border-red-200 bg-red-50 text-red-700'
      : 'border-green-200 bg-green-50 text-green-700'"
  >
    <span class="min-w-0 flex-1">{{ store.errorToast }}</span>
    <button class="-mr-1 rounded p-0.5 text-current opacity-60 hover:opacity-100" @click="store.dismissToast()">
      <X class="size-4" />
    </button>
  </div>
</template>
