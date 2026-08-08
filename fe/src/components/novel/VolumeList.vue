<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { ChevronDown, ChevronRight, ChevronsLeft, ChevronsRight, MoreHorizontal, Loader2, Eye, Pencil, CheckCircle2, Download, Share2 } from 'lucide-vue-next'
import { useAppStore } from '@/stores/app'
import { overviewSummary } from '@/utils/overview'
import emptyVolumeImage from '@/assets/img/volume.webp'

const store = useAppStore()

const volumeMenuId = ref<number | null>(null)
const chapterMenuId = ref<number | null>(null)

function toggleVolumeMenu(id: number, e: Event) {
  e.stopPropagation()
  volumeMenuId.value = volumeMenuId.value === id ? null : id
}

function toggleChapterMenu(id: number, e: Event) {
  e.stopPropagation()
  chapterMenuId.value = chapterMenuId.value === id ? null : id
}

function closeMenus() {
  volumeMenuId.value = null
  chapterMenuId.value = null
}

function onDocumentClick(e: MouseEvent) {
  if (!volumeMenuId.value && !chapterMenuId.value) return
  const target = e.target as HTMLElement
  if (!target.closest('.context-menu-trigger') && !target.closest('.context-menu-dropdown')) {
    closeMenus()
  }
}

function handleDownloadVolume(volumeId: number) {
  volumeMenuId.value = null
  void store.startDownload('volume', volumeId)
}

function handleShareVolume(volumeId: number) {
  volumeMenuId.value = null
  const volume = store.selectedNovel?.volumes.find((v) => v.id === volumeId)
  if (volume) {
    store.openShare('volume', volume.id, store.volumeTitle(volume), overviewSummary(volume.planData) || '卷梗概尚未生成')
  }
}

function handleShareChapter(chapterId: number) {
  chapterMenuId.value = null
  const chapter = store.selectedNovel?.volumes.flatMap((v) => v.chapters).find((c) => c.id === chapterId)
  if (chapter) {
    const snippet = (chapter.content || '').replace(/\s/g, '')
    const desc = snippet.length > 0 ? snippet.slice(0, 100) : '章节内容尚未生成'
    store.openShare('chapter', chapter.id, store.chapterTitle(chapter), desc)
  }
}

function handleDownloadChapter(chapterId: number) {
  chapterMenuId.value = null
  void store.startDownload('chapter', chapterId)
}

function isChapterDone(chapter: { content?: string; status: number; completedAt?: Date | null }) {
  return Boolean(chapter.completedAt || chapter.content || chapter.status === 2)
}

onMounted(() => document.addEventListener('click', onDocumentClick))
onUnmounted(() => document.removeEventListener('click', onDocumentClick))
</script>

