<script setup lang="ts">
import { ref } from 'vue'
import { Check, Monitor, Sun, Moon, ChevronDown } from 'lucide-vue-next'
import { useAppStore } from '@/stores/app'
import { LANGUAGE_OPTIONS } from '@/constants'

const store = useAppStore()

const langOpen = ref(false)

function setTheme(mode: 'system' | 'light' | 'dark') {
  store.personalizationSettings.themeMode = mode
}
</script>

<template>
  <div class="space-y-8">
    <div>
      <h4 class="font-medium text-gray-900 dark:text-white">主题模式</h4>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">选择适合你的界面主题</p>
      <div class="mt-4 grid grid-cols-3 gap-4">
        <button
          class="relative flex flex-col items-center gap-2 rounded-lg border px-4 py-3 transition-colors"
          title="跟随系统会读取操作系统的深色/浅色偏好；同时结合本地时间，19:00-07:00 自动使用深色模式。"
          :class="store.personalizationSettings.themeMode === 'system'
            ? 'border-gray-900 bg-white text-gray-900 dark:border-white dark:bg-gray-800 dark:text-white'
            : 'border-gray-200 bg-gray-50 text-gray-600 hover:bg-white dark:border-gray-700 dark:bg-gray-800/60 dark:text-gray-400 dark:hover:bg-gray-800'"
          @click="setTheme('system')"
        >
          <Monitor class="size-6 text-gray-600 dark:text-gray-400" />
          <span class="text-sm text-gray-700 dark:text-gray-300">跟随系统</span>
          <Check v-if="store.personalizationSettings.themeMode === 'system'" class="absolute right-2 top-2 size-4" />
        </button>
        <button
          class="relative flex flex-col items-center gap-2 rounded-lg border px-4 py-3 transition-colors"
          :class="store.personalizationSettings.themeMode === 'light'
            ? 'border-gray-900 bg-white text-gray-900 dark:border-white dark:bg-gray-800 dark:text-white'
            : 'border-gray-200 bg-gray-50 text-gray-600 hover:bg-white dark:border-gray-700 dark:bg-gray-800/60 dark:text-gray-400 dark:hover:bg-gray-800'"
          @click="setTheme('light')"
        >
          <Sun class="size-6 text-gray-600 dark:text-gray-400" />
          <span class="text-sm text-gray-700 dark:text-gray-300">浅色模式</span>
          <Check v-if="store.personalizationSettings.themeMode === 'light'" class="absolute right-2 top-2 size-4" />
        </button>
        <button
          class="relative flex flex-col items-center gap-2 rounded-lg border px-4 py-3 transition-colors"
          :class="store.personalizationSettings.themeMode === 'dark'
            ? 'border-gray-900 bg-white text-gray-900 dark:border-white dark:bg-gray-800 dark:text-white'
            : 'border-gray-200 bg-gray-50 text-gray-600 hover:bg-white dark:border-gray-700 dark:bg-gray-800/60 dark:text-gray-400 dark:hover:bg-gray-800'"
          @click="setTheme('dark')"
        >
          <Moon class="size-6 text-gray-600 dark:text-gray-400" />
          <span class="text-sm text-gray-700 dark:text-gray-300">深色模式</span>
          <Check v-if="store.personalizationSettings.themeMode === 'dark'" class="absolute right-2 top-2 size-4" />
        </button>
      </div>
    </div>

    <div class="flex items-start justify-between">
      <div>
        <h4 class="font-medium text-gray-900 dark:text-white">界面语言</h4>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">选择应用显示语言</p>
      </div>
      <div class="relative">
        <button
          class="flex w-28 items-center justify-between rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
          @click="langOpen = !langOpen"
        >
          {{ store.personalizationSettings.language }}
          <ChevronDown class="size-4 text-gray-400" />
        </button>
        <div
          v-if="langOpen"
          class="absolute right-0 z-10 mt-1 w-28 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
        >
          <button
            v-for="option in LANGUAGE_OPTIONS"
            :key="option"
            class="w-full px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
            :class="store.personalizationSettings.language === option ? 'bg-gray-100 dark:bg-gray-700' : ''"
            @click="store.personalizationSettings.language = option; langOpen = false"
          >
            {{ option }}
          </button>
        </div>
      </div>
    </div>

  </div>
</template>
