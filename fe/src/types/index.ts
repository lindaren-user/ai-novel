export type PlanValue = string | number | boolean | unknown[] | object | null | undefined

export interface PlanData {
  title?: string
  summary?: string
  timeline?: string
  locations?: string[]
  characters?: string[]
  character_settings?: string[]
  character_plan?: string
  key_selling_points?: string[]
  risk_control?: string
  genre?: string
  protagonist?: string
  world?: string
  main_goal?: string
  core_conflict?: string
  tone?: string
  style?: string
  character_development?: string
  setting_development?: string
  setting_boundaries?: SettingBoundary[]
  current_state?: string
  end_state?: string
  key_events?: string[]
  references?: string[]
  intertextual_links?: string
  foreshadowing?: string
  other_highlights?: string
  temporary_settings?: NovelSetupData
  [key: string]: PlanValue
}

export interface SettingBoundary {
  name?: string
  state_before?: string
  state_after?: string
  allowed_progress?: string
  forbidden_progress?: string
}

export type OverviewData = PlanData

export interface NovelSetupData {
  originalText?: string
  title: string
  direction: string
  tagGroups?: Record<string, string[]>
  characters?: Array<{
    name: string
    appearanceTime?: string
    notes?: string
  }>
  relationships?: Array<{
    characterA: string
    characterB: string
    description?: string
  }>
  maps?: Array<{
    name: string
    appearanceTime?: string
    notes?: string
  }>
  forces?: Array<{
    name: string
    appearanceTime?: string
    notes?: string
  }>
  other_settings?: Array<{
    title: string
    description?: string
    items: Array<{
      name: string
      notes?: string
      appearanceTime?: string
    }>
  }>
  perspective?: string
  length: string
  lengthRange: string
}

// 小说相关类型
export interface Novel {
  id: number
  title: string
  planData: PlanData
  setupOriginalText?: string
  status: number
  cover?: string
  wordCount: number
  createdAt: Date
  updatedAt: Date
  volumes: Volume[]
}

export interface NovelOverviewItem {
  id: number
  title: string
  planData: PlanData
  setupOriginalText?: string
  wordCount: number
  updatedAt: Date
}

export interface WorkspaceDashboard {
  totalWords: number
  completedChapters: number
  volumeCount: number
  writingHours: number
  lastEditedAt: Date
  wordTrend: WordTrendPoint[]
}

export interface WordTrendPoint {
  date: string
  weekday: string
  words: number
  wordLabel: string
}

export interface Volume {
  id: number
  novelId: number
  planData: PlanData
  chapterCount: number
  wordCount: number
  createdAt: Date
  chapters: Chapter[]
  expanded?: boolean
}

export interface Chapter {
  id: number
  volumeId: number
  planData: PlanData
  content?: string
  status: number
  wordCount: number
  createdAt: Date
  completedAt?: Date | null
}

// 消息相关类型
export interface Message {
  id: string
  draftId?: number
  role: 'user' | 'assistant'
  content: string
  timestamp: Date
  temporary?: boolean
  optimistic?: boolean
  isThinking?: boolean
  lastTextAt?: Date
  planOptions?: PlanOption[]
  planOptionsPlaceholder?: boolean
  chapterGeneration?: ChapterGenerationProgress
  chapterDraft?: ChapterDraft
}

export interface PlanOption {
  id: string
  title: string
  description: string
  optionType?: string
  custom?: boolean
  details?: Record<string, unknown>
}

export interface ChapterDraft {
  draftId?: number
  title: string
  content: string
  revisionNotes: string
}

export interface ChapterGenerationProgress {
  stage: string
  text: string
  attempt: number
  steps: string[]
  preview: string
  issues: string[]
  stepOutputs: ChapterGenerationStepOutput[]
  currentStepLabel: string
  currentStepStartedAt?: Date
  stepTimings: ChapterGenerationStepTiming[]
  complete: boolean
  collapsed: boolean
  failed: boolean
}

export interface ChapterGenerationStepTiming {
  key: string
  label: string
  startedAt?: Date
  endedAt?: Date
}

export interface ChapterGenerationStepOutput {
  step: string
  attempt: number
  type: string
  text: string
  items: string[]
}

export interface ChapterContentDraft {
  id: number
  chapterId: number
  sourceMessageId: number
  draftType: number
  originDraftId: number
  draftName: string
  content: string
  status: number
  wordCount: number
  usedAt?: Date | null
  createdAt: Date
  updatedAt: Date
}

// 设置相关类型
export interface GeneralSettings {
  consistencyCheckCount: number
  autoSave: string
  modelId: number
  model: string
  customProvider: string
  customModelId: string
  customApiUrl: string
  customApiKey: string
  downloadFormat: string
  downloadLayout: 'volume' | 'chapter'
  shareSecurityKey: string
}

export interface NotificationSettings {
  desktopNotification: boolean
  soundAlert: boolean
  notificationContent: {
    newMessage: boolean
    comment: boolean
    like: boolean
    system: boolean
  }
  doNotDisturb: boolean
  doNotDisturbStart: string
  doNotDisturbEnd: string
}

export interface PersonalizationSettings {
  themeMode: 'system' | 'light' | 'dark'
  language: string
}

export interface AccountSettings {
  avatar: string
  email: string
  username: string
  language: string
}

export type SettingsTab = 'general' | 'notification' | 'personalization' | 'account'
