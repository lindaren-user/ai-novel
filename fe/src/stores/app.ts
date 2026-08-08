import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import {
  type ApiChapter,
  type ApiChatSessionMeta,
  type ApiMessage,
  type ApiModel,
  type ApiNovel,
  type ApiNovelOverview,
  type ApiWorkspaceDashboard,
  type ApiVolume,
  ApiRequestError,
  applyChapterPlanApi,
  applyVolumePlanApi,
  createModelApi,
  deleteChapterDraftApi,
  deleteModelApi,
  createNovelApi,
  createDownloadApi,
  downloadFileApi,
  deleteAccountApi,
  archiveNovelApi,
  createShareLinkApi,
  cancelChatStreamApi,
  getDashboardApi,
  getDownloadApi,
  listDownloadsApi,
  getChaptersApi,
  getMessagesApi,
  getModelsApi,
  getNovelOverviewApi,
  getNovelsApi,
  getSettingsApi,
  getVolumesApi,
  joinChapterDraftApi,
  listChapterDraftsApi,
  loginApi,
  loginByCodeApi,
  meApi,
  registerApi,
  restoreNovelApi,
  saveNovelSetupDraftApi,
  logoutApi,
  startNovelSetupDraftApi,
  testModelApi,
  toChapterContentDraft,
  updateNovelSetupDraftApi,
  updateSettingsApi,
  updateChapterDraftApi,
  useChapterDraftApi,
  streamChatApi,
  resumeChatStreamApi,
  streamA2UIData,
  toChapterDraft,
  streamDeltaData,
  streamDoneData,
  streamErrorData,
  streamSyncData,
  type StreamDone,
  type PlanOption,
  type A2UIEvent,
  type RenderData,
  type AppSettingsPayload,
  type AuthUser,
  type DownloadJob,
} from '@/services/api'
import type {
  Novel,
  Volume,
  Chapter,
  ChapterDraft,
  ChapterGenerationProgress,
  ChapterGenerationStepOutput,
  ChapterGenerationStepTiming,
  ChapterContentDraft,
  Message,
  GeneralSettings,
  NotificationSettings,
  PersonalizationSettings,
  AccountSettings,
  SettingsTab,
  OverviewData,
  NovelSetupData,
  NovelOverviewItem,
  WorkspaceDashboard,
} from '@/types'

const SELECTION_KEY = 'ai-novel-ide.selection'
const VIEW_MODE_KEY = 'ai-novel-ide.view-mode'
const ACTIVE_STREAM_KEYS_KEY = 'ai-novel-ide.active-stream-keys'
const MODEL_STATUS_ENABLED = 1
const NOVEL_STATUS_SETUP = 1
const NOVEL_STATUS_NORMAL = 2
const NOVEL_STATUS_ARCHIVED = 3
const CHAPTER_DRAFT_STATUS_NORMAL = 1
const CHAPTER_DRAFT_STATUS_CURRENT = 2
let localMessageSeq = 0

type ChatScope = 'novel' | 'volume' | 'chapter'

interface StreamingReply {
  id: string
  type: ChatScope
  entityId: number
  novelId?: number
  volumeId?: number
  content: string
  startedAt: Date
  lastTextAt?: Date
  isStreaming: boolean
  isRenderingUI?: boolean
  resumeStarted?: boolean
  planOptions?: PlanOption[]
  planOptionsPlaceholder?: boolean
  chapterDraft?: ChapterDraft
  chapterGeneration?: ChapterGenerationProgress
}

export type NovelStage =
  | 'idle'
  | 'awaiting_overview'
  | 'overview_received'
  | 'volume_planning'
  | 'volume_planned'
  | 'awaiting_chapter_count'
  | 'chapter_writing'

