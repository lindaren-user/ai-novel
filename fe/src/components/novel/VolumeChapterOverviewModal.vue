<script setup lang="ts">
import { ChevronRight, Info, MapPinned, Puzzle, Target, Users, X } from 'lucide-vue-next'
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import { computed, ref, watch } from 'vue'
import { useAppStore } from '@/stores/app'
import { overviewDetails, overviewSummary } from '@/utils/overview'

const store = useAppStore()
const markdown = new MarkdownIt({ html: false, linkify: true, breaks: true })
const selectedTemporarySetupSection = ref<SetupListSection | null>(null)
const selectedTemporarySetupItem = ref<SetupListItem | null>(null)

const target = computed(() => store.overviewVolumeChapter)

const wordCount = computed(() => {
  if (!target.value) return 0
  const novel = store.selectedNovel
  if (!novel) return 0
  if (target.value.type === 'volume') {
    const vol = novel.volumes.find((v) => v.id === target.value!.id)
    return vol?.wordCount || 0
  }
  const ch = novel.volumes.flatMap((v) => v.chapters).find((c) => c.id === target.value!.id)
  return ch?.wordCount || 0
})

const currentVolume = computed(() => {
  if (!target.value || target.value.type !== 'volume') return null
  return store.selectedNovel?.volumes.find((v) => v.id === target.value!.id) || null
})

const currentChapter = computed(() => {
  if (!target.value || target.value.type !== 'chapter') return null
  return store.selectedNovel?.volumes.flatMap((v) => v.chapters).find((c) => c.id === target.value!.id) || null
})

const chapterOverviewDetails = computed(() => {
  const planData = currentChapter.value?.planData
  return planData ? overviewDetails(planData) : []
})

const volumeOverviewDetails = computed(() => {
  const planData = currentVolume.value?.planData
  if (!planData) return []
  return overviewDetails(planData).filter((detail) => detail.key !== 'temporary_settings')
})

const temporarySetupSections = computed(() => {
  const temporarySettings = currentVolume.value?.planData?.temporary_settings
  if (!temporarySettings || typeof temporarySettings !== 'object') return []
  const raw = temporarySettings as Record<string, unknown>
  return [
    namedSection('characters', '人物设定', raw.characters),
    namedSection('maps', '地点设定', raw.maps),
    namedSection('forces', '势力设定', raw.forces),
    otherSettingsSection(raw.other_settings),
  ].filter((section): section is SetupListSection => !!section && section.items.length > 0)
})

function detailListItems(value: string | string[]) {
  return Array.isArray(value) ? value : [value]
}

function isLongDetailItem(value: string) {
  return value.length > 32 || /[\r\n]/.test(value)
}

function detailCardClass(value: string, compactTextClass = '') {
  const base = 'chat-markdown max-w-full rounded-md bg-gray-50 dark:bg-gray-800/70'
  if (isLongDetailItem(value)) {
    return `${base} flex w-full px-3 py-3 leading-6 ${compactTextClass}`.trim()
  }
  return `${base} inline-flex px-2.5 py-1.5 ${compactTextClass}`.trim()
}

function renderMarkdown(content: string): string {
  return DOMPurify.sanitize(markdown.render(content || ''))
}

type SetupListItem = {
  title: string
  appearanceTime: string
  description: string
  children: SetupListItem[]
}

type SetupListSection = {
  key: string
  label: string
  items: SetupListItem[]
}

function setupListItem(title: string, description: string, appearanceTime = '', children: SetupListItem[] = []): SetupListItem {
  return { title, appearanceTime, description, children }
}

function namedSection(key: string, label: string, value: unknown): SetupListSection | null {
  if (!Array.isArray(value)) return null
  const items = value
    .map((item) => {
      if (!item || typeof item !== 'object') return null
      const raw = item as Record<string, unknown>
      const title = setupString(raw.name)
      const description = setupString(raw.notes)
      const appearanceTime = setupString(raw.appearanceTime || raw.appearance_time)
      if (!title && !description) return null
      return setupListItem(title || description, title ? description : '', appearanceTime)
    })
    .filter((item): item is SetupListItem => !!item)
  return items.length > 0 ? { key, label, items } : null
}

function otherSettingsSection(value: unknown): SetupListSection | null {
  if (!Array.isArray(value)) return null
  const items = value
    .map((item) => {
      if (!item || typeof item !== 'object') return null
      const raw = item as Record<string, unknown>
      const title = setupString(raw.title)
      const description = setupString(raw.description)
      const children = namedSection('children', '', raw.items)?.items || []
      if (!title && !description && children.length === 0) return null
      return setupListItem(title || description || '未命名设定', title ? description : '', '', children)
    })
    .filter((item): item is SetupListItem => !!item)
  return items.length > 0 ? { key: 'other_settings', label: '其他设定', items } : null
}