<template>
  <div
    class="flex shrink-0 flex-col border-gray-200 bg-white transition-all dark:border-gray-800 dark:bg-gray-950"
    :class="store.isVolumesCollapsed ? 'h-screen w-11 border-r' : 'h-screen w-[240px] border-r'"
  >
    <!-- Expand button when collapsed -->
    <div v-if="store.isVolumesCollapsed" class="flex h-full items-start justify-center pt-4">
      <button
        class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40 dark:text-gray-400 dark:hover:bg-gray-800"
        :disabled="!store.selectedNovel || store.isNovelSetupOpen || store.isNovelSetupChoiceOpen"
        @click="store.setVolumesCollapsed(false)"
      >
        <ChevronsRight class="size-6" />
      </button>
    </div>

    <template v-if="!store.isVolumesCollapsed">
    <!-- Header -->
    <div class="flex h-16 items-center justify-between border-b border-gray-200 px-4 dark:border-gray-800">
      <h2 class="truncate font-medium text-gray-900 dark:text-white">
        {{ store.selectedNovel?.title || '选择小说' }}
      </h2>
      <div class="flex items-center gap-1">
        <button
          class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40 dark:text-gray-400 dark:hover:bg-gray-800"
          :disabled="store.isNovelSetupOpen || store.isNovelSetupChoiceOpen"
          @click="store.setVolumesCollapsed(true)"
        >
          <ChevronsLeft class="size-5" />
        </button>
      </div>
    </div>

    <!-- Empty state -->
    <div
      v-if="store.selectedNovel && store.isVolumesLoading"
      class="flex flex-1 flex-col items-center justify-center text-gray-400 dark:text-gray-500"
    >
      <p class="text-sm">正在加载卷列表...</p>
    </div>

    <div
      v-else-if="!store.selectedNovel || store.selectedNovel.volumes.length === 0"
      class="flex flex-1 flex-col items-center justify-center pb-20 text-gray-400 dark:text-gray-500"
    >
      <img
        :src="emptyVolumeImage"
        alt=""
        class="h-24 w-32 object-contain opacity-80 dark:opacity-60 dark:contrast-90"
      />
      <p class="mt-2 text-sm">{{ store.selectedNovel ? '暂无卷' : '选择一部小说' }}</p>
      <p class="mt-1 text-xs text-gray-400">{{ store.selectedNovel ? '在小说对话中规划卷结构' : '点击左侧小说开始创作' }}</p>
    </div>

    <!-- Volume Tree -->
    <div v-else class="flex-1 overflow-y-auto">
      <div class="space-y-1 p-2">
        <div
          v-for="(volume, volumeIndex) in store.selectedNovel?.volumes"
          :key="volume.id"
          class="group/volume relative"
        >
          <!-- Volume Header -->
          <div
            class="group flex w-full min-w-0 cursor-pointer items-center gap-2 rounded-lg px-3 py-2.5"
            :class="store.selectedVolumeId === volume.id ? 'bg-gray-100 dark:bg-gray-800' : 'hover:bg-gray-50 dark:hover:bg-gray-800/50'"
          >
            <div class="flex min-w-0 flex-1 items-center gap-2 text-left">
              <button
                class="rounded p-0.5 text-gray-400 hover:bg-gray-100 dark:text-gray-500 dark:hover:bg-gray-800"
                type="button"
                title="展开章节"
                @mousedown.stop
                @click.stop.prevent="store.toggleVolumeExpanded(volume.id)"
              >
                <component
                  :is="volume.expanded ? ChevronDown : ChevronRight"
                  class="size-4 shrink-0"
                />
              </button>
              <button
                class="flex min-w-0 flex-1 cursor-pointer items-center gap-2 text-left"
                type="button"
                @click="store.selectVolume(volume.id)"
              >
              <span class="min-w-0 flex-1 truncate text-sm text-gray-900 dark:text-white" :title="store.volumeTitle(volume)">
                第{{ volumeIndex + 1 }}卷：{{ store.volumeTitle(volume) }}
              </span>
              <span v-if="!store.isSessionStreaming('volume', volume.id)" class="shrink-0 text-xs text-gray-500 dark:text-gray-400">
                {{ volume.chapterCount }} 章
              </span>
              <CheckCircle2 v-if="volume.chapters.length > 0 && volume.chapters.every(isChapterDone)" class="size-3.5 shrink-0 text-green-500" />
              </button>
            </div>
            <Loader2
              v-if="store.isSessionStreaming('volume', volume.id)"
              class="size-4 shrink-0 animate-spin text-gray-400"
              title="AI 正在回复"
            />
            <div class="relative w-6 shrink-0">
              <button
                class="context-menu-trigger rounded p-1 text-gray-400 opacity-0 transition-opacity group-hover:opacity-100 hover:bg-gray-200 dark:text-gray-500 dark:hover:bg-gray-700"
                @click="toggleVolumeMenu(volume.id, $event)"
              >
                <MoreHorizontal class="size-4" />
              </button>
              <div
                v-if="volumeMenuId === volume.id"
                class="context-menu-dropdown absolute right-0 top-full z-20 mt-1 w-28 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
                @click.stop
              >
                <button
                  class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                  @click="volumeMenuId = null; store.openVolumeChapterOverview('volume', volume.id, store.volumeTitle(volume), volume.createdAt)"
                >
                  <Eye class="size-4" />
                  查看
                </button>
                <button
                  class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                  @click="handleDownloadVolume(volume.id)"
                >
                  <Download class="size-4" />
                  下载
                </button>
                <button
                  class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                  @click="handleShareVolume(volume.id)"
                >
                  <Share2 class="size-4" />
                  分享
                </button>
              </div>
            </div>
          </div>
          <!-- Chapters -->
          <div
            v-if="volume.expanded && store.isChaptersLoading && volume.chapters.length === 0"
            class="ml-8 py-2 text-xs text-gray-400 dark:text-gray-500"
          >
            正在加载章节...
          </div>

          <div
            v-else-if="volume.expanded && volume.chapters.length > 0"
            class="ml-4 space-y-0.5 border-l border-gray-200 pl-4 dark:border-gray-700"
          >
            <div
              v-for="(chapter, chapterIndex) in volume.chapters"
              :key="chapter.id"
              class="group flex items-center rounded-lg"
              :class="store.selectedChapterId === chapter.id
                ? 'bg-gray-100 text-gray-900 dark:bg-gray-800 dark:text-white'
                : 'text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-gray-800/50'"
            >
              <div
                class="flex min-w-0 flex-1 cursor-pointer items-center gap-1.5 px-3 py-2 text-left text-sm"
                @click="store.selectChapter(chapter.id)"
                :title="store.chapterTitle(chapter)"
              >
                <span class="min-w-0 flex-1 truncate">第{{ chapterIndex + 1 }}章：{{ store.chapterTitle(chapter) }}</span>
                <CheckCircle2 v-if="isChapterDone(chapter)" class="size-3.5 shrink-0 text-green-500" />
              </div>
              <Loader2
                v-if="store.isSessionStreaming('chapter', chapter.id)"
                class="size-3.5 shrink-0 animate-spin text-gray-400"
                title="AI 正在回复"
              />
              <div class="relative w-5 shrink-0">
                <button
                  class="context-menu-trigger rounded p-1 text-gray-400 opacity-0 transition-opacity group-hover:opacity-100 hover:bg-gray-200 dark:text-gray-500 dark:hover:bg-gray-700"
                  @click="toggleChapterMenu(chapter.id, $event)"
                >
                  <MoreHorizontal class="size-3.5" />
                </button>
                <div
                  v-if="chapterMenuId === chapter.id"
                  class="context-menu-dropdown absolute right-0 top-full z-20 mt-1 w-28 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
                  @click.stop
                >
                  <button
                    class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    @click="chapterMenuId = null; store.openEditorMode(chapter.id)"
                  >
                    <Pencil class="size-4" />
                    编辑
                  </button>
                  <button
                    class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    @click="chapterMenuId = null; store.openVolumeChapterOverview('chapter', chapter.id, store.chapterTitle(chapter), chapter.createdAt)"
                  >
                    <Eye class="size-4" />
                    查看
                  </button>
                  <button
                    class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    @click="handleDownloadChapter(chapter.id)"
                  >
                    <Download class="size-4" />
                    下载
                  </button>
                  <button
                    class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    @click="handleShareChapter(chapter.id)"
                  >
                    <Share2 class="size-4" />
                    分享
                  </button>
                </div>
              </div>
            </div>
          </div>

        </div>
      </div>
    </div>
    </template>
  </div>
</template>