export const useAppStore = defineStore('app', () => {
  // ---- 认证状态 ----
  const isLoggedIn = ref(false)
  const authUsername = ref('')
  const authToken = ref('')
  const isAuthModalOpen = ref(false)
  const authModalMode = ref<'login' | 'register'>('login')

  // ---- 界面状态 ----
  const isSettingsOpen = ref(false)
  const settingsTab = ref<SettingsTab>('general')
  const isVolumesCollapsed = ref(true)
  const isNovelSetupChoiceOpen = ref(false)
  const isNovelSetupOpen = ref(false)
  const novelSetupFormResetTick = ref(0)
  const isLogoutConfirmOpen = ref(false)
  const isUnsavedSetupLeaveOpen = ref(false)
  const unsavedSetupLeaveAction = ref<(() => void) | null>(null)
  const novelSetupDirty = ref(false)
  const novelSetupGenerating = ref(false)

  // ---- 选中项 ----
  const selectedNovelId = ref<number | null>(null)
  const setupDraftNovelId = ref<number | null>(null)
  const selectedVolumeId = ref<number | null>(null)
  const selectedChapterId = ref<number | null>(null)
  const pendingSelection = ref<{ novelId?: number; volumeId?: number; chapterId?: number } | null>(null)

  // ---- 数据 ----
  const novels = ref<Novel[]>([])
  const models = ref<ApiModel[]>([])
  const isModelsLoading = ref(false)
  const isModelCreating = ref(false)
  const modelError = ref('')
  const isNovelsLoading = ref(false)
  const isVolumesLoading = ref(false)
  const isChaptersLoading = ref(false)
  const isMessagesLoading = ref(false)
  const isNovelCreating = ref(false)
  const savingDraftMessageId = ref<string | null>(null)
  const editorDrafts = ref<Record<string, ChapterContentDraft[]>>({})
  const editorDraftId = ref<number | null>(null)
  const editorSaveStatus = ref<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const applyingPlanMessageId = ref<string | null>(null)

  // 按实体 ID 缓存消息
  const novelMessages = ref<Record<string, Message[]>>({})
  const volumeMessages = ref<Record<string, Message[]>>({})
  const chapterMessages = ref<Record<string, Message[]>>({})
  const chatSessionMeta = ref<Record<string, ApiChatSessionMeta>>({})
  const activeStreams = ref<Record<string, StreamingReply>>({})
  const resumableStreamKeys = ref<Set<string>>(loadResumableStreamKeys())

  // 按实体 ID 缓存对话阶段
  const stages = ref<Record<string, NovelStage>>({})

  const archivedNovels = ref<Novel[]>([])
  const novelOverviews = ref<Record<string, NovelOverviewItem>>({})
  const dashboard = ref<WorkspaceDashboard | null>(null)
  const isDashboardLoading = ref(false)

  // ---- 计算属性 ----
  const selectedNovel = computed(() => novels.value.find((n) => n.id === selectedNovelId.value))
  const selectedVolume = computed(() => selectedNovel.value?.volumes.find((v) => v.id === selectedVolumeId.value))
  const selectedChapter = computed(() => selectedVolume.value?.chapters.find((c) => c.id === selectedChapterId.value))

  // 当前消息：章 > 卷 > 小说
  const activeMessages = computed<Message[]>(() => {
    if (selectedChapterId.value) {
      return chapterMessages.value[selectedChapterId.value] || []
    }
    if (selectedVolumeId.value) {
      return volumeMessages.value[selectedVolumeId.value] || []
    }
    if (selectedNovelId.value) {
      return novelMessages.value[selectedNovelId.value] || []
    }
    return []
  })
  const activeStream = computed(() => {
    if (selectedChapterId.value) return activeStreams.value[sessionKey('chapter', selectedChapterId.value)]
    if (selectedVolumeId.value) return activeStreams.value[sessionKey('volume', selectedVolumeId.value)]
    if (selectedNovelId.value) return activeStreams.value[sessionKey('novel', selectedNovelId.value)]
    return undefined
  })
  const activeChatPlaceholder = computed(() => '输入你的想法')

  // 面包屑：从小说到当前选中层级的路径
  const chatBreadcrumb = computed(() => {
    const parts: string[] = []
    if (selectedNovel.value) parts.push(selectedNovel.value.title)
    if (selectedVolume.value) parts.push(volumeDisplayTitle(selectedVolume.value))
    if (selectedChapter.value) parts.push(chapterDisplayTitle(selectedChapter.value))
    const stage = chatStageLabel()
    if (stage && parts.length > 0) {
      parts[parts.length - 1] = `${parts[parts.length - 1]}（${stage}）`
    }
    return parts
  })

  // 弹窗
  const overviewNovelId = ref<number | null>(null)
  const archiveTargetId = ref<number | null>(null)
  const overviewVolumeChapter = ref<{ type: 'volume' | 'chapter'; id: number; title: string; createdAt: Date } | null>(null)
  const shareTarget = ref<{ type: 'novel' | 'volume' | 'chapter'; id: number; title: string; description: string } | null>(null)
  const shareLink = ref('')
  const isShareCreating = ref(false)
  const shareError = ref('')
  const downloadJob = ref<DownloadJob | null>(null)
  const downloadJobs = ref<DownloadJob[]>([])
  const downloadError = ref('')
  const errorToast = ref('')
  const toastKind = ref<'error' | 'info'>('info')
  const customModelRequestTick = ref(0)
  const shareSecurityRequestTick = ref(0)

  // 设置
  const generalSettings = ref<GeneralSettings>({
    consistencyCheckCount: 3,
    autoSave: '30 秒',
    modelId: 0,
    model: '官方模型',
    customProvider: 'deepseek',
    customModelId: '',
    customApiUrl: '',
    customApiKey: '',
    downloadFormat: '.txt',
    downloadLayout: 'volume',
    shareSecurityKey: '',
  })

  const notificationSettings = ref<NotificationSettings>({
    desktopNotification: true,
    soundAlert: false,
    notificationContent: { newMessage: true, comment: true, like: true, system: false },
    doNotDisturb: false,
    doNotDisturbStart: '22:00',
    doNotDisturbEnd: '08:00',
  })

  const personalizationSettings = ref<PersonalizationSettings>({
    themeMode: 'light',
    language: '简体中文',
  })

  const accountSettings = ref<AccountSettings>({
    avatar: '/avatar.jpg',
    email: '',
    username: '',
    language: '简体中文',
  })

  const isSettingsLoading = ref(false)
  const isSettingsSaving = ref(false)
  const settingsError = ref('')
  const settingsUpdatedAt = ref('')
  const hasLoadedRemoteSettings = computed(() => !!settingsUpdatedAt.value)
  let settingsSnapshot = ''
  // 编辑模式
  const viewMode = ref<'chat' | 'editor'>('chat')
  const editorChapterId = ref<number | null>(null)

  const editorChapter = computed(() => {
    if (!editorChapterId.value) return null
    for (const novel of novels.value) {
      for (const vol of novel.volumes) {
        const ch = vol.chapters.find((c) => c.id === editorChapterId.value)
        if (ch) return { chapter: ch, volume: vol, novel }
      }
    }
    return null
  })

  const editorBreadcrumb = computed(() => {
    const ch = editorChapter.value
    if (!ch) return [] as string[]
    return [ch.novel.title, volumeDisplayTitle(ch.volume, ch.novel), chapterDisplayTitle(ch.chapter, ch.volume)]
  })
  const currentEditorDrafts = computed(() => editorChapterId.value ? editorDrafts.value[editorChapterId.value] || [] : [])
  const currentEditorDraft = computed(() => currentEditorDrafts.value.find((draft) => draft.id === editorDraftId.value) || null)

  const selectedModel = computed(() => models.value.find((model) => model.id === generalSettings.value.modelId))
  const isCustomModelSelected = computed(() => !!selectedModel.value && !!selectedModel.value.userId)
  const officialModel = computed(() => models.value.find((model) => !model.userId))

  // ---- 工具函数 ----
  function sessionKey(type: ChatScope, id: number) {
    return `${type}:${id}`
  }

  function parseSessionKey(key: string): { type: ChatScope; id: number } | null {
    const [type, rawId] = key.split(':')
    const id = Number(rawId)
    if ((type === 'novel' || type === 'volume' || type === 'chapter') && Number.isFinite(id)) {
      return { type, id }
    }
    return null
  }

  function loadResumableStreamKeys() {
    try {
      const parsed = JSON.parse(localStorage.getItem(ACTIVE_STREAM_KEYS_KEY) || '[]')
      return new Set(Array.isArray(parsed) ? parsed.filter((item) => typeof item === 'string') : [])
    } catch {
      return new Set<string>()
    }
  }

  function persistResumableStreamKeys() {
    localStorage.setItem(ACTIVE_STREAM_KEYS_KEY, JSON.stringify([...resumableStreamKeys.value]))
  }

  function markResumableStream(key: string) {
    resumableStreamKeys.value.add(key)
    persistResumableStreamKeys()
  }

  function clearResumableStream(key: string) {
    if (!resumableStreamKeys.value.delete(key)) return
    persistResumableStreamKeys()
  }

  function isSessionStreaming(type: 'novel' | 'volume' | 'chapter', id: number) {
    const key = sessionKey(type, id)
    return !!activeStreams.value[key] || resumableStreamKeys.value.has(key)
  }

  function findVolumeOwner(volumeId: number) {
    for (const novel of novels.value) {
      const volume = novel.volumes.find((item) => item.id === volumeId)
      if (volume) return { novel, volume }
    }
    return null
  }

  function findChapterOwner(chapterId: number) {
    for (const novel of novels.value) {
      for (const volume of novel.volumes) {
        const chapter = volume.chapters.find((item) => item.id === chapterId)
        if (chapter) return { novel, volume, chapter }
      }
    }
    return null
  }

  function streamOwner(type: 'novel' | 'volume' | 'chapter', id: number, stream?: StreamingReply) {
    if (type === 'novel') return { novelId: id, volumeId: undefined }
    if (type === 'volume') {
      return {
        novelId: stream?.novelId || findVolumeOwner(id)?.novel.id,
        volumeId: id,
      }
    }
    const owner = findChapterOwner(id)
    return {
      novelId: stream?.novelId || owner?.novel.id,
      volumeId: stream?.volumeId || owner?.volume.id,
    }
  }

  function streamingTargets() {
    const keys = new Set([...Object.keys(activeStreams.value), ...resumableStreamKeys.value])
    return [...keys].flatMap((key) => {
      const parsed = parseSessionKey(key)
      if (!parsed) return []
      const stream = activeStreams.value[key]
      const owner = streamOwner(parsed.type, parsed.id, stream)
      return [{ ...parsed, key, ...owner }]
    })
  }

  function streamingTargetForNovel(novelId: number) {
    const targets = streamingTargets().filter((item) => item.novelId === novelId)
    return targets.find((item) => item.type === 'chapter') ||
      targets.find((item) => item.type === 'volume') ||
      targets.find((item) => item.type === 'novel') ||
      null
  }

  function isNovelOrChildStreaming(novelId: number) {
    const target = streamingTargetForNovel(novelId)
    if (!target) return false
    return !isStreamingTargetVisibleInOutline(target)
  }

  function isStreamingTargetVisibleInOutline(target: ReturnType<typeof streamingTargetForNovel>) {
    if (!target || target.type === 'novel') return false
    if (selectedNovelId.value !== target.novelId || isVolumesCollapsed.value) return false
    if (target.type === 'volume') return true
    if (!target.volumeId) return false
    return !!findVolumeOwner(target.volumeId)?.volume.expanded
  }

  function toNovel(apiNovel: ApiNovel): Novel {
    const planData = normalizeOverview(apiNovel.planData)
    return {
      id: apiNovel.id,
      title: apiNovel.title || overviewTitle(planData, '未命名小说'),
      planData,
      status: apiNovel.status || 0,
      wordCount: apiNovel.wordCount || 0,
      createdAt: new Date(apiNovel.createdAt),
      updatedAt: new Date(apiNovel.updatedAt),
      volumes: [],
    }
  }

  function toNovelOverview(apiOverview: ApiNovelOverview): NovelOverviewItem {
    return {
      id: apiOverview.id,
      title: apiOverview.title,
      planData: normalizeOverview(apiOverview.planData),
      setupOriginalText: apiOverview.setupOriginalText || '',
      wordCount: apiOverview.wordCount || 0,
      updatedAt: new Date(apiOverview.updatedAt),
    }
  }

  function toDashboard(apiDashboard: ApiWorkspaceDashboard): WorkspaceDashboard {
    return {
      totalWords: apiDashboard.totalWords || 0,
      completedChapters: apiDashboard.completedChapters || 0,
      volumeCount: apiDashboard.volumeCount || 0,
      writingHours: apiDashboard.writingHours || 0,
      lastEditedAt: new Date(apiDashboard.lastEditedAt),
      wordTrend: (apiDashboard.wordTrend || []).map((point) => ({
        date: point.date,
        weekday: point.weekday,
        words: point.words || 0,
        wordLabel: point.wordLabel || `${point.words || 0} 字`,
      })),
    }
  }

  function toVolume(apiVolume: ApiVolume): Volume {
    const planData = normalizeOverview(apiVolume.planData)
    return {
      id: apiVolume.id,
      novelId: apiVolume.novelId,
      planData,
      chapterCount: apiVolume.chapterCount || 0,
      wordCount: apiVolume.wordCount || 0,
      createdAt: new Date(apiVolume.createdAt),
      chapters: [],
      expanded: false,
    }
  }

  function toChapter(apiChapter: ApiChapter): Chapter {
    return {
      id: apiChapter.id,
      volumeId: apiChapter.volumeId,
      planData: normalizeOverview(apiChapter.planData),
      content: apiChapter.content,
      status: apiChapter.status,
      wordCount: apiChapter.wordCount,
      createdAt: new Date(apiChapter.createdAt),
      completedAt: apiChapter.completedAt ? new Date(apiChapter.completedAt) : null,
    }
  }

  function normalizeOverview(value: unknown): OverviewData {
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      return value as OverviewData
    }
    if (typeof value === 'string' && value.trim()) {
      return { summary: value }
    }
    return {}
  }

  function overviewTitle(overview: OverviewData, fallback: string): string {
    const title = typeof overview.title === 'string' ? overview.title.trim() : ''
    return title || fallback
  }

  function setupPlanData(setupData: NovelSetupData): OverviewData {
    const planData: Record<string, unknown> = {
      title: setupData.title.trim(),
      summary: setupData.direction.trim(),
      direction: setupData.direction.trim(),
      tag_groups: setupData.tagGroups || {},
      characters: setupData.characters || [],
      relationships: setupData.relationships || [],
      maps: setupData.maps || [],
      forces: setupData.forces || [],
      other_settings: setupData.other_settings || [],
      perspective: setupData.perspective || '',
      length: setupData.length || '',
      length_range: setupData.lengthRange || '',
    }
    return planData as OverviewData
  }

  function countTextWords(content: string): number {
    let count = 0
    for (const ch of content) {
      if (ch === ' ' || ch === '\n' || ch === '\r' || ch === '\t') continue
      count += 1
    }
    return count
  }

  function chatStageLabel(): string {
    if (selectedChapterId.value) return '正文规划'
    if (selectedVolumeId.value) return '章节规划'
    if (selectedNovelId.value) return '卷规划'
    return ''
  }

  function volumeDisplayTitle(volume: Volume, novel = selectedNovel.value): string {
    const index = novel?.volumes.findIndex((item) => item.id === volume.id) ?? -1
    const title = volumeTitle(volume)
    return index >= 0 ? `第${index + 1}卷：${title}` : title
  }

  function chapterDisplayTitle(chapter: Chapter, volume = selectedVolume.value): string {
    const index = volume?.chapters.findIndex((item) => item.id === chapter.id) ?? -1
    const title = chapterTitle(chapter)
    return index >= 0 ? `第${index + 1}章：${title}` : title
  }

  function volumeTitle(volume: Volume): string {
    return overviewTitle(volume.planData, '未命名卷')
  }

  function chapterTitle(chapter: Chapter): string {
    return overviewTitle(chapter.planData, '未命名章节')
  }

  function numericID(value: unknown): number | undefined {
    if (typeof value === 'number' && Number.isFinite(value)) return value
    return undefined
  }

  function persistSelection() {
    const payload = {
      novelId: selectedNovelId.value || undefined,
      volumeId: selectedVolumeId.value || undefined,
      chapterId: selectedChapterId.value || undefined,
      isVolumesCollapsed: isVolumesCollapsed.value,
      isWorkspaceHome: isNovelSetupChoiceOpen.value && !selectedNovelId.value,
    }
    localStorage.setItem(SELECTION_KEY, JSON.stringify(payload))
  }

  function isSetupNovel(novel?: Novel | null): boolean {
    return novel?.status === NOVEL_STATUS_SETUP
  }

  function blockSetupLeaveWhileGenerating(): boolean {
    if (!isNovelSetupOpen.value || !novelSetupGenerating.value) return false
    unsavedSetupLeaveAction.value = null
    isUnsavedSetupLeaveOpen.value = true
    return true
  }

  async function openSetupNovel(novel: Novel) {
    selectedNovelId.value = novel.id
    selectedVolumeId.value = null
    selectedChapterId.value = null
    setupDraftNovelId.value = novel.id
    isNovelSetupChoiceOpen.value = false
    isNovelSetupOpen.value = false
    isVolumesCollapsed.value = true
    pendingSelection.value = null
    persistSelection()
    await loadNovelOverview(novel.id)
    isNovelSetupOpen.value = true
  }

  function restoreSelectionSnapshot() {
    const raw = localStorage.getItem(SELECTION_KEY)
    if (!raw) return null
    try {
      const parsed = JSON.parse(raw) as { novelId?: unknown; volumeId?: unknown; chapterId?: unknown; isVolumesCollapsed?: boolean; isWorkspaceHome?: boolean }
      if (typeof parsed.isVolumesCollapsed === 'boolean') {
        isVolumesCollapsed.value = parsed.isVolumesCollapsed
      }
      if (parsed.isWorkspaceHome) {
        isNovelSetupChoiceOpen.value = true
        selectedNovelId.value = null
        selectedVolumeId.value = null
        selectedChapterId.value = null
        return {}
      }
      return parsed && typeof parsed === 'object'
        ? {
            novelId: numericID(parsed.novelId),
            volumeId: numericID(parsed.volumeId),
            chapterId: numericID(parsed.chapterId),
          }
        : null
    } catch {
      return null
    }
  }

  function persistViewMode() {
    localStorage.setItem(VIEW_MODE_KEY, JSON.stringify({
      viewMode: viewMode.value,
      editorChapterId: editorChapterId.value || undefined,
      editorDraftId: editorDraftId.value || undefined,
    }))
  }

  function restoreViewModeSnapshot() {
    const raw = localStorage.getItem(VIEW_MODE_KEY)
    if (!raw) return
    try {
      const parsed = JSON.parse(raw) as { viewMode?: 'chat' | 'editor'; editorChapterId?: unknown; editorDraftId?: unknown }
      if (parsed.viewMode === 'editor') {
        viewMode.value = 'editor'
        editorChapterId.value = numericID(parsed.editorChapterId) || null
        editorDraftId.value = numericID(parsed.editorDraftId) || null
      } else {
        viewMode.value = 'chat'
        editorChapterId.value = null
      }
    } catch {
      viewMode.value = 'chat'
      editorChapterId.value = null
    }
  }

  function toMessage(apiMessage: NonNullable<Awaited<ReturnType<typeof createNovelApi>>['message']> | ApiMessage): Message {
    let content = apiMessage.content
    let planOptions: PlanOption[] | undefined
    let chapterDraft: ChapterDraft | undefined
    let chapterGeneration: ChapterGenerationProgress | undefined
    const renderData = apiMessage.role === 'assistant' ? apiMessage.renderData : undefined
    if (apiMessage.role === 'assistant') {
      chapterDraft = renderData?.kind === 'chapter_draft' ? toChapterDraft(renderData.draft) : undefined
      if (chapterDraft && apiMessage.draftId) {
        chapterDraft.draftId = apiMessage.draftId
      }
      planOptions = renderData?.kind === 'plan_options' && Array.isArray(renderData.options)
        ? renderData.options.map((option) => ({
            ...option,
            optionType: option.optionType || renderData.optionType,
          }))
        : undefined
      chapterGeneration = renderData?.kind === 'chapter_generation_progress' ? toChapterGenerationProgress(renderData) : undefined
    }
    return {
      id: String(apiMessage.id),
      draftId: apiMessage.draftId,
      role: apiMessage.role,
      content,
      timestamp: new Date(apiMessage.createdAt),
      temporary: apiMessage.temporary,
      isThinking: Boolean(apiMessage.temporary),
      planOptions,
      chapterGeneration,
      chapterDraft,
    }
  }

  function toChapterGenerationProgress(renderData: RenderData): ChapterGenerationProgress {
    const issues = Array.isArray(renderData.issues)
      ? renderData.issues.filter((item): item is string => typeof item === 'string' && item.trim().length > 0)
      : []
    const steps = Array.isArray(renderData.steps)
      ? renderData.steps.filter((item): item is string => typeof item === 'string' && item.trim().length > 0)
      : []
    const stepOutputs: ChapterGenerationStepOutput[] = Array.isArray(renderData.step_outputs)
      ? renderData.step_outputs
          .map(parseChapterGenerationStepOutput)
          .filter((item): item is ChapterGenerationStepOutput => Boolean(item))
      : []
    const stepTimings: ChapterGenerationStepTiming[] = Array.isArray(renderData.step_timings)
      ? renderData.step_timings
          .map((item) => ({
            key: typeof item.key === 'string' ? item.key : '',
            label: typeof item.label === 'string' ? item.label : '',
            startedAt: dateFromUnknown(item.startedAt),
            endedAt: dateFromUnknown(item.endedAt),
          }))
          .filter((item) => item.key && item.label)
      : []
    return {
      stage: typeof renderData.stage === 'string' ? renderData.stage : '',
      text: typeof renderData.text === 'string' ? renderData.text : '',
      attempt: typeof renderData.attempt === 'number' ? renderData.attempt : Number(renderData.attempt) || 0,
      steps,
      preview: typeof renderData.preview === 'string' ? renderData.preview : '',
      issues,
      stepOutputs,
      currentStepLabel: typeof renderData.current_step_label === 'string' ? renderData.current_step_label : '',
      currentStepStartedAt: dateFromUnknown(renderData.current_step_started_at),
      stepTimings,
      complete: renderData.complete === true,
      collapsed: renderData.collapsed === true,
      failed: renderData.failed === true,
    }
  }

  function parseChapterGenerationStepOutput(raw: unknown): ChapterGenerationStepOutput | undefined {
    if (!raw || typeof raw !== 'object') return undefined
    const item = raw as { step?: unknown; attempt?: unknown; type?: unknown; text?: unknown; items?: unknown }
    const output = {
      step: typeof item.step === 'string' ? item.step : '',
      attempt: typeof item.attempt === 'number' ? item.attempt : Number(item.attempt) || 0,
      type: typeof item.type === 'string' ? item.type : '',
      text: typeof item.text === 'string' ? item.text : '',
      items: Array.isArray(item.items)
        ? item.items.filter((value): value is string => typeof value === 'string' && value.trim().length > 0)
        : [],
    }
    return output.step && (output.text.trim() || output.items.length > 0) ? output : undefined
  }

  function dateFromUnknown(value: unknown): Date | undefined {
    if (typeof value !== 'string' || !value.trim()) return undefined
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? undefined : date
  }

  function settingsPayload(): AppSettingsPayload {
    return {
      general: { ...generalSettings.value },
      notification: {
        ...notificationSettings.value,
        notificationContent: { ...notificationSettings.value.notificationContent },
      },
      personalization: { ...personalizationSettings.value },
      account: { ...accountSettings.value },
    }
  }

  function notifyError(message: string) {
    const normalized = message === '响应格式不正确' ? '服务响应异常，请稍后重试' : message
    toastKind.value = 'error'
    errorToast.value = normalized
    window.setTimeout(() => {
      if (errorToast.value === normalized) errorToast.value = ''
    }, 3500)
  }

  function notifyInfo(message: string) {
    toastKind.value = 'info'
    errorToast.value = message
    window.setTimeout(() => {
      if (errorToast.value === message) errorToast.value = ''
    }, 3500)
  }

  function dismissToast() {
    errorToast.value = ''
  }

  function handleInvalidIDError(err: unknown) {
    const message = err instanceof Error ? err.message : ''
    if (message !== 'ID 参数不正确') return false
    selectedNovelId.value = null
    selectedVolumeId.value = null
    selectedChapterId.value = null
    pendingSelection.value = null
    localStorage.removeItem(SELECTION_KEY)
    notifyError('已清理失效选择，请重新选择小说、卷或章节')
    return true
  }

  function applySettings(settings: Partial<AppSettingsPayload>) {
    if (settings.general) {
      generalSettings.value = sanitizeGeneralSettings({ ...generalSettings.value, ...settings.general })
      generalSettings.value.consistencyCheckCount = normalizeConsistencyCheckCount(generalSettings.value.consistencyCheckCount)
    }
    if (settings.notification) {
      notificationSettings.value = {
        ...notificationSettings.value,
        ...settings.notification,
        notificationContent: {
          ...notificationSettings.value.notificationContent,
          ...settings.notification.notificationContent,
        },
      }
    }
    if (settings.personalization) {
      personalizationSettings.value = sanitizePersonalizationSettings({
        ...personalizationSettings.value,
        ...settings.personalization,
      })
    }
    if (settings.account) {
      const prevEmail = accountSettings.value.email
      accountSettings.value = { ...accountSettings.value, ...settings.account }
      // 服务端设置中 email 可能为空，保留已有的邮箱
      if (!accountSettings.value.email && prevEmail) {
        accountSettings.value.email = prevEmail
      }
      authUsername.value = accountSettings.value.username
    }
  }

  function normalizeConsistencyCheckCount(value: number) {
    const count = Math.trunc(Number(value) || 0)
    if (count <= 0) return 3
    return Math.min(10, count)
  }

  function sanitizeGeneralSettings(settings: GeneralSettings): GeneralSettings {
    return {
      consistencyCheckCount: normalizeConsistencyCheckCount(settings.consistencyCheckCount),
      autoSave: settings.autoSave,
      modelId: settings.modelId,
      model: settings.model,
      customProvider: settings.customProvider,
      customModelId: settings.customModelId,
      customApiUrl: settings.customApiUrl,
      customApiKey: settings.customApiKey,
      downloadFormat: settings.downloadFormat,
      downloadLayout: settings.downloadLayout,
      shareSecurityKey: settings.shareSecurityKey,
    }
  }

  function sanitizePersonalizationSettings(settings: PersonalizationSettings): PersonalizationSettings {
    return {
      themeMode: settings.themeMode,
      language: settings.language,
    }
  }

  function ensureSelectedModel() {
    if (generalSettings.value.modelId > 0 && models.value.some((model) => model.id === generalSettings.value.modelId)) {
      const model = selectedModel.value
      if (model) generalSettings.value.model = model.name
      return
    }

    const fallback = officialModel.value || models.value[0]
    if (!fallback) return
    generalSettings.value.modelId = fallback.id
    generalSettings.value.model = fallback.name
  }

  async function loadSettings() {
    if (!authToken.value) return
    isSettingsLoading.value = true
    settingsError.value = ''
    try {
      const response = await getSettingsApi()
      applySettings(response.settings || {})
      if (models.value.length > 0) ensureSelectedModel()
      settingsUpdatedAt.value = response.updatedAt
    } catch (err) {
      settingsError.value = err instanceof Error ? err.message : '读取设置失败'
      notifyError(settingsError.value)
      throw err
    } finally {
      isSettingsLoading.value = false
    }
  }

  async function saveSettings() {
    if (!authToken.value || !isLoggedIn.value) return
    isSettingsSaving.value = true
    settingsError.value = ''
    try {
      await updateSettingsApi(settingsPayload())
      settingsUpdatedAt.value = new Date().toISOString()
    } catch (err) {
      settingsError.value = err instanceof Error ? err.message : '保存设置失败'
      notifyError(settingsError.value)
    } finally {
      isSettingsSaving.value = false
    }
  }

  async function loadModels() {
    if (!authToken.value) return
    isModelsLoading.value = true
    modelError.value = ''
    try {
      models.value = await getModelsApi()
      ensureSelectedModel()
    } catch (err) {
      modelError.value = err instanceof Error ? err.message : '获取模型列表失败'
      notifyError(modelError.value)
    } finally {
      isModelsLoading.value = false
    }
  }

  async function createCustomModel(name?: string) {
    if (!authToken.value) {
      openAuthModal('login')
      return
    }
    isModelCreating.value = true
    modelError.value = ''
    try {
      const customCount = models.value.filter((item) => !!item.userId).length
      const model = await createModelApi({
        name: name || `自定义模型 ${customCount + 1}`,
        provider: generalSettings.value.customProvider,
        modelId: generalSettings.value.customModelId,
        apiUrl: generalSettings.value.customApiUrl,
        apiKey: generalSettings.value.customApiKey,
        status: MODEL_STATUS_ENABLED,
      })
      models.value = [...models.value.filter((item) => item.id !== model.id), model]
      generalSettings.value.modelId = model.id
      generalSettings.value.model = model.name
      generalSettings.value.customProvider = model.provider
      generalSettings.value.customModelId = model.modelId
      generalSettings.value.customApiUrl = model.apiUrl
      generalSettings.value.customApiKey = model.apiKey
      await saveSettings()
    } catch (err) {
      modelError.value = err instanceof Error ? err.message : '新增模型失败'
      throw err
    } finally {
      isModelCreating.value = false
    }
  }

  async function deleteCustomModel(modelId: number) {
    if (!authToken.value) return
    try {
      await deleteModelApi(modelId)
      models.value = models.value.filter((item) => item.id !== modelId)
      if (generalSettings.value.modelId === modelId) {
        const fallback = officialModel.value || models.value[0]
        if (fallback) {
          generalSettings.value.modelId = fallback.id
          generalSettings.value.model = fallback.name
        } else {
          generalSettings.value.modelId = 0
          generalSettings.value.model = '官方模型'
        }
      }
      notifyInfo('自定义模型已删除')
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '删除模型失败')
      throw err
    }
  }

  async function testCustomModel() {
    if (!authToken.value) {
      openAuthModal('login')
      return { ok: false, message: '请先登录' }
    }
    try {
      return await testModelApi({
        name: '模型连接测试',
        provider: generalSettings.value.customProvider,
        modelId: generalSettings.value.customModelId,
        apiUrl: generalSettings.value.customApiUrl,
        apiKey: generalSettings.value.customApiKey,
        status: MODEL_STATUS_ENABLED,
      })
    } catch (err) {
      throw err
    }
  }

  async function loadNovels() {
    if (!authToken.value) return
    isNovelsLoading.value = true
    try {
      const response = await getNovelsApi()
      if (!pendingSelection.value) pendingSelection.value = restoreSelectionSnapshot()
      const previousVolumes = new Map(novels.value.map((novel) => [novel.id, novel.volumes]))
      novels.value = response.map((apiNovel) => {
        const novel = toNovel(apiNovel)
        novel.volumes = previousVolumes.get(novel.id) || []
        return novel
      })
      if (novels.value.length === 0) {
        selectedNovelId.value = null
        selectedVolumeId.value = null
        selectedChapterId.value = null
        isVolumesCollapsed.value = true
        persistSelection()
        return
      }
      if (selectedNovelId.value && !novels.value.some((novel) => novel.id === selectedNovelId.value)) {
        selectedNovelId.value = null
        selectedVolumeId.value = null
        selectedChapterId.value = null
        isVolumesCollapsed.value = true
        persistSelection()
      }
      if (!selectedNovelId.value && pendingSelection.value?.novelId) {
        const pendingNovel = novels.value.find(
          (novel) => novel.id === pendingSelection.value?.novelId,
        )
        if (pendingNovel) {
          if (isSetupNovel(pendingNovel)) {
            void openSetupNovel(pendingNovel)
          } else {
            selectedNovelId.value = pendingNovel.id
            setupDraftNovelId.value = null
            isNovelSetupOpen.value = false
            void loadVolumes(pendingNovel.id)
            if (!pendingSelection.value.volumeId && !pendingSelection.value.chapterId) {
              void loadMessages('novel', pendingNovel.id)
            }
          }
        }
      }
    } catch (err) {
      if (handleInvalidIDError(err)) return
      notifyError(err instanceof Error ? err.message : '获取小说列表失败')
    } finally {
      isNovelsLoading.value = false
    }
  }

  async function loadNovelOverview(id: number, force = false) {
    if (!authToken.value) return
    if (novelOverviews.value[id] && !force) return novelOverviews.value[id]
    try {
      const overview = toNovelOverview(await getNovelOverviewApi(id))
      novelOverviews.value = {
        ...novelOverviews.value,
        [overview.id]: overview,
      }
      const novel = novels.value.find((item) => item.id === overview.id)
      if (novel) {
        novel.planData = overview.planData
        novel.setupOriginalText = overview.setupOriginalText || ''
        novel.wordCount = overview.wordCount
        novel.updatedAt = overview.updatedAt
      }
      return overview
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '获取小说梗概失败')
      return undefined
    }
  }

  async function loadDashboard(force = false) {
    if (!authToken.value) return
    if (dashboard.value && !force) return
    isDashboardLoading.value = true
    try {
      dashboard.value = toDashboard(await getDashboardApi())
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '获取工作台数据失败')
    } finally {
      isDashboardLoading.value = false
    }
  }

  async function loadArchivedNovels() {
    if (!authToken.value) return
    try {
      const archived = await getNovelsApi('archived')
      archivedNovels.value = archived.map(toNovel)
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '获取回收站列表失败')
    }
  }

  async function loadVolumes(novelId: number) {
    if (!authToken.value) return
    const novel = novels.value.find((item) => item.id === novelId)
    if (!novel) return
    if (isSetupNovel(novel)) return

    isVolumesLoading.value = true
    try {
      const response = await getVolumesApi(novelId)
      const previousVolumes = new Map(novel.volumes.map((volume) => [volume.id, volume]))
      novel.volumes = response.map((apiVolume) => {
        const previous = previousVolumes.get(apiVolume.id)
        const volume = toVolume(apiVolume)
        if (previous) {
          volume.expanded = previous.expanded
          volume.chapters = previous.chapters
          volume.chapterCount = volume.chapterCount || previous.chapterCount
        }
        return volume
      })
      if (selectedVolumeId.value && !novel.volumes.some((volume) => volume.id === selectedVolumeId.value)) {
        selectedVolumeId.value = null
        selectedChapterId.value = null
      }
      const pending = pendingSelection.value
      if (pending?.volumeId && pending.novelId === novelId && novel.volumes.some((volume) => volume.id === pending.volumeId)) {
        selectedVolumeId.value = pending.volumeId
        const volume = novel.volumes.find((item) => item.id === pending.volumeId)
        if (volume) volume.expanded = true
        void loadChapters(pending.volumeId)
        if (!pending.chapterId) {
          pendingSelection.value = null
          void loadMessages('volume', pending.volumeId)
        }
      }
    } catch (err) {
      if (handleInvalidIDError(err)) return
      notifyError(err instanceof Error ? err.message : '获取卷列表失败')
    } finally {
      isVolumesLoading.value = false
    }
  }

  async function loadChapters(volumeId: number) {
    if (!authToken.value) return
    let targetVolume: Volume | undefined
    for (const novel of novels.value) {
      targetVolume = novel.volumes.find((volume) => volume.id === volumeId)
      if (targetVolume) break
    }
    if (!targetVolume) return

    isChaptersLoading.value = true
    try {
      const response = await getChaptersApi(volumeId)
      const previousChapters = new Map(targetVolume.chapters.map((chapter) => [chapter.id, chapter]))
      targetVolume.chapters = response.map((apiChapter) => ({
        ...toChapter(apiChapter),
        content: apiChapter.content || previousChapters.get(apiChapter.id)?.content,
      }))
      targetVolume.chapterCount = targetVolume.chapters.length
      targetVolume.expanded = true
      if (selectedChapterId.value && !targetVolume.chapters.some((chapter) => chapter.id === selectedChapterId.value)) {
        selectedChapterId.value = null
      }
      const pending = pendingSelection.value
      if (pending?.chapterId && pending.volumeId === volumeId && targetVolume.chapters.some((chapter) => chapter.id === pending.chapterId)) {
        selectedChapterId.value = pending.chapterId
        if (viewMode.value === 'editor') {
          editorChapterId.value = pending.chapterId
          void loadChapterDrafts(pending.chapterId)
          persistViewMode()
        }
        pendingSelection.value = null
        void loadMessages('chapter', pending.chapterId)
      }
    } catch (err) {
      if (handleInvalidIDError(err)) return
      notifyError(err instanceof Error ? err.message : '获取章节列表失败')
    } finally {
      isChaptersLoading.value = false
    }
  }

  // ---- 选中项 ----
  function messageStoreByType(type: 'novel' | 'volume' | 'chapter') {
    if (type === 'novel') return novelMessages.value
    if (type === 'volume') return volumeMessages.value
    return chapterMessages.value
  }

  function nextLocalMessageId(prefix: string) {
    localMessageSeq += 1
    return `${prefix}${Date.now()}-${localMessageSeq}`
  }

  function replaceMessageList(type: 'novel' | 'volume' | 'chapter', id: number, list: Message[]) {
    if (type === 'novel') {
      novelMessages.value = { ...novelMessages.value, [id]: list }
      return
    }
    if (type === 'volume') {
      volumeMessages.value = { ...volumeMessages.value, [id]: list }
      return
    }
    chapterMessages.value = { ...chapterMessages.value, [id]: list }
  }

  function upsertMessage(store: Record<string, Message[]>, id: number, message: Message) {
    if (!store[id]) store[id] = []
    if (message.temporary) {
      store[id] = store[id].filter((item) => !item.temporary || item.id === message.id)
    }
    const index = store[id].findIndex((item) => item.id === message.id)
    if (index >= 0) {
      store[id].splice(index, 1, message)
    } else {
      store[id].push(message)
    }
  }

  function removeMessage(type: 'novel' | 'volume' | 'chapter', id: number, messageId: string) {
    const list = messageStoreByType(type)[id] || []
    replaceMessageList(type, id, list.filter((item) => item.id !== messageId))
  }

  function mergeActiveStream(type: 'novel' | 'volume' | 'chapter', id: number, messages: Message[]) {
    const stream = activeStreams.value[sessionKey(type, id)]
    const existingMessages = messageStoreByType(type)[id] || []
    const stableMessages = mergeOptimisticMessages(messages.filter((item) => !item.temporary), existingMessages)
    if (!stream) return [...stableMessages, ...messages.filter((item) => item.temporary).slice(-1)]
    const streamingMessage: Message = {
      id: stream.id,
      role: 'assistant',
      content: stream.content,
      timestamp: stream.startedAt,
      lastTextAt: stream.lastTextAt,
      temporary: true,
      isThinking: stream.isStreaming && !stream.isRenderingUI,
      planOptions: stream.planOptions,
      planOptionsPlaceholder: stream.planOptionsPlaceholder,
      chapterGeneration: stream.chapterGeneration,
      chapterDraft: stream.chapterDraft,
    }
    return [...stableMessages, streamingMessage]
  }

  function mergeOptimisticMessages(serverMessages: Message[], existingMessages: Message[]) {
    const merged = [...serverMessages]
    for (const message of existingMessages) {
      if (!message.optimistic || message.temporary) continue
      const existsOnServer = serverMessages.some((item) =>
        item.role === message.role &&
        item.content === message.content &&
        Math.abs(item.timestamp.getTime() - message.timestamp.getTime()) < 5 * 60 * 1000
      )
      if (!existsOnServer) merged.push(message)
    }
    return merged.sort((left, right) => left.timestamp.getTime() - right.timestamp.getTime())
  }

  function ensureResumeStream(type: 'novel' | 'volume' | 'chapter', id: number, messages: Message[]) {
    const key = sessionKey(type, id)
    if (activeStreams.value[key]) return
    const serverStreamingMessage = messages.find((item) => item.temporary)
    if (!serverStreamingMessage && !resumableStreamKeys.value.has(key)) return
    const message = serverStreamingMessage || {
      id: `resume-${key}`,
      role: 'assistant' as const,
      content: '',
      timestamp: new Date(),
      temporary: true,
      isThinking: true,
    }
    const store = messageStoreByType(type)
    upsertMessage(store, id, message)
    const owner = streamOwner(type, id)
    activeStreams.value[key] = {
      id: message.id,
      type,
      entityId: id,
      novelId: owner.novelId,
      volumeId: owner.volumeId,
      content: message.content,
      startedAt: message.timestamp,
      lastTextAt: message.lastTextAt,
      isStreaming: true,
      isRenderingUI: Boolean(message.planOptions || message.chapterGeneration || message.chapterDraft),
      chapterGeneration: message.chapterGeneration,
      resumeStarted: true,
    }
    void resumeStream(type, id, message.id)
  }

  async function resumeStream(type: 'novel' | 'volume' | 'chapter', id: number, messageId: string) {
    if (!authToken.value) return
    const key = sessionKey(type, id)
    try {
      for await (const event of resumeChatStreamApi(type, id)) {
        const stream = activeStreams.value[key]
        const currentMessage = messageStoreByType(type)[id]?.find((item) => item.id === messageId)
        if (event.type === 'sync') {
          const sync = streamSyncData(event)
          if (stream) {
            stream.content = sync?.content || ''
            if (stream.content.trim()) stream.lastTextAt = new Date()
          }
          if (currentMessage) {
            currentMessage.content = sync?.content || ''
            if (currentMessage.content.trim()) currentMessage.lastTextAt = new Date()
          }
          if (sync?.renderData) applyA2UIEvent(stream, currentMessage, { data: sync.renderData }, true, true)
        } else if (event.type === 'delta') {
          const text = streamDeltaData(event)?.text || ''
          if (stream) {
            stream.content += text
            if (text.trim()) stream.lastTextAt = new Date()
            if (currentMessage) {
              currentMessage.content = stream.content
              currentMessage.lastTextAt = stream.lastTextAt
            }
          } else if (currentMessage) {
            currentMessage.content += text
            if (text.trim()) currentMessage.lastTextAt = new Date()
          }
        } else if (event.type === 'a2ui') {
          applyA2UIEvent(stream, currentMessage, streamA2UIData(event))
        } else if (event.type === 'done') {
          const done = streamDoneData(event)
          const finalMessage = applyDoneMessage(type, id, messageId, done, stream?.content || currentMessage?.content || '')
          delete activeStreams.value[key]
          clearResumableStream(key)
          if (!finalMessage?.content && !finalMessage?.chapterDraft && !finalMessage?.chapterGeneration && !finalMessage?.planOptions?.length) {
            const list = messageStoreByType(type)[id] || []
            messageStoreByType(type)[id] = list.filter((item) => item.id !== messageId)
          }
          return
        } else if (event.type === 'error') {
          delete activeStreams.value[key]
          clearResumableStream(key)
          clearStreamingMessage(type, id, messageId)
          notifyError(streamErrorData(event)?.message || 'AI 响应出错')
          return
        }
      }
    } catch (err) {
      const currentMessage = messageStoreByType(type)[id]?.find((item) => item.id === messageId)
      delete activeStreams.value[key]
      clearResumableStream(key)
      if (currentMessage?.content) {
        currentMessage.isThinking = false
        currentMessage.temporary = false
        currentMessage.chapterGeneration = undefined
      } else if (currentMessage) {
        clearStreamingMessage(type, id, messageId)
      }
    }
  }

  async function loadMessages(type: 'novel' | 'volume' | 'chapter', id: number, silent = false) {
    if (!authToken.value) return
    if (type === 'novel' && isSetupNovel(novels.value.find((item) => item.id === id))) return
    if (!silent) isMessagesLoading.value = true
    try {
      const messages = await getMessagesApi(type, id)
      const store = messageStoreByType(type)
      const mappedMessages = mergeActiveStream(type, id, messages.messages.map(toMessage))
      store[id] = mappedMessages
      chatSessionMeta.value[sessionKey(type, id)] = messages.session
      ensureResumeStream(type, id, mappedMessages)
    } catch (err) {
      if (!handleInvalidIDError(err)) {
        notifyError(err instanceof Error ? err.message : '获取对话消息失败')
      }
    } finally {
      if (!silent) isMessagesLoading.value = false
    }
  }

  function planOptionPayload(option: PlanOption): Record<string, unknown> {
    const details = option.details && typeof option.details === 'object' ? option.details : {}
    return {
      ...details,
      title: option.title,
      summary: option.description || (typeof details.summary === 'string' ? details.summary : ''),
    }
  }

  async function applyGeneratedPlan(messageId: string, optionType: 'volume' | 'chapter', options: PlanOption[], force = false) {
    const plans = options.filter((item) => !item.custom).map(planOptionPayload)
    if (plans.length === 0) {
      notifyError('没有可应用的规划')
      return 'error' as const
    }

    applyingPlanMessageId.value = messageId
    try {
      if (optionType === 'volume') {
        if (!selectedNovelId.value) throw new Error('请先选择小说')
        const volumes = (await applyVolumePlanApi(selectedNovelId.value, plans, force)).map(toVolume)
        const novel = novels.value.find((item) => item.id === selectedNovelId.value)
        if (novel) {
          novel.volumes = volumes
          novel.status = NOVEL_STATUS_NORMAL
          isVolumesCollapsed.value = false
        }
        selectedVolumeId.value = null
        selectedChapterId.value = null
        delete novelOverviews.value[selectedNovelId.value]
      } else {
        if (!selectedVolumeId.value) throw new Error('请先选择卷')
        const chapters = (await applyChapterPlanApi(selectedVolumeId.value, plans, force)).map(toChapter)
        const volume = selectedVolume.value
        if (volume) {
          volume.chapters = chapters
          volume.expanded = true
        }
        selectedChapterId.value = null
      }
      dashboard.value = null
      persistSelection()
      notifyInfo('规划已应用')
      return 'applied' as const
    } catch (err) {
      if (err instanceof ApiRequestError && err.status === 409) return 'overwrite_required' as const
      notifyError(err instanceof Error ? err.message : '应用规划失败')
      return 'error' as const
    } finally {
      applyingPlanMessageId.value = null
    }
  }

  function selectNovel(id: number) {
    if (blockSetupLeaveWhileGenerating()) return
    if (isNovelSetupOpen.value && novelSetupDirty.value) {
      unsavedSetupLeaveAction.value = () => {
        isNovelSetupOpen.value = false
        isNovelSetupChoiceOpen.value = false
        setupDraftNovelId.value = null
        novelSetupDirty.value = false
        const novel = novels.value.find((item) => item.id === id)
        if (!novel) return
        if (isSetupNovel(novel)) {
          void openSetupNovel(novel)
          return
        }
        openNovelChat(novel.id)
      }
      isUnsavedSetupLeaveOpen.value = true
      return
    }
    const novel = novels.value.find((item) => item.id === id)
    if (!novel) return
    if (isSetupNovel(novel)) {
      void openSetupNovel(novel)
      return
    }
    openNovelChat(novel.id)
  }

  function openNovelChat(novelId: number) {
    const target = streamingTargetForNovel(novelId)
    isNovelSetupChoiceOpen.value = false
    isNovelSetupOpen.value = false
    setupDraftNovelId.value = null
    isVolumesCollapsed.value = false
    selectedNovelId.value = novelId
    selectedVolumeId.value = null
    selectedChapterId.value = null
    pendingSelection.value = null

    if (target?.type === 'chapter' && target.volumeId) {
      selectedVolumeId.value = target.volumeId
      selectedChapterId.value = target.id
      pendingSelection.value = { novelId, volumeId: target.volumeId, chapterId: target.id }
      expandLoadedVolume(target.volumeId)
      persistSelection()
      void loadVolumes(novelId)
      void loadChapters(target.volumeId)
      void loadMessages('chapter', target.id)
      return
    }

    if (target?.type === 'volume') {
      selectedVolumeId.value = target.id
      pendingSelection.value = { novelId, volumeId: target.id }
      expandLoadedVolume(target.id)
      persistSelection()
      void loadVolumes(novelId)
      void loadChapters(target.id)
      void loadMessages('volume', target.id)
      return
    }

    persistSelection()
    void loadVolumes(novelId)
    void loadMessages('novel', novelId)
  }

  function expandLoadedVolume(volumeId: number) {
    const volume = findVolumeOwner(volumeId)?.volume
    if (volume) volume.expanded = true
  }

  function selectVolume(id: number) {
    selectedVolumeId.value = id
    selectedChapterId.value = null
    for (const novel of novels.value) {
      const vol = novel.volumes.find((v) => v.id === id)
      if (vol) { selectedNovelId.value = novel.id; break }
    }
    pendingSelection.value = null
    persistSelection()
    void loadMessages('volume', id)
  }

  function selectChapter(id: number) {
    selectedChapterId.value = id
    for (const novel of novels.value) {
      for (const vol of novel.volumes) {
        if (vol.chapters.some((c) => c.id === id)) {
          selectedNovelId.value = novel.id
          selectedVolumeId.value = vol.id
          pendingSelection.value = null
          persistSelection()
          break
        }
      }
    }
    if (viewMode.value === 'editor') {
      editorChapterId.value = id
      void loadChapterDrafts(id)
      persistViewMode()
    } else {
      void loadMessages('chapter', id)
    }
  }

  function toggleVolumeExpanded(id: number) {
    const novel = selectedNovel.value
    if (!novel) return
    const volume = novel.volumes.find((v) => v.id === id)
    if (!volume) return
    const nextExpanded = !volume.expanded
    volume.expanded = nextExpanded
    if (nextExpanded && volume.chapters.length === 0) {
      void loadChapters(id)
    }
  }

  function setVolumesCollapsed(collapsed: boolean) {
    isVolumesCollapsed.value = collapsed
    persistSelection()
  }

  function renderDataFromA2UI(ui?: A2UIEvent): RenderData | undefined {
    if (!ui?.data) return undefined
    return ui.data
  }

  function applyA2UIEvent(
    stream: StreamingReply | undefined,
    message: Message | undefined,
    ui?: A2UIEvent,
    replacePlanOptions = false,
    snapshot = false,
  ) {
    if (stream) stream.isRenderingUI = true
    if (message) message.isThinking = false
    const data = renderDataFromA2UI(ui)
    if (!data) return
    if (data.kind === 'plan_options' && Array.isArray(data.options)) {
      if (data.options.length === 0) {
        if (stream) stream.planOptionsPlaceholder = true
        if (message) {
          message.planOptionsPlaceholder = true
          message.planOptions = message.planOptions || []
          message.isThinking = false
        }
        return
      }
      const incomingOptions = data.options.map((option) => ({
        ...option,
        optionType: option.optionType || data.optionType,
      }))
      const streamOptions = replacePlanOptions ? incomingOptions : mergePlanOptions(stream?.planOptions, incomingOptions)
      const messageOptions = replacePlanOptions ? incomingOptions : mergePlanOptions(message?.planOptions, incomingOptions)
      if (stream) {
        stream.planOptions = streamOptions
        stream.planOptionsPlaceholder = false
      }
      if (message) {
        message.planOptions = messageOptions
        message.planOptionsPlaceholder = false
        message.isThinking = false
      }
      return
    }
    if (data.kind === 'chapter_draft') {
      const draft = mergeChapterDraftPatch(message?.chapterDraft || stream?.chapterDraft, data.draft, snapshot)
      if (!draft) return
      if (stream) {
        stream.chapterDraft = draft
      }
      if (message) {
        message.chapterDraft = draft
        if (message.chapterGeneration) {
          message.chapterGeneration = {
            ...message.chapterGeneration,
            complete: true,
            collapsed: false,
          }
        }
        message.isThinking = false
      }
      return
    }
    if (data.kind === 'chapter_generation_progress') {
      const progress = mergeChapterGenerationProgressPatch(
        message?.chapterGeneration || stream?.chapterGeneration,
        data,
        snapshot,
      )
      if (stream) {
        stream.chapterGeneration = progress
        stream.content = ''
      }
      if (message) {
        message.chapterGeneration = progress
        message.content = ''
        message.isThinking = false
      }
    }
  }

  function mergeChapterGenerationProgressPatch(
    current: ChapterGenerationProgress | undefined,
    renderData: RenderData,
    snapshot = false,
  ): ChapterGenerationProgress {
    if (snapshot) return toChapterGenerationProgress(renderData)
    if (!renderData.step_output_delta) {
      const incoming = toChapterGenerationProgress(renderData)
      if (!current) return incoming
      return {
        ...incoming,
        stepOutputs: mergeChapterGenerationStepOutputs(current.stepOutputs, incoming.stepOutputs),
      }
    }
    const output = parseChapterGenerationStepOutput(renderData.step_output_delta)
    const progress = current || toChapterGenerationProgress(renderData)
    if (!output) return progress

    const stepOutputs = [...progress.stepOutputs]
    const index = stepOutputs.findIndex((item) => item.step === output.step && item.attempt === output.attempt)
    if (index >= 0) {
      const currentOutput = stepOutputs[index]
      stepOutputs[index] = {
        ...currentOutput,
        type: output.type || currentOutput.type,
        text: `${currentOutput.text || ''}${output.text || ''}`,
        items: output.items.length ? output.items : currentOutput.items,
      }
    } else {
      stepOutputs.push(output)
    }
    return { ...progress, stepOutputs }
  }

  function mergeChapterGenerationStepOutputs(
    current: ChapterGenerationStepOutput[],
    incoming: ChapterGenerationStepOutput[],
  ) {
    const currentByKey = new Map(current.map((item) => [chapterGenerationStepOutputKey(item), item]))
    const merged = incoming.map((item) => {
      const existing = currentByKey.get(chapterGenerationStepOutputKey(item))
      if (!existing || item.type !== 'text' || existing.type !== 'text') return item
      return existing.text.length > item.text.length ? { ...item, text: existing.text } : item
    })
    for (const item of current) {
      if (!merged.some((value) => chapterGenerationStepOutputKey(value) === chapterGenerationStepOutputKey(item))) {
        merged.push(item)
      }
    }
    return merged
  }

  function chapterGenerationStepOutputKey(output: Pick<ChapterGenerationStepOutput, 'step' | 'attempt'>) {
    return `${output.step}:${output.attempt}`
  }

  function mergeChapterDraftPatch(
    current: ChapterDraft | undefined,
    raw: NonNullable<RenderData['draft']> | undefined,
    snapshot = false,
  ): ChapterDraft | undefined {
    if (!raw || typeof raw !== 'object') return current
    const title = typeof raw.title === 'string' && raw.title.trim() ? raw.title.trim() : ''
    const content = typeof raw.content === 'string' ? raw.content : ''
    const revisionNotes = typeof raw.revision_notes === 'string' ? raw.revision_notes.trim() : ''
    const draftId = numericID(raw.draft_id ?? raw.draftId)

    const next: ChapterDraft = current
      ? { ...current }
      : {
          title: title || '章节正文',
          content: '',
          revisionNotes: '',
        }

    if (snapshot) {
      if (title) next.title = title
      next.revisionNotes = revisionNotes
    } else {
      if (title) next.title = title
      if (revisionNotes) next.revisionNotes = revisionNotes
    }
    if (draftId) next.draftId = draftId

    if (snapshot) {
      next.content = content
    } else if (content) {
      next.content = (next.content || '') + content
    }

    return next
  }

  function mergePlanOptions(current: PlanOption[] | undefined, incoming: PlanOption[]) {
    const merged: PlanOption[] = []
    const seen = new Set<string>()
    for (const option of [...(current || []), ...incoming]) {
      const key = planOptionKey(option)
      if (seen.has(key)) continue
      seen.add(key)
      merged.push(option)
    }
    return merged.map((option, index) => ({
      ...option,
      id: option.id || `option-${index + 1}`,
    }))
  }

  function planOptionKey(option: PlanOption) {
    return [
      option.optionType || '',
      option.title || '',
      option.description || '',
      JSON.stringify(option.details || {}),
    ].join('|')
  }

  function clearStreamingMessage(type: 'novel' | 'volume' | 'chapter', id: number, messageId: string) {
    const message = messageStoreByType(type)[id]?.find((item) => item.id === messageId)
    if (!message) return
    const hadChapterGeneration = Boolean(message.chapterGeneration)
    message.isThinking = false
    message.temporary = false
    if (message.chapterDraft) {
      message.chapterGeneration = undefined
      message.content = ''
      return
    }
    message.chapterGeneration = undefined
    if (hadChapterGeneration) {
      message.content = ''
    }
    if (!message.content.trim() && !message.chapterDraft && !message.planOptions?.length) {
      removeMessage(type, id, message.id)
    }
  }

  function finishStreamingMessage(message?: Message) {
    if (!message) return
    message.isThinking = false
    message.temporary = false
    message.timestamp = new Date()
    if (message.planOptions) {
      message.planOptions = message.planOptions.map((option) => ({
        ...option,
      }))
    }
    message.planOptionsPlaceholder = false
  }

  function applyDoneEvent(message: Message | undefined, draftId?: number) {
    if (!message || !draftId) return
    message.draftId = draftId
    if (!message.chapterDraft && message.chapterGeneration) {
      const content = latestChapterGenerationDraftContent(message.chapterGeneration)
      if (content.trim()) {
        message.chapterDraft = {
          draftId,
          title: '章节正文',
          content,
          revisionNotes: '高一致性模式已完成上下文校验。',
        }
        message.chapterGeneration = undefined
        message.content = ''
      }
    }
    if (message.chapterDraft) {
      message.chapterDraft.draftId = draftId
    }
  }

  function latestChapterGenerationDraftContent(progress: ChapterGenerationProgress) {
    const rewrite = latestChapterGenerationOutputByStep(progress.stepOutputs, '按校验意见重写')
    const generated = latestChapterGenerationOutputByStep(progress.stepOutputs, '生成正文')
    return rewrite?.text || generated?.text || ''
  }

  function latestChapterGenerationOutputByStep(outputs: ChapterGenerationStepOutput[], step: string) {
    return outputs
      .filter((item) => item.type === 'text' && item.step === step)
      .reduce<ChapterGenerationStepOutput | undefined>(
        (latest, item) => (!latest || item.attempt > latest.attempt ? item : latest),
        undefined,
      )
  }

  function applyDoneMessage(type: 'novel' | 'volume' | 'chapter', id: number, localMessageId: string, done?: StreamDone, fallbackContent = '') {
    const currentMessage = messageStoreByType(type)[id]?.find((item) => item.id === localMessageId)
    if (currentMessage && !currentMessage.content && fallbackContent.trim()) {
      currentMessage.content = fallbackContent
    }
    applyDoneEvent(currentMessage, done?.params?.draftId)
    finishStreamingMessage(currentMessage)
    return currentMessage
  }

  // ---- 发送消息（流式） ----
  async function sendMessage(content: string, options: { graphMode?: boolean } = {}) {
    const targetId = selectedChapterId.value || selectedVolumeId.value || selectedNovelId.value
    if (!targetId || !authToken.value) return
    let msgStore: Record<string, Message[]>
    let scopeType: 'novel' | 'volume' | 'chapter'
    if (selectedChapterId.value) {
      msgStore = chapterMessages.value
      scopeType = 'chapter'
    } else if (selectedVolumeId.value) {
      msgStore = volumeMessages.value
      scopeType = 'volume'
    } else {
      msgStore = novelMessages.value
      scopeType = 'novel'
    }

    const key = sessionKey(scopeType, targetId)
    isMessagesLoading.value = false
    const userMessage: Message = {
      id: nextLocalMessageId('u'),
      role: 'user',
      content,
      timestamp: new Date(),
      optimistic: true,
    }
    replaceMessageList(scopeType, targetId, [...(msgStore[targetId] || []), userMessage])
    msgStore = messageStoreByType(scopeType)

    const aiMsgId = nextLocalMessageId('a')
    const assistantMessage: Message = {
      id: aiMsgId,
      role: 'assistant',
      content: '',
      temporary: true,
      isThinking: !(scopeType === 'chapter' && options.graphMode === true),
      timestamp: new Date(),
    }
    const owner = streamOwner(scopeType, targetId)
    activeStreams.value[key] = {
      id: aiMsgId,
      type: scopeType,
      entityId: targetId,
      novelId: owner.novelId,
      volumeId: owner.volumeId,
      content: '',
      startedAt: assistantMessage.timestamp,
      lastTextAt: assistantMessage.lastTextAt,
      isStreaming: true,
      isRenderingUI: scopeType === 'chapter' && options.graphMode === true,
    }
    markResumableStream(key)
    upsertMessage(msgStore, targetId, assistantMessage)

    try {
      for await (const event of streamChatApi(scopeType, targetId, content, scopeType === 'chapter' && options.graphMode === true)) {
        const stream = activeStreams.value[key]
        const currentStore = messageStoreByType(scopeType)
        const currentMessage = currentStore[targetId]?.find((item) => item.id === aiMsgId)
        if (event.type === 'delta') {
          const text = streamDeltaData(event)?.text || ''
          if (stream) {
            stream.content += text
            if (text.trim()) stream.lastTextAt = new Date()
            if (currentMessage) {
              currentMessage.content = stream.content
              currentMessage.lastTextAt = stream.lastTextAt
            }
          } else if (currentMessage) {
            currentMessage.content += text
            if (text.trim()) currentMessage.lastTextAt = new Date()
          }
        } else if (event.type === 'a2ui') {
          applyA2UIEvent(stream, currentMessage, streamA2UIData(event))
        } else if (event.type === 'done') {
          const finalContent = stream?.content || currentMessage?.content || ''
          if (stream) stream.isStreaming = false
          if (currentMessage) currentMessage.content = finalContent
          const done = streamDoneData(event)
          applyDoneMessage(scopeType, targetId, aiMsgId, done, finalContent)
          delete activeStreams.value[key]
          clearResumableStream(key)
        } else if (event.type === 'error') {
          delete activeStreams.value[key]
          clearResumableStream(key)
          clearStreamingMessage(scopeType, targetId, aiMsgId)
          notifyError(streamErrorData(event)?.message || 'AI 响应出错')
        }
      }
    } catch (err) {
      const currentMessage = messageStoreByType(scopeType)[targetId]?.find((item) => item.id === aiMsgId)
      delete activeStreams.value[key]
      clearResumableStream(key)
      const message = err instanceof Error ? err.message : '请求 AI 失败'
      if (currentMessage) clearStreamingMessage(scopeType, targetId, aiMsgId)
      notifyError(message)
    }
  }

  async function stopActiveStream() {
    const stream = activeStream.value
    if (!stream || !authToken.value) return
    const { type, entityId, id } = stream
    try {
      await cancelChatStreamApi(type, entityId)
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '停止 AI 回复失败')
    } finally {
      delete activeStreams.value[sessionKey(type, entityId)]
      clearResumableStream(sessionKey(type, entityId))
      clearStreamingMessage(type, entityId, id)
    }
  }

  // ---- 选项点击 ----
  function selectPlanOption(optionId: string) {
    const msgs = activeMessages.value
    const lastMessage = msgs[msgs.length - 1]
    if (!lastMessage?.planOptions) return
    const option = lastMessage.planOptions.find((p) => p.id === optionId)
    if (!option) return
    if (option.custom) return
    sendMessage(`选择：${option.title}`)
  }

  async function joinChapterDraft(messageId: string) {
    if (!selectedChapterId.value) return
    const message = activeMessages.value.find((item) => item.id === messageId)
    const draftId = message?.chapterDraft?.draftId || message?.draftId
    if (!draftId) {
      notifyError('正文草稿尚未入库，请稍后再试')
      return
    }
    savingDraftMessageId.value = messageId
    try {
      const draft = toChapterContentDraft(await joinChapterDraftApi(selectedChapterId.value, draftId))
      upsertEditorDraft(draft)
      editorDraftId.value = draft.id
      openEditorMode(selectedChapterId.value, draft.id)
      notifyInfo('已加入草稿')
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '加入正文草稿失败')
    } finally {
      savingDraftMessageId.value = null
    }
  }

  async function loadChapterDrafts(chapterId = editorChapterId.value || selectedChapterId.value || 0) {
    if (!authToken.value || !chapterId) return
    try {
      editorDrafts.value[chapterId] = (await listChapterDraftsApi(chapterId)).map(toChapterContentDraft)
      if (editorDraftId.value && !editorDrafts.value[chapterId]?.some((draft) => draft.id === editorDraftId.value)) {
        editorDraftId.value = null
      }
      if (!editorDraftId.value && editorDrafts.value[chapterId]?.length) {
        editorDraftId.value = editorDrafts.value[chapterId][0].id
        persistViewMode()
      }
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '获取正文草稿失败')
    }
  }

  function selectEditorDraft(draftId: number) {
    editorDraftId.value = draftId
    editorSaveStatus.value = 'saved'
    persistViewMode()
  }

  async function saveEditorDraft(content: string) {
    if (!authToken.value || !editorChapterId.value || !editorDraftId.value) return
    const current = currentEditorDraft.value
    if (!current) return
    editorSaveStatus.value = 'saving'
    try {
      await updateChapterDraftApi(editorChapterId.value, editorDraftId.value, content)
      const draft: ChapterContentDraft = {
        ...current,
        content,
        wordCount: countTextWords(content),
        updatedAt: new Date(),
      }
      upsertEditorDraft(draft)
      editorDraftId.value = draft.id
      editorSaveStatus.value = 'saved'
      persistViewMode()
    } catch (err) {
      editorSaveStatus.value = 'error'
      notifyError(err instanceof Error ? err.message : '保存正文草稿失败')
    }
  }

  async function saveEditorDraftName(draftName: string, content?: string) {
    if (!authToken.value || !editorChapterId.value || !editorDraftId.value || !currentEditorDraft.value) return
    const current = currentEditorDraft.value
    const name = draftName.trim()
    if (!name) {
      notifyError('草稿名称不能为空')
      return
    }
    editorSaveStatus.value = 'saving'
    try {
      const nextContent = content ?? current.content
      await updateChapterDraftApi(editorChapterId.value, editorDraftId.value, nextContent, name)
      const draft: ChapterContentDraft = {
        ...current,
        draftName: name,
        content: nextContent,
        wordCount: countTextWords(nextContent),
        updatedAt: new Date(),
      }
      upsertEditorDraft(draft)
      editorDraftId.value = draft.id
      editorSaveStatus.value = 'saved'
      persistViewMode()
    } catch (err) {
      editorSaveStatus.value = 'error'
      notifyError(err instanceof Error ? err.message : '保存草稿名称失败')
    }
  }

  async function applyEditorDraft() {
    if (!authToken.value || !editorChapterId.value || !editorDraftId.value) return
    const current = currentEditorDraft.value
    if (!current) return
    editorSaveStatus.value = 'saving'
    try {
      await useChapterDraftApi(editorChapterId.value, editorDraftId.value)
      const now = new Date()
      const list = editorDrafts.value[editorChapterId.value] || []
      editorDrafts.value[editorChapterId.value] = list.map((item) => ({
        ...item,
        status: item.id === current.id ? CHAPTER_DRAFT_STATUS_CURRENT : item.status === CHAPTER_DRAFT_STATUS_CURRENT ? CHAPTER_DRAFT_STATUS_NORMAL : item.status,
        usedAt: item.id === current.id ? now : item.usedAt,
        updatedAt: item.id === current.id || item.status === CHAPTER_DRAFT_STATUS_CURRENT ? now : item.updatedAt,
      }))
      const draft = editorDrafts.value[editorChapterId.value].find((item) => item.id === current.id) || {
        ...current,
        status: CHAPTER_DRAFT_STATUS_CURRENT,
        usedAt: now,
        updatedAt: now,
      }
      upsertEditorDraft(draft)
      editorDraftId.value = draft.id
      editorSaveStatus.value = 'saved'
      persistViewMode()
      applyDraftToLocalChapter(draft)
      notifyInfo('正文已应用')
    } catch (err) {
      editorSaveStatus.value = 'error'
      notifyError(err instanceof Error ? err.message : '应用正文失败')
    }
  }

  async function deleteEditorDraft(draftId: number) {
    if (!authToken.value || !editorChapterId.value) return
    const chapterId = editorChapterId.value
    try {
      await deleteChapterDraftApi(chapterId, draftId)
      const list = editorDrafts.value[chapterId] || []
      editorDrafts.value[chapterId] = list.filter((draft) => draft.id !== draftId)
      if (editorDraftId.value === draftId) {
        editorDraftId.value = editorDrafts.value[chapterId][0]?.id || null
        persistViewMode()
      }
      editorSaveStatus.value = editorDraftId.value ? 'saved' : 'idle'
      notifyInfo('草稿已删除')
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '删除正文草稿失败')
    }
  }

  function applyDraftToLocalChapter(draft: ChapterContentDraft) {
    const located = findChapterById(draft.chapterId)
    if (!located) return
    const previousWordCount = located.chapter.wordCount || 0
    const delta = draft.wordCount - previousWordCount
    const usedAt = draft.usedAt || new Date()
    const patchedChapter: Chapter = {
      ...located.chapter,
      content: draft.content,
      status: 2,
      wordCount: draft.wordCount,
      completedAt: usedAt,
    }
    located.volume.chapters = located.volume.chapters.map((chapter) =>
      chapter.id === draft.chapterId ? patchedChapter : chapter
    )
    if (delta === 0) return
    located.volume.wordCount = Math.max(0, (located.volume.wordCount || 0) + delta)
    located.novel.wordCount = Math.max(0, (located.novel.wordCount || 0) + delta)
    dashboard.value = null
    delete novelOverviews.value[located.novel.id]
  }

  function findChapterById(chapterId: number) {
    for (const novel of novels.value) {
      for (const volume of novel.volumes) {
        const chapter = volume.chapters.find((item) => item.id === chapterId)
        if (chapter) return { novel, volume, chapter }
      }
    }
    return null
  }

  function upsertEditorDraft(draft: ChapterContentDraft) {
    const list = editorDrafts.value[draft.chapterId] || []
    const index = list.findIndex((item) => item.id === draft.id)
    if (index >= 0) {
      list.splice(index, 1, draft)
    } else {
      list.unshift(draft)
    }
    editorDrafts.value[draft.chapterId] = [...list]
  }


  // ---- 认证 ----
  function applyAuth(user: AuthUser) {
    authToken.value = 'cookie'
    isLoggedIn.value = true
    authUsername.value = user.username
    accountSettings.value = {
      ...accountSettings.value,
      username: user.username,
      email: user.email || accountSettings.value.email,
    }
	void loadSettings()
	void loadModels()
	void loadNovels()
}

  async function login(username: string, password: string, turnstileToken = '') {
    const response = await loginApi(username, password, turnstileToken)
    applyAuth(response.user)
  }

  async function loginByCode(email: string, code: string, turnstileToken = '') {
    const response = await loginByCodeApi(email, code, turnstileToken)
    applyAuth(response.user)
  }

  async function register(username: string, email: string, password: string, code: string, turnstileToken = '') {
    const response = await registerApi(username, email, password, code, turnstileToken)
    applyAuth(response.user)
  }

  async function initializeAuth() {
    restoreViewModeSnapshot()
    try {
      const response = await meApi()
      applyAuth(response.user)
    } catch {
      clearAuthState()
    }
  }

  function openAuthModal(mode: 'login' | 'register' = 'login') {
    authModalMode.value = mode
    isAuthModalOpen.value = true
  }

  function closeAuthModal() {
    isAuthModalOpen.value = false
  }

  function setAuthModalMode(mode: 'login' | 'register') {
    authModalMode.value = mode
  }

  async function logout() {
    try {
      await logoutApi()
    } catch {
      // 登出失败也清本地状态
    }
    clearAuthState()
  }

  function requestLogout() {
    isLogoutConfirmOpen.value = true
  }

  function cancelLogout() {
    isLogoutConfirmOpen.value = false
  }

  async function confirmLogout() {
    isLogoutConfirmOpen.value = false
    await logout()
    notifyInfo('已退出登录')
  }

  function clearAuthState() {
    isLoggedIn.value = false
    authUsername.value = ''
    authToken.value = ''
    selectedNovelId.value = null
    selectedVolumeId.value = null
    selectedChapterId.value = null
    novels.value = []
    models.value = []
    archivedNovels.value = []
    novelOverviews.value = {}
    dashboard.value = null
    novelMessages.value = {}
    volumeMessages.value = {}
    chapterMessages.value = {}
    chatSessionMeta.value = {}
    activeStreams.value = {}
    resumableStreamKeys.value = new Set()
    stages.value = {}
    editorDrafts.value = {}
    editorDraftId.value = null
    editorChapterId.value = null
    editorSaveStatus.value = 'idle'
    personalizationSettings.value = {
      themeMode: 'light',
      language: '简体中文',
    }
    settingsError.value = ''
    settingsUpdatedAt.value = ''
    localStorage.removeItem(SELECTION_KEY)
    localStorage.removeItem(VIEW_MODE_KEY)
    localStorage.removeItem(ACTIVE_STREAM_KEYS_KEY)
  }

  // ---- 新建小说 ----
  function openWorkspaceHome() {
    if (blockSetupLeaveWhileGenerating()) return
    isNovelSetupChoiceOpen.value = true
    isNovelSetupOpen.value = false
    isVolumesCollapsed.value = true
    selectedNovelId.value = null
    selectedVolumeId.value = null
    selectedChapterId.value = null
    editorChapterId.value = null
    editorDraftId.value = null
    viewMode.value = 'chat'
    pendingSelection.value = null
    persistSelection()
    persistViewMode()
    void loadDashboard()
  }

  function beginNovelSetup() {
    if (!authToken.value) {
      openAuthModal('login')
      return
    }
    openWorkspaceHome()
  }

  function beginNovelSetupForm() {
    beginNovelSetup()
    openNovelSetupForm()
  }

  function openNovelSetupForm() {
    isNovelSetupChoiceOpen.value = false
    isNovelSetupOpen.value = true
    setupDraftNovelId.value = null
    novelSetupFormResetTick.value += 1
  }

  function cancelNovelSetup() {
    if (blockSetupLeaveWhileGenerating()) return
    if (novelSetupDirty.value) {
      unsavedSetupLeaveAction.value = () => {
        isNovelSetupChoiceOpen.value = true
        isNovelSetupOpen.value = false
        setupDraftNovelId.value = null
        novelSetupDirty.value = false
        void loadDashboard()
      }
      isUnsavedSetupLeaveOpen.value = true
      return
    }
    isNovelSetupChoiceOpen.value = true
    isNovelSetupOpen.value = false
    setupDraftNovelId.value = null
    void loadDashboard()
  }

  function closeUnsavedSetupLeave() {
    isUnsavedSetupLeaveOpen.value = false
    unsavedSetupLeaveAction.value = null
  }

  function executeStashedLeaveAction() {
    const action = unsavedSetupLeaveAction.value
    isUnsavedSetupLeaveOpen.value = false
    unsavedSetupLeaveAction.value = null
    novelSetupDirty.value = false
    if (action) action()
  }

  function confirmLeaveWithoutSaving() {
    executeStashedLeaveAction()
  }

  async function createNovel(setupData?: NovelSetupData) {
    if (!authToken.value) {
      openAuthModal('login')
      return undefined
    }

    isNovelCreating.value = true
    try {
      const response =
        setupDraftNovelId.value && setupData
          ? await startNovelSetupDraftApi(setupDraftNovelId.value, setupData)
          : await createNovelApi(setupData)
      const novel = toNovel(response.novel)

      novels.value = [
        novel,
        ...novels.value.filter((item) => item.id !== novel.id),
      ]
      dashboard.value = null
      delete novelOverviews.value[novel.id]
      selectedNovelId.value = novel.id
      selectedVolumeId.value = null
      selectedChapterId.value = null
      isVolumesCollapsed.value = false
      pendingSelection.value = null
      persistSelection()
      stages.value[novel.id] = 'awaiting_overview'
      novelMessages.value[novel.id] = response.message
        ? [toMessage(response.message)]
        : []
      isNovelSetupOpen.value = false
      isNovelSetupChoiceOpen.value = false
      setupDraftNovelId.value = null
      return novel.id
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '新建小说失败')
      return undefined
    } finally {
      isNovelCreating.value = false
    }
  }

  async function createNovelFromSetup(content: string, setupData?: NovelSetupData) {
    const text = content.trim()
    if (!text) {
      notifyError('请先填写小说信息')
      return
    }
    const novelId = await createNovel(setupData)
    if (!novelId) return
    await sendMessage(text)
  }

  // ---- 编辑模式 ----
  function openEditorMode(chapterId: number, draftId?: number) {
    viewMode.value = 'editor'
    editorChapterId.value = chapterId
    if (draftId) editorDraftId.value = draftId
    isVolumesCollapsed.value = false
    persistSelection()
    persistViewMode()
    selectChapter(chapterId)
  }

  function switchToEditorMode() {
    viewMode.value = 'editor'
    if (!editorChapterId.value && selectedChapterId.value) {
      editorChapterId.value = selectedChapterId.value
      void loadChapterDrafts(selectedChapterId.value)
    }
    persistViewMode()
  }

  function switchToChatMode() {
    viewMode.value = 'chat'
    editorChapterId.value = null
    editorDraftId.value = null
    persistViewMode()
    if (selectedChapterId.value) {
      void loadMessages('chapter', selectedChapterId.value)
    } else if (selectedVolumeId.value) {
      void loadMessages('volume', selectedVolumeId.value)
    } else if (selectedNovelId.value) {
      void loadMessages('novel', selectedNovelId.value)
    }
  }

  // ---- 梗概 / 归档 / 编辑 ----
  function openOverview(id: number) {
    overviewNovelId.value = id
    void loadNovelOverview(id)
  }

  function closeOverview() {
    overviewNovelId.value = null
  }

  function openArchiveConfirm(id: number) {
    archiveTargetId.value = id
  }

  function closeArchiveConfirm() {
    archiveTargetId.value = null
  }

  function confirmArchive() {
    const id = archiveTargetId.value
    if (!id) return
    const novel = novels.value.find((n) => n.id === id)
    if (!novel) return
    void archiveNovel(id)
  }
  async function archiveNovel(id: number) {
    const novel = novels.value.find((n) => n.id === id)
    if (!novel) return
    try {
      await archiveNovelApi(id)
      const archived: Novel = {
        ...novel,
        status: NOVEL_STATUS_ARCHIVED,
        updatedAt: new Date(),
      }
      novels.value = novels.value.filter((n) => n.id !== id)
      archivedNovels.value = [archived, ...archivedNovels.value.filter((n) => n.id !== id)]
      delete novelOverviews.value[id]
      dashboard.value = null
      if (selectedNovelId.value === id) {
        selectedNovelId.value = null
        selectedVolumeId.value = null
        selectedChapterId.value = null
        isVolumesCollapsed.value = true
        persistSelection()
      }
      archiveTargetId.value = null
      notifyInfo('小说已归档')
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '归档小说失败')
    }
  }
  async function restoreNovel(id: number) {
    const novel = archivedNovels.value.find((n) => n.id === id)
    if (!novel) return
    try {
      await restoreNovelApi(id)
      const next: Novel = {
        ...novel,
        status: NOVEL_STATUS_NORMAL,
        updatedAt: new Date(),
      }
      archivedNovels.value = archivedNovels.value.filter((n) => n.id !== id)
      novels.value.unshift(next)
      delete novelOverviews.value[id]
      dashboard.value = null
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '恢复小说失败')
    }
  }
  function openVolumeChapterOverview(type: 'volume' | 'chapter', id: number, title: string, createdAt: Date) {
    overviewVolumeChapter.value = { type, id, title, createdAt }
  }

  function closeVolumeChapterOverview() {
    overviewVolumeChapter.value = null
  }

  function openShare(type: 'novel' | 'volume' | 'chapter', id: number, title: string, description: string) {
    shareTarget.value = { type, id, title, description }
    shareLink.value = ''
    shareError.value = ''
    void createShareLink()
  }
  function closeShare() {
    shareTarget.value = null
    shareLink.value = ''
    shareError.value = ''
  }

  async function createShareLink(password = generalSettings.value.shareSecurityKey) {
    if (!authToken.value || !shareTarget.value) return
    isShareCreating.value = true
    shareError.value = ''
    try {
      const response = await createShareLinkApi({
        type: shareTarget.value.type,
        id: shareTarget.value.id,
        password,
      })
      shareLink.value = browserShareURL(response.url)
    } catch (err) {
      shareError.value = err instanceof Error ? err.message : '创建分享链接失败'
    } finally {
      isShareCreating.value = false
    }
  }

  function browserShareURL(rawURL: string) {
    try {
      const url = new URL(rawURL, window.location.origin)
      return `${window.location.origin}${url.pathname}${url.search}${url.hash}`
    } catch {
      return rawURL
    }
  }

  async function startDownload(type: 'novel' | 'volume' | 'chapter', id: number) {
    if (!authToken.value) {
      openAuthModal('login')
      return
    }
    downloadError.value = ''
    try {
      downloadJob.value = await createDownloadApi({
        type,
        id,
        format: generalSettings.value.downloadFormat,
        layout: generalSettings.value.downloadLayout,
      })
      await loadDownloadJobs()
      notifyInfo('下载任务已创建，请到「更多」里的下载任务查看')
      await pollDownload(downloadJob.value.id)
    } catch (err) {
      downloadError.value = err instanceof Error ? err.message : '创建下载任务失败'
    }
  }

  async function exportAllData() {
    if (!authToken.value) {
      openAuthModal('login')
      return
    }
    downloadError.value = ''
    try {
      downloadJob.value = await createDownloadApi({
        type: 'all',
        format: generalSettings.value.downloadFormat,
        layout: generalSettings.value.downloadLayout,
      })
      await loadDownloadJobs()
      await pollDownload(downloadJob.value.id)
    } catch (err) {
      downloadError.value = err instanceof Error ? err.message : '创建导出任务失败'
      notifyError(downloadError.value)
    }
  }

  async function pollDownload(jobId: string) {
    for (let i = 0; i < 120; i++) {
      const job = await getDownloadApi(jobId)
      downloadJob.value = job
      upsertDownloadJob(job)
      if (job.status === 'done') {
        await loadDownloadJobs()
        await saveDownloadFile(job.id)
        return
      }
      if (job.status === 'error') {
        downloadError.value = job.message || '生成下载文件失败'
        await loadDownloadJobs()
        return
      }
      await new Promise((resolve) => setTimeout(resolve, 500))
    }
    downloadError.value = '下载任务超时'
  }

  async function loadDownloadJobs() {
    if (!authToken.value) return
    try {
      downloadJobs.value = await listDownloadsApi()
      downloadJob.value = downloadJobs.value[0] || downloadJob.value
    } catch (err) {
      downloadError.value = err instanceof Error ? err.message : '获取下载任务失败'
      notifyError(downloadError.value)
    }
  }

  async function deleteAccount(password: string) {
    try {
      await deleteAccountApi(password)
      clearAuthState()
      notifyInfo('账号已进入 7 天注销冷静期')
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '注销用户失败')
      throw err
    }
  }

  async function saveNovelSetupDraft(setupData: NovelSetupData) {
    if (!authToken.value) {
      openAuthModal('login')
      return undefined
    }
    isNovelCreating.value = true
    try {
      let novel: Novel
      if (setupDraftNovelId.value) {
        const existing = novels.value.find((item) => item.id === setupDraftNovelId.value)
        if (!existing) throw new Error('暂存草稿不存在，请刷新后重试')
        await updateNovelSetupDraftApi(setupDraftNovelId.value, setupData)
        const planData = setupPlanData(setupData)
        novel = {
          ...existing,
          title: setupData.title.trim() || overviewTitle(planData, '未命名小说'),
          planData,
          setupOriginalText: setupData.originalText?.trim() || '',
          status: NOVEL_STATUS_SETUP,
          updatedAt: new Date(),
        }
      } else {
        novel = toNovel(await saveNovelSetupDraftApi(setupData))
      }
      novels.value = [novel, ...novels.value.filter((item) => item.id !== novel.id)]
      novelOverviews.value = {
        ...novelOverviews.value,
        [novel.id]: {
          id: novel.id,
          title: novel.title,
          planData: novel.planData,
          setupOriginalText: novel.setupOriginalText || '',
          wordCount: novel.wordCount,
          updatedAt: novel.updatedAt,
        },
      }
      selectedNovelId.value = novel.id
      selectedVolumeId.value = null
      selectedChapterId.value = null
      setupDraftNovelId.value = novel.id
      isNovelSetupChoiceOpen.value = false
      isNovelSetupOpen.value = true
      isVolumesCollapsed.value = true
      pendingSelection.value = null
      persistSelection()
      notifyInfo('已保存草稿')
      return novel.id
    } catch (err) {
      notifyError(err instanceof Error ? err.message : '暂存失败')
      return undefined
    } finally {
      isNovelCreating.value = false
    }
  }

  function upsertDownloadJob(job: DownloadJob) {
    const existing = downloadJobs.value.findIndex((item) => item.id === job.id)
    if (existing >= 0) {
      downloadJobs.value.splice(existing, 1, job)
    } else {
      downloadJobs.value.unshift(job)
    }
  }

  async function saveDownloadFile(jobId: string) {
    const { blob, filename } = await downloadFileApi(jobId)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    a.style.display = 'none'
    document.body.appendChild(a)
    a.click()
    a.remove()
    setTimeout(() => URL.revokeObjectURL(url), 30000)
  }

  // ---- 设置 ----
  function openSettings(tab: SettingsTab = 'general') {
    settingsTab.value = tab
    isSettingsOpen.value = true
    settingsSnapshot = JSON.stringify(settingsPayload())
    if (isLoggedIn.value && models.value.length === 0) {
      void loadModels()
    }
    if (isLoggedIn.value && !hasLoadedRemoteSettings.value) {
      void loadSettings()
    }
  }

  function openCustomModelSettings() {
    customModelRequestTick.value += 1
    openSettings('general')
  }

  function openShareSecuritySettings() {
    shareSecurityRequestTick.value += 1
    openSettings('general')
  }

  function closeSettings() {
    isSettingsOpen.value = false
    if (JSON.stringify(settingsPayload()) !== settingsSnapshot) {
      void saveSettings()
    }
  }

  return {
    isSettingsOpen, settingsTab, isVolumesCollapsed, isNovelSetupChoiceOpen, isNovelSetupOpen, novelSetupFormResetTick, isLogoutConfirmOpen, isUnsavedSetupLeaveOpen, unsavedSetupLeaveAction, novelSetupDirty, novelSetupGenerating,
    selectedNovelId, selectedVolumeId, selectedChapterId, setupDraftNovelId,
    novels, models, selectedModel, isCustomModelSelected, officialModel,
    isModelsLoading, isModelCreating, modelError,
    isNovelCreating, archivedNovels, novelOverviews, dashboard, isDashboardLoading,
    isNovelsLoading, isVolumesLoading, isChaptersLoading, isMessagesLoading, savingDraftMessageId, applyingPlanMessageId,
    activeMessages, activeStream, activeChatPlaceholder, isSessionStreaming, isNovelOrChildStreaming,
    novelMessages, volumeMessages, chapterMessages, chatSessionMeta, stages,
    chatBreadcrumb, editorBreadcrumb,
    overviewNovelId, archiveTargetId, overviewVolumeChapter, shareTarget, shareLink, isShareCreating, shareError,
    downloadJob, downloadJobs, downloadError, errorToast, toastKind, customModelRequestTick, shareSecurityRequestTick,
    generalSettings, notificationSettings, personalizationSettings, accountSettings,
    isSettingsLoading, isSettingsSaving, settingsError, settingsUpdatedAt,
    viewMode, editorChapterId, editorChapter, editorDrafts, editorDraftId, editorSaveStatus, currentEditorDrafts, currentEditorDraft,
    selectedNovel, selectedVolume, selectedChapter,
    volumeTitle, chapterTitle, volumeDisplayTitle, chapterDisplayTitle,
    openSettings, openCustomModelSettings, openShareSecuritySettings, closeSettings, loadSettings, saveSettings, loadModels, createCustomModel, deleteCustomModel, testCustomModel,
    loadNovels, loadNovelOverview, loadDashboard, loadArchivedNovels, loadVolumes, loadChapters,
    selectNovel, selectVolume, selectChapter, toggleVolumeExpanded, setVolumesCollapsed,
    openWorkspaceHome, beginNovelSetup, beginNovelSetupForm, openNovelSetupForm, cancelNovelSetup, closeUnsavedSetupLeave, confirmLeaveWithoutSaving, executeStashedLeaveAction, createNovel, createNovelFromSetup, saveNovelSetupDraft, sendMessage, stopActiveStream, selectPlanOption, applyGeneratedPlan, joinChapterDraft,
    loadChapterDrafts, selectEditorDraft, upsertEditorDraft, saveEditorDraft, saveEditorDraftName, applyEditorDraft, deleteEditorDraft,
    openEditorMode, switchToEditorMode, switchToChatMode,
    openOverview, closeOverview,
    openArchiveConfirm, closeArchiveConfirm, confirmArchive, restoreNovel,
    openVolumeChapterOverview, closeVolumeChapterOverview,
    openShare, closeShare, createShareLink,
    startDownload, exportAllData, loadDownloadJobs, deleteAccount, notifyError, notifyInfo, dismissToast,
    isLoggedIn, authUsername, authToken, isAuthModalOpen, authModalMode, login, loginByCode, register, initializeAuth, logout, requestLogout, cancelLogout, confirmLogout, openAuthModal, closeAuthModal, setAuthModalMode,
  }
})
