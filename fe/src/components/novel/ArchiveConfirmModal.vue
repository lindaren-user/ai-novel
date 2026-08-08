<script setup lang="ts">
import { X } from 'lucide-vue-next'
import { computed } from 'vue'
import { useAppStore } from '@/stores/app'

const store = useAppStore()

const novel = computed(() =>
  store.archiveTargetId
    ? store.novels.find((n) => n.id === store.archiveTargetId)
    : null
)
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="store.archiveTargetId"
        class="fixed inset-0 z-50 flex items-center justify-center"
      >
        <div class="absolute inset-0 bg-black/50" @click="store.closeArchiveConfirm()" />
        <div class="relative z-10 w-[400px] overflow-hidden rounded-xl bg-white shadow-2xl dark:bg-gray-900">
          <div class="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-800">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">确认归档</h3>
            <button
              class="rounded-lg p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300"
              @click="store.closeArchiveConfirm()"
            >
              <X class="size-5" />
            </button>
          </div>
          <div class="p-6">
            <p class="text-sm text-gray-700 dark:text-gray-300">
              确定要归档 <span class="font-semibold text-gray-900 dark:text-white">《{{ novel?.title }}》</span> 吗？
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">归档后可在回收站查看和恢复</p>
            <div class="mt-6 flex gap-3">
              <button
                class="flex-1 rounded-lg border border-gray-200 px-4 py-2.5 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
                @click="store.closeArchiveConfirm()"
              >
                取消
              </button>
              <button
                class="flex-1 rounded-lg bg-gray-900 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
                @click="store.confirmArchive()"
              >
                确认归档
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
