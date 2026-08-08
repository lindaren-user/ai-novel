<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { BookOpen, Check, ChevronDown, ChevronRight, Lock, PanelLeftClose, PanelLeftOpen } from 'lucide-vue-next'
import { getSharedContentApi, type ApiSharedContent, type ApiSharedChapter } from '@/services/api'
import BackToTopButton from '@/components/common/BackToTopButton.vue'
import aiNovelLogo from '@/assets/img/ai-novel.webp'

const route = useRoute()
const router = useRouter()

const isLoading = ref(false)
const error = ref('')
const password = ref(typeof route.query.pwd === 'string' ? route.query.pwd : '')
const shared = ref<ApiSharedContent | null>(null)
const selectedVolumeId = ref<number | null>(null)
const selectedChapterId = ref<number | null>(null)
const isSidebarExpanded = ref(true)
const expandedVolumeIds = ref<number[]>([])
const mainRef = ref<HTMLElement | null>(null)

const volumes = computed(() => shared.value?.novel.volumes || [])
const selectedVolume = computed(() => volumes.value.find((volume) => volume.id === selectedVolumeId.value) || volumes.value[0])
const chapters = computed(() => selectedVolume.value?.chapters || [])
const selectedChapter = computed<ApiSharedChapter | undefined>(() => chapters.value.find((chapter) => chapter.id === selectedChapterId.value) || chapters.value[0])
const shouldShowSidebar = computed(() => !!shared.value && shared.value.type !== 'chapter')

async function loadSharedContent() {
  const type = String(route.params.type || '')
  const token = String(route.params.token || '')
  isLoading.value = true
  error.value = ''
  try {
    const response = await getSharedContentApi(type, token, password.value)
    shared.value = response
    selectedVolumeId.value = response.selectedVolumeId || response.novel.volumes[0]?.id || null
    selectedChapterId.value = response.selectedChapterId || response.novel.volumes.find((volume) => volume.id === selectedVolumeId.value)?.chapters[0]?.id || null
    expandedVolumeIds.value = selectedVolumeId.value ? [selectedVolumeId.value] : []
  } catch (err) {
    shared.value = null
    error.value = err instanceof Error ? err.message : '读取分享内容失败'
  } finally {
    isLoading.value = false
  }
}

// isVolumeExpanded 判断分享页卷节点是否展开。
function isVolumeExpanded(volumeId: number) {
  return expandedVolumeIds.value.includes(volumeId)
}

// toggleVolume 展开或收起分享页卷节点。
function toggleVolume(volumeId: number) {
  expandedVolumeIds.value = isVolumeExpanded(volumeId)
    ? expandedVolumeIds.value.filter((id) => id !== volumeId)
    : [...expandedVolumeIds.value, volumeId]
}

// selectChapter 切换当前阅读章节，并同步选中卷。
function selectChapter(volumeId: number, chapterId: number) {
  selectedVolumeId.value = volumeId
  selectedChapterId.value = chapterId
  if (!isVolumeExpanded(volumeId)) {
    expandedVolumeIds.value = [...expandedVolumeIds.value, volumeId]
  }
}

// chapterDone 判断章节是否已有正文。
function chapterDone(chapter: ApiSharedChapter) {
  return !!chapter.content || !!chapter.completedAt
}

// volumeDone 判断卷下章节是否都已有正文。
function volumeDone(volume: { chapters: ApiSharedChapter[] }) {
  return volume.chapters.length > 0 && volume.chapters.every(chapterDone)
}

// toggleSidebar 切换分享阅读侧边栏展开状态。
function toggleSidebar() {
  isSidebarExpanded.value = !isSidebarExpanded.value
}

// currentVolumeChapterCount 统计当前选中卷章节数。
function currentVolumeChapterCount(volumeId: number) {
  return volumes.value.find((volume) => volume.id === volumeId)?.chapters.length || 0
}

function planTitle(item: { planData?: Record<string, unknown>; sortOrder?: number }, fallback: string) {
  const title = typeof item.planData?.title === 'string' ? item.planData.title.trim() : ''
  return title || (item.sortOrder ? `${fallback}${item.sortOrder}` : fallback)
}

function volumeTitle(volume: { planData?: Record<string, unknown>; sortOrder?: number }) {
  return planTitle(volume, '未命名卷')
}

function chapterTitle(chapter: { planData?: Record<string, unknown>; sortOrder?: number }) {
  return planTitle(chapter, '未命名章节')
}

function selectVolume(volumeId: number) {
  selectedVolumeId.value = volumeId
  selectedChapterId.value = volumes.value.find((volume) => volume.id === volumeId)?.chapters[0]?.id || null
  toggleVolume(volumeId)
}

function submitPassword() {
  router.replace({ query: password.value ? { pwd: password.value } : {} })
  void loadSharedContent()
}

onMounted(loadSharedContent)
</script>

