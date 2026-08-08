<script setup lang="ts">
import { X, Settings, Bell, Palette, User } from 'lucide-vue-next'
import { useAppStore } from '@/stores/app'
import type { SettingsTab } from '@/types'
import SettingsGeneral from './SettingsGeneral.vue'
import SettingsNotification from './SettingsNotification.vue'
import SettingsPersonalization from './SettingsPersonalization.vue'
import SettingsAccount from './SettingsAccount.vue'

const store = useAppStore()

interface TabItem {
  id: SettingsTab
  label: string
  icon: typeof Settings
}

const tabs: TabItem[] = [
  { id: 'general', label: '常规', icon: Settings },
  { id: 'notification', label: '通知', icon: Bell },
  { id: 'personalization', label: '个性化', icon: Palette },
  { id: 'account', label: '账户', icon: User },
]

const tabTitles: Record<SettingsTab, string> = {
  general: '常规',
  notification: '通知',
  personalization: '个性化',
  account: '账户',
}
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="store.isSettingsOpen"
        class="fixed inset-0 z-[70] flex items-center justify-center"
      >
        <div
          class="absolute inset-0 bg-black/50"
          @click="store.closeSettings"
        />

        <div class="relative z-10 flex h-[600px] w-[720px] overflow-hidden rounded-xl bg-white shadow-2xl dark:bg-gray-900">
          <div class="w-[180px] border-r border-gray-200 bg-gray-50 p-4 dark:border-gray-800 dark:bg-gray-950">
            <h2 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">设置</h2>
            <nav class="space-y-1">
              <button
                v-for="tab in tabs"
                :key="tab.id"
                class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors"
                :class="store.settingsTab === tab.id
                  ? 'bg-white text-gray-900 shadow-sm dark:bg-gray-800 dark:text-white'
                  : 'text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800'"
                @click="store.settingsTab = tab.id"
              >
                <component :is="tab.icon" class="size-5" />
                {{ tab.label }}
              </button>
            </nav>
          </div>

          <div class="flex flex-1 flex-col">
            <div class="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-800">
              <div>
                <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ tabTitles[store.settingsTab] }}
                </h3>
                <p
                  v-if="store.isLoggedIn"
                  class="mt-0.5 text-xs"
                  :class="store.settingsError ? 'text-red-500' : 'text-gray-400 dark:text-gray-500'"
                >
                  <span v-if="store.isSettingsLoading">正在加载设置...</span>
                  <span v-else-if="store.isSettingsSaving">正在保存...</span>
                  <span v-else-if="store.settingsError">{{ store.settingsError }}</span>
                </p>
              </div>
              <div class="flex items-center gap-2">
                <button
                  v-if="store.isLoggedIn && store.settingsError"
                  class="rounded-lg border border-gray-200 px-3 py-1.5 text-xs text-gray-600 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
                  @click="store.saveSettings()"
                >
                  重试
                </button>
                <button
                  class="rounded-lg p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300"
                  @click="store.closeSettings"
                >
                  <X class="size-5" />
                </button>
              </div>
            </div>

            <div class="flex-1 overflow-y-auto p-6">
              <SettingsGeneral v-if="store.settingsTab === 'general'" />
              <SettingsNotification v-else-if="store.settingsTab === 'notification'" />
              <SettingsPersonalization v-else-if="store.settingsTab === 'personalization'" />
              <SettingsAccount v-else-if="store.settingsTab === 'account'" />
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