function setupString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function sectionSubtitle(section: SetupListSection): string {
  if (section.key === 'characters') return `${section.items.length} 人 · 管理角色信息与关系`
  if (section.key === 'maps') return `${section.items.length} 处 · 管理场景与地理信息`
  if (section.key === 'forces') return `${section.items.length} 个 · 管理阵营、组织与势力关系`
  return `${section.items.length} 类 · 货币、装备、规则等自定义设定`
}

function setupItemKey(section: SetupListSection, item: SetupListItem, index: number) {
  return `${section.key}-${index}-${item.title}`
}

function setupChildKey(item: SetupListItem, child: SetupListItem, index: number) {
  return `${item.title}-${index}-${child.title}`
}

watch(target, () => {
  selectedTemporarySetupSection.value = null
  selectedTemporarySetupItem.value = null
})
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="target"
        class="fixed inset-0 z-50 flex items-center justify-center"
      >
        <div class="absolute inset-0 bg-black/50" @click="store.closeVolumeChapterOverview()" />
        <div class="relative z-10 w-[480px] overflow-hidden rounded-xl bg-white shadow-2xl dark:bg-gray-900">
          <div class="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-800">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ target.type === 'volume' ? '卷梗概' : '章梗概' }}
            </h3>
            <button
              class="rounded-lg p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300"
              @click="store.closeVolumeChapterOverview()"
            >
              <X class="size-5" />
            </button>
          </div>
          <div class="max-h-[500px] overflow-y-auto p-6">
            <h4 class="text-xl font-bold text-gray-900 dark:text-white">{{ target.title }}</h4>
            <p class="mt-1 flex justify-between text-xs text-gray-400 dark:text-gray-500"><span>创建于 {{ target.createdAt?.toLocaleString('zh-CN') || '' }}</span><span>{{ wordCount }} 字</span></p>
            <template v-if="target.type === 'volume'">
              <div v-if="currentVolume && (volumeOverviewDetails.length > 0 || temporarySetupSections.length > 0)" class="mt-4 space-y-4 text-sm text-gray-700 dark:text-gray-300">
                <div
                  v-if="overviewSummary(currentVolume.planData)"
                  :class="detailCardClass(overviewSummary(currentVolume.planData))"
                  v-html="renderMarkdown(overviewSummary(currentVolume.planData))"
                />
                <div v-if="temporarySetupSections.length > 0" class="space-y-3">
                  <h5 class="mb-1 flex items-center gap-1.5 text-sm font-bold text-gray-950 dark:text-white">
                    临时设定
                    <span
                      class="inline-flex cursor-help text-gray-400"
                      title="临时设定由卷规划按剧情需要生成，只服务当前卷；正文可以使用，但不能继续新增重要设定。"
                    >
                      <Info class="size-3.5" />
                    </span>
                  </h5>
                  <button
                    v-for="section in temporarySetupSections"
                    :key="section.key"
                    class="group flex w-full items-center gap-3 rounded-xl bg-gray-50 px-3 py-3 text-left transition-colors hover:bg-gray-100 dark:bg-gray-800/70 dark:hover:bg-gray-800"
                    @click="selectedTemporarySetupSection = section"
                  >
                    <span class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-white text-gray-500 dark:bg-gray-900 dark:text-gray-300">
                      <Users v-if="section.key === 'characters'" class="size-4" />
                      <MapPinned v-else-if="section.key === 'maps'" class="size-4" />
                      <Target v-else-if="section.key === 'forces'" class="size-4" />
                      <Puzzle v-else class="size-4" />
                    </span>
                    <span class="min-w-0 flex-1">
                      <span class="block text-base font-medium text-gray-900 dark:text-white">
                        {{ section.label }}
                      </span>
                      <span class="mt-0.5 block text-sm text-gray-500 dark:text-gray-400">
                        {{ sectionSubtitle(section) }}
                      </span>
                    </span>
                    <ChevronRight class="size-4 shrink-0 text-gray-400 transition-transform group-hover:translate-x-0.5" />
                  </button>
                </div>
                <div v-for="detail in volumeOverviewDetails" :key="detail.key">
                  <h5 class="mb-1 text-sm font-bold text-gray-950 dark:text-white">{{ detail.label }}</h5>
                  <div class="flex flex-wrap gap-1.5">
                    <span
                      v-for="item in detailListItems(detail.value)"
                      :key="`${detail.key}-${item}`"
                      :class="detailCardClass(item, 'text-sm leading-5 text-gray-700 dark:text-gray-200')"
                      v-html="renderMarkdown(item)"
                    />
                  </div>
                </div>
              </div>
              <div v-else class="mt-6 rounded-lg border border-dashed border-gray-200 p-4 text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400">
                卷梗概尚未生成。请先在小说对话中保存全书分卷规划。
              </div>
            </template>
            <template v-else>
              <div v-if="chapterOverviewDetails.length > 0" class="mt-4 space-y-4 text-sm text-gray-700 dark:text-gray-300">
                <div
                  v-if="currentChapter && overviewSummary(currentChapter.planData)"
                  :class="detailCardClass(overviewSummary(currentChapter.planData))"
                  v-html="renderMarkdown(overviewSummary(currentChapter.planData))"
                />
                <div v-for="detail in chapterOverviewDetails" :key="detail.label">
                  <h5 class="mb-1 text-sm font-bold text-gray-950 dark:text-white">{{ detail.label }}</h5>
                  <div class="flex flex-wrap gap-1.5">
                    <span
                      v-for="item in detailListItems(detail.value)"
                      :key="`${detail.key}-${item}`"
                      :class="detailCardClass(item, 'text-sm leading-5 text-gray-700 dark:text-gray-200')"
                      v-html="renderMarkdown(item)"
                    />
                  </div>
                </div>
              </div>
              <div v-else class="mt-6 rounded-lg border border-dashed border-gray-200 p-4 text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400">
                章梗概尚未生成。请先在卷对话中应用章节规划。
              </div>
            </template>
          </div>
          <div
            v-if="selectedTemporarySetupSection"
            class="absolute inset-0 z-20 flex items-center justify-center bg-black/30 px-6"
            @click.self="selectedTemporarySetupSection = null"
          >
            <div class="flex max-h-[72vh] w-full max-w-md flex-col rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900">
              <div class="mb-4 flex items-center justify-between">
                <div>
                  <h4 class="text-base font-semibold text-gray-900 dark:text-white">
                    {{ selectedTemporarySetupSection.label }}
                  </h4>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ sectionSubtitle(selectedTemporarySetupSection) }}
                  </p>
                </div>
                <button
                  class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                  @click="selectedTemporarySetupSection = null"
                >
                  <X class="size-4" />
                </button>
              </div>
              <div class="min-h-0 flex-1 space-y-2 overflow-y-auto pr-1">
                <button
                  v-for="(item, index) in selectedTemporarySetupSection.items"
                  :key="setupItemKey(selectedTemporarySetupSection, item, index)"
                  class="w-full rounded-lg border border-gray-200 bg-white p-3 text-left transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-900 dark:hover:bg-gray-800"
                  @click="selectedTemporarySetupItem = item"
                >
                  <div class="flex items-center gap-2">
                    <span class="flex size-6 shrink-0 items-center justify-center rounded bg-gray-100 text-xs font-semibold text-gray-500 dark:bg-gray-800 dark:text-gray-300">
                      {{ index + 1 }}
                    </span>
                    <span class="min-w-0 flex-1 truncate text-sm font-medium text-gray-900 dark:text-white">
                      {{ item.title }}
                    </span>
                    <span v-if="item.appearanceTime" class="shrink-0 text-xs text-gray-400">
                      {{ item.appearanceTime }}
                    </span>
                  </div>
                  <p v-if="item.description" class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
                    {{ item.description }}
                  </p>
                </button>
              </div>
            </div>
          </div>
          <div
            v-if="selectedTemporarySetupItem"
            class="absolute inset-0 z-30 flex items-center justify-center bg-black/30 px-6"
            @click.self="selectedTemporarySetupItem = null"
          >
            <div class="flex max-h-[72vh] w-full max-w-md flex-col rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900">
              <div class="mb-4 flex shrink-0 items-center justify-between">
                <div>
                  <h4 class="text-base font-semibold text-gray-900 dark:text-white">
                    {{ selectedTemporarySetupItem.title }}
                  </h4>
                  <p v-if="selectedTemporarySetupItem.appearanceTime" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    出场时间：{{ selectedTemporarySetupItem.appearanceTime }}
                  </p>
                </div>
                <button
                  class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                  @click="selectedTemporarySetupItem = null"
                >
                  <X class="size-4" />
                </button>
              </div>
              <div class="min-h-0 flex-1 overflow-y-auto pr-1">
                <p v-if="selectedTemporarySetupItem.description" class="whitespace-pre-wrap rounded-lg bg-gray-50 p-3 text-sm leading-6 text-gray-700 dark:bg-gray-800 dark:text-gray-300">
                  {{ selectedTemporarySetupItem.description }}
                </p>
                <div v-if="selectedTemporarySetupItem.children.length > 0" class="mt-3 space-y-2">
                  <div
                    v-for="(child, index) in selectedTemporarySetupItem.children"
                    :key="setupChildKey(selectedTemporarySetupItem, child, index)"
                    class="rounded-lg border border-gray-200 p-3 dark:border-gray-700"
                  >
                    <div class="flex items-center justify-between gap-2">
                      <span class="text-sm font-medium text-gray-900 dark:text-white">{{ child.title }}</span>
                      <span v-if="child.appearanceTime" class="shrink-0 text-xs text-gray-400">{{ child.appearanceTime }}</span>
                    </div>
                    <p v-if="child.description" class="mt-1 whitespace-pre-wrap text-xs leading-5 text-gray-600 dark:text-gray-300">
                      {{ child.description }}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