<template>
  <div class="flex h-screen bg-white text-gray-900 dark:bg-gray-950 dark:text-white">
    <aside
      v-if="shouldShowSidebar && isSidebarExpanded"
      class="fixed left-0 top-0 z-40 h-screen w-72 border-r border-gray-200 bg-gray-50 shadow-xl dark:border-gray-800 dark:bg-gray-950"
    >
      <div class="flex h-full flex-col">
        <div class="flex h-14 items-center gap-2 border-b border-gray-200 px-3 dark:border-gray-800">
          <BookOpen class="size-5 shrink-0" />
          <h1 class="min-w-0 flex-1 truncate text-base font-semibold">{{ shared?.novel.title || '分享阅读' }}</h1>
          <button
            class="rounded-lg p-1.5 text-gray-500 hover:bg-gray-200 dark:text-gray-400 dark:hover:bg-gray-800"
            title="收起侧边栏"
            @click="toggleSidebar"
          >
            <PanelLeftClose class="size-4" />
          </button>
        </div>

        <div v-if="shared" class="flex-1 overflow-y-auto p-2">
          <div class="space-y-1">
            <div
              v-for="volume in volumes"
              :key="volume.id"
              class="relative"
            >
              <div
                class="group flex w-full cursor-pointer items-center gap-2 rounded-lg px-3 py-2.5"
                :class="selectedVolumeId === volume.id ? 'bg-gray-100 dark:bg-gray-800' : 'hover:bg-gray-100 dark:hover:bg-gray-800/50'"
                @click="selectVolume(volume.id)"
              >
                <component
                  :is="isVolumeExpanded(volume.id) ? ChevronDown : ChevronRight"
                  class="size-4 shrink-0 text-gray-400 dark:text-gray-500"
                />
                <span class="min-w-0 flex-1 truncate text-sm text-gray-900 dark:text-white" :title="volumeTitle(volume)">
                  {{ volumeTitle(volume) }}
                </span>
                <span class="shrink-0 text-xs text-gray-500 dark:text-gray-400">
                  {{ currentVolumeChapterCount(volume.id) }} 章
                </span>
                <Check v-if="volumeDone(volume)" class="size-3.5 shrink-0 text-green-500" />
              </div>

              <div
                v-if="isVolumeExpanded(volume.id) && volume.chapters.length > 0"
                class="ml-4 space-y-0.5 border-l border-gray-200 pl-4 dark:border-gray-700"
              >
                <button
                  v-for="chapter in volume.chapters"
                  :key="chapter.id"
                  class="flex w-full items-center gap-1.5 rounded-lg px-3 py-2 text-left text-sm"
                  :class="selectedChapterId === chapter.id
                    ? 'bg-gray-100 text-gray-900 dark:bg-gray-800 dark:text-white'
                    : 'text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800/50'"
                  :title="chapterTitle(chapter)"
                  @click="selectChapter(volume.id, chapter.id)"
                >
                  <span class="min-w-0 flex-1 truncate">{{ chapterTitle(chapter) }}</span>
                  <Check v-if="chapterDone(chapter)" class="size-3.5 shrink-0 text-green-500" />
                </button>
              </div>

              <div
                v-if="!isVolumeExpanded(volume.id) && volume.chapters.length > 0"
                class="ml-8 py-1 text-xs text-gray-400 dark:text-gray-500"
              >
                ...
              </div>
            </div>
          </div>
        </div>
      </div>
    </aside>
    <button
      v-if="shouldShowSidebar && !isSidebarExpanded"
      class="fixed left-3 top-3 z-40 flex size-10 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-600 shadow-lg hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-200 dark:hover:bg-gray-800"
      title="展开侧边栏"
      @click="toggleSidebar"
    >
      <PanelLeftOpen class="size-5" />
    </button>

    <main ref="mainRef" class="flex min-w-0 flex-1 flex-col overflow-y-auto">
      <div v-if="isLoading" class="flex flex-1 items-center justify-center text-sm text-gray-500">正在加载分享内容...</div>

      <div v-else-if="!shared" class="flex flex-1 items-center justify-center px-6">
        <div class="w-full max-w-sm rounded-xl border border-gray-200 bg-white p-6 text-center shadow-sm dark:border-gray-800 dark:bg-gray-900">
          <Lock class="mx-auto size-8 text-gray-400" />
          <h2 class="mt-3 text-lg font-semibold">需要分享密钥</h2>
          <p class="mt-2 text-sm" :class="error ? 'text-red-500' : 'text-gray-500'">{{ error || '请输入分享链接密钥继续阅读。' }}</p>
          <input
            v-model="password"
            class="mt-4 w-full rounded-lg border border-gray-200 px-3 py-2 text-sm outline-none dark:border-gray-700 dark:bg-gray-950"
            placeholder="输入提取密钥"
            @keydown.enter="submitPassword"
          />
          <button class="mt-3 w-full rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white dark:bg-gray-100 dark:text-gray-900" @click="submitPassword">
            进入阅读
          </button>
        </div>
      </div>

      <article v-else class="mx-auto w-full max-w-3xl px-8 py-10">
        <div class="mb-8 border-b border-gray-200 pb-5 dark:border-gray-800">
          <div class="flex items-start justify-between gap-6">
            <div class="min-w-0 flex-1">
              <p class="text-sm text-gray-500">{{ selectedVolume ? volumeTitle(selectedVolume) : '' }}</p>
              <h2 class="mt-2 text-3xl font-bold">{{ selectedChapter ? chapterTitle(selectedChapter) : '暂无章节' }}</h2>
            </div>
            <img
              :src="aiNovelLogo"
              alt="AI Novel"
              class="hidden h-16 w-auto shrink-0 object-contain sm:block dark:invert"
            />
          </div>
        </div>
        <div v-if="selectedChapter?.content" class="whitespace-pre-wrap text-lg leading-9 text-gray-800 dark:text-gray-200">{{ selectedChapter.content }}</div>
        <div v-else class="rounded-lg border border-dashed border-gray-200 p-6 text-center text-sm text-gray-500 dark:border-gray-800">
          本章正文尚未生成。
        </div>
      </article>
      <BackToTopButton :target="mainRef" />
    </main>
  </div>
</template>
