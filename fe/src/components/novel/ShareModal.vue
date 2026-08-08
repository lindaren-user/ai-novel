<script setup lang="ts">
import { ref, computed } from 'vue'
import { X, Copy, Eye, EyeOff } from 'lucide-vue-next'
import { useAppStore } from '@/stores/app'

const store = useAppStore()

const copied = ref(false)
const showShareKey = ref(false)

const shareLink = computed(() => store.shareLink)

const truncatedDesc = computed(() => {
  const t = store.shareTarget
  if (!t) return ''
  return t.description.length > 80 ? t.description.slice(0, 80) + '...' : t.description
})

async function copyLink() {
  if (!shareLink.value) return
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(shareLink.value)
    } else {
      copyWithTextarea(shareLink.value)
    }
    copied.value = true
    store.notifyInfo('链接已复制')
    setTimeout(() => { copied.value = false }, 2000)
  } catch {
    try {
      copyWithTextarea(shareLink.value)
      copied.value = true
      store.notifyInfo('链接已复制')
      setTimeout(() => { copied.value = false }, 2000)
    } catch {
      store.notifyError('复制失败，请手动复制链接')
    }
  }
}

function copyWithTextarea(text: string) {
  const input = document.createElement('textarea')
  input.value = text
  input.style.position = 'fixed'
  input.style.opacity = '0'
  document.body.appendChild(input)
  input.select()
  const ok = document.execCommand('copy')
  document.body.removeChild(input)
  if (!ok) throw new Error('copy failed')
}
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="store.shareTarget" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="absolute inset-0 bg-black/50" @click="store.closeShare()" />
        <div class="relative z-10 w-[460px] overflow-hidden rounded-xl bg-white shadow-2xl dark:bg-gray-900">
          <div class="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-800">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">分享</h3>
            <button class="rounded-lg p-2 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300" @click="store.closeShare()">
              <X class="size-5" />
            </button>
          </div>
          <div class="p-6">
            <h4 class="text-base font-semibold text-gray-900 dark:text-white">{{ store.shareTarget.title }}</h4>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ truncatedDesc }}</p>

            <div class="mt-4">
              <input :value="store.isShareCreating ? '正在生成分享链接...' : shareLink" readonly class="w-full rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-700 outline-none dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300" />
              <p v-if="store.shareError" class="mt-2 text-xs text-red-500">{{ store.shareError }}</p>
            </div>

            <div class="mt-3 flex items-center gap-2 text-xs">
              <span class="text-gray-500 dark:text-gray-400">当前使用密钥：</span>
              <span class="max-w-44 truncate text-gray-600 dark:text-gray-400">
                {{
                  store.generalSettings.shareSecurityKey
                    ? showShareKey ? store.generalSettings.shareSecurityKey : '••••••••'
                    : '未设置'
                }}
              </span>
              <button
                v-if="store.generalSettings.shareSecurityKey"
                class="rounded p-1 text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
                title="显示或隐藏密钥"
                @click="showShareKey = !showShareKey"
              >
                <EyeOff v-if="showShareKey" class="size-4" />
                <Eye v-else class="size-4" />
              </button>
              <button class="ml-auto text-gray-400 underline underline-offset-4 hover:text-gray-700 dark:hover:text-gray-200" @click="store.openShareSecuritySettings()">前往设置</button>
            </div>

            <div class="mt-6 flex gap-3">
              <button class="flex-1 rounded-lg border border-gray-200 px-4 py-2.5 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800" @click="store.closeShare()">取消</button>
              <button class="flex flex-1 items-center justify-center gap-1.5 rounded-lg bg-gray-900 px-4 py-2.5 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200" :disabled="!shareLink || store.isShareCreating" @click="copyLink">
                <Copy class="size-4" />
                {{ copied ? '已复制' : '复制链接' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
