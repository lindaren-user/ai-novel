<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import NovelList from '@/components/novel/NovelList.vue'
import VolumeList from '@/components/novel/VolumeList.vue'
import ChatArea from '@/components/chat/ChatArea.vue'
import SettingsModal from '@/components/settings/SettingsModal.vue'
import NovelOverviewModal from '@/components/novel/NovelOverviewModal.vue'
import ArchiveConfirmModal from '@/components/novel/ArchiveConfirmModal.vue'
import VolumeChapterOverviewModal from '@/components/novel/VolumeChapterOverviewModal.vue'
import ShareModal from '@/components/novel/ShareModal.vue'
import AuthModal from '@/components/auth/AuthModal.vue'

const store = useAppStore()
const router = useRouter()

const shouldLoadDashboard = computed(() => (
  store.viewMode === 'chat' &&
  !store.isNovelSetupOpen &&
  !store.selectedNovelId &&
  store.isNovelSetupChoiceOpen
))

watch(
  [shouldLoadDashboard, () => store.authToken],
  ([visible, token]) => {
    if (visible && token) void store.loadDashboard()
  },
  { immediate: true },
)

async function confirmLogout() {
  await store.confirmLogout()
  await router.push('/')
}
</script>

<template>
  <div class="flex h-screen bg-white text-gray-900 dark:bg-gray-950 dark:text-white">
    <NovelList />
    <VolumeList />
    <ChatArea />

    <SettingsModal />
    <NovelOverviewModal />
    <ArchiveConfirmModal />
    <VolumeChapterOverviewModal />
    <ShareModal />
    <AuthModal
      :open="store.isAuthModalOpen"
      :mode="store.authModalMode"
      @close="store.closeAuthModal()"
      @update:mode="store.setAuthModalMode($event)"
    />
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="store.isLogoutConfirmOpen" class="fixed inset-0 z-[80] flex items-center justify-center">
          <div class="absolute inset-0 bg-black/50" @click="store.cancelLogout()" />
          <div class="relative z-10 w-[360px] rounded-xl bg-white p-6 shadow-2xl dark:bg-gray-900">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">确定退出登录吗？</h3>
            <div class="mt-6 flex justify-end gap-2">
              <button class="rounded-lg border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800" @click="store.cancelLogout()">取消</button>
              <button class="rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200" @click="confirmLogout">确定退出</button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
