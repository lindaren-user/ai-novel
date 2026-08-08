import type {
  AccountSettings,
  GeneralSettings,
  Message,
  NotificationSettings,
  Novel,
  PersonalizationSettings,
  ChapterDraft,
  ChapterContentDraft,
  NovelSetupData,
} from '@/types'

export interface AuthUser {
  id: string
  username: string
  email: string
  status: number
  createdAt: string
  updatedAt: string
}

export interface AuthResponse {
  token?: string
  user: AuthUser
}

export interface AppSettingsPayload {
  general: GeneralSettings
  notification: NotificationSettings
  personalization: PersonalizationSettings
  account: AccountSettings
}

export interface SettingsResponse {
  settings: Partial<AppSettingsPayload>
  updatedAt: string
}

export interface CreateNovelResponse {
  novel: Omit<Novel, 'id' | 'createdAt' | 'updatedAt' | 'volumes'> & {
    id: number
    userId: number
    title: string
    planData: Record<string, unknown>
    status: number
    wordCount: number
    createdAt: string
    updatedAt: string
  }
  message?: Omit<Message, 'id' | 'timestamp'> & {
    id: number
    sessionId: number
    renderData?: RenderData
    createdAt: string
  }
}

export interface ApiNovel {
  id: number
  userId: number
  title: string
  planData: Record<string, unknown>
  status: number
  wordCount: number
  createdAt: string
  updatedAt: string
}

export interface ApiNovelOverview {
  id: number
  title: string
  planData: Record<string, unknown>
  setupOriginalText?: string
  wordCount: number
  updatedAt: string
}

export interface ApiWorkspaceDashboard {
  totalWords: number
  completedChapters: number
  volumeCount: number
  writingHours: number
  lastEditedAt: string
  wordTrend: ApiWordTrendPoint[]
}

export interface ApiWordTrendPoint {
  date: string
  weekday: string
  words: number
  wordLabel: string
}

export interface ApiVolume {
  id: number
  novelId: number
  planData: Record<string, unknown>
  sortOrder: number
  status: number
  wordCount: number
  chapterCount: number
  createdAt: string
  updatedAt: string
}

export interface ApiChapter {
  id: number
  volumeId: number
  planData: Record<string, unknown>
  content: string
  sortOrder: number
  status: number
  wordCount: number
  createdAt: string
  updatedAt: string
  completedAt: string | null
}

export interface CreateSharePayload {
  type: 'novel' | 'volume' | 'chapter'
  id: number
  password?: string
}

export interface CreateShareResponse {
  url: string
  type: 'novel' | 'volume' | 'chapter'
  requiresPassword: boolean
}

export interface ApiSharedChapter {
  id: number
  volumeId: number
  planData: Record<string, unknown>
  content: string
  sortOrder: number
  wordCount: number
  updatedAt: string
  completedAt: string | null
}

export interface ApiSharedVolume {
  id: number
  novelId: number
  planData: Record<string, unknown>
  sortOrder: number
  chapters: ApiSharedChapter[]
}

export interface ApiSharedNovel {
  id: number
  title: string
  planData: Record<string, unknown>
  volumes: ApiSharedVolume[]
}

export interface ApiSharedContent {
  type: 'novel' | 'volume' | 'chapter'
  requiresPassword: boolean
  novel: ApiSharedNovel
  selectedVolumeId: number
  selectedChapterId: number
}

export interface DownloadJob {
  id: string
  status: 'pending' | 'running' | 'done' | 'error'
  progress: number
  message: string
  filename: string
}

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'
export const API_ERROR_EVENT = 'ai-novel-api-error'
export const API_CODE_OK = 0
let refreshPromise: Promise<boolean> | null = null

export class ApiRequestError extends Error {
  code: number
  status: number

  constructor(message: string, code: number, status: number) {
    super(message)
    this.name = 'ApiRequestError'
    this.code = code
    this.status = status
  }
}

type ApiRequestInit = RequestInit & {
  silentError?: boolean
}

function normalizeErrorMessage(message: string): string {
  if (message.includes('invalid id checksum')) return 'ID 参数不正确，请刷新后重试'
  if (message === '响应格式不正确') return '服务响应异常，请稍后重试'
  if (message === '请求失败') return '网络请求失败，请检查后端服务是否正常'
  return message || '请求失败'
}

function emitApiError(message: string) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(API_ERROR_EVENT, {
    detail: { message: normalizeErrorMessage(message) },
  }))
}

async function request<T>(path: string, options: ApiRequestInit = {}): Promise<T> {
  return requestWithRefresh<T>(path, options, true)
}

async function requestWithRefresh<T>(path: string, options: ApiRequestInit, allowRefresh: boolean): Promise<T> {
  try {
    const response = await fetchJson(path, options)

    const body = await response.json().catch(() => ({ code: -1, msg: response.ok ? '服务响应异常' : '请求失败', data: null }))
    if (allowRefresh && response.status === 401 && shouldRefreshAuth(path)) {
      const refreshed = await refreshAuthSession()
      if (refreshed) return requestWithRefresh<T>(path, options, false)
    }
    if (!response.ok || body.code !== API_CODE_OK) {
      const message = normalizeErrorMessage(body.msg || '请求失败')
      throw new ApiRequestError(message, Number(body.code) || -1, response.status)
    }
    return body.data as T
  } catch (err) {
    if (
      (err instanceof DOMException && err.name === 'AbortError') ||
      (err instanceof Error && err.name === 'AbortError')
    ) {
      throw err
    }
    if (err instanceof Error) {
      const message = normalizeErrorMessage(err.message)
      if (!options.silentError) emitApiError(message)
      if (err instanceof ApiRequestError) {
        err.message = message
        throw err
      }
      throw new Error(message)
    }
    emitApiError('网络请求失败，请检查后端服务是否正常')
    throw err
  }
}

export function loginApi(username: string, password: string, turnstileToken = '') {
  return request<AuthResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password, mode: 'password', turnstileToken }),
  })
}

export function loginByCodeApi(email: string, code: string, turnstileToken = '') {
  return request<AuthResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, code, mode: 'code', turnstileToken }),
  })
}

export function registerApi(username: string, email: string, password: string, code: string, turnstileToken = '') {
  return request<AuthResponse>('/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, email, password, code, turnstileToken }),
  })
}

export function sendCodeApi(email: string) {
  return request<void>('/auth/send-code', {
    method: 'POST',
    body: JSON.stringify({ email }),
  })
}

export interface TurnstileConfig {
  siteKey: string
  enabled: boolean
}

export function getTurnstileConfigApi() {
  return request<TurnstileConfig>('/auth/turnstile-config')
}

export function logoutApi() {
  return request<void>('/auth/logout', {
    method: 'POST',
  })
}

export function changePasswordApi(oldPassword: string, newPassword: string) {
  return request<void>('/auth/change-password', {
    method: 'POST',
    body: JSON.stringify({ oldPassword, newPassword }),
  })
}

export function deleteAccountApi(password: string) {
  return request<void>('/auth/me', {
    method: 'DELETE',
    body: JSON.stringify({ password }),
  })
}

export function createFeedbackApi(content: string) {
  return createFeedbackWithImagesApi(content, [])
}

export function createFeedbackWithImagesApi(content: string, imageUrls: string[]) {
  return request<void>('/feedbacks', {
    method: 'POST',
    body: JSON.stringify({ content, imageUrls }),
  })
}

export async function uploadFileApi(file: File, type = 'feedback') {
  try {
    const token = await request<{
      key: string
      url: string
      uploadUrl: string
      method: string
      headers: Record<string, string>
      expiresAt: string
    }>('/files', {
      method: 'POST',
      body: JSON.stringify({
        type,
        filename: file.name,
        contentType: file.type,
        size: file.size,
      }),
    })
    const response = await fetch(token.uploadUrl, {
      method: token.method || 'PUT',
      headers: token.headers || {},
      body: file,
    })
    if (!response.ok) {
      throw new Error('上传失败')
    }
    return { key: token.key, url: token.url }
  } catch (err) {
    if (err instanceof Error) {
      const message = normalizeErrorMessage(err.message)
      emitApiError(message)
      throw new Error(message)
    }
    emitApiError('上传失败')
    throw err
  }
}

async function fetchJson(path: string, options: ApiRequestInit = {}) {
  await waitForActiveRefresh(path)
  const { silentError, ...fetchOptions } = options
  void silentError
  return fetch(`${API_BASE_URL}${path}`, {
    ...fetchOptions,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(fetchOptions.headers || {}),
    },
  })
}

function shouldRefreshAuth(path: string) {
  const target = authTargetPath(path)
  return !target.startsWith('/auth/login') &&
    !target.startsWith('/auth/register') &&
    !target.startsWith('/auth/refresh')
}

function authTargetPath(target: string) {
  if (target.startsWith(API_BASE_URL)) return target.slice(API_BASE_URL.length) || '/'
  try {
    const url = new URL(target, typeof window === 'undefined' ? 'http://localhost' : window.location.origin)
    const apiBase = API_BASE_URL.startsWith('http') ? new URL(API_BASE_URL).pathname : API_BASE_URL
    return url.pathname.startsWith(apiBase) ? url.pathname.slice(apiBase.length) || '/' : url.pathname
  } catch {
    return target
  }
}

async function waitForActiveRefresh(target: string) {
  if (!refreshPromise || !shouldRefreshAuth(target)) return
  const refreshed = await refreshPromise
  if (!refreshed) throw new ApiRequestError('登录已过期', 401, 401)
}

async function refreshAuthSession() {
  if (!refreshPromise) {
    refreshPromise = rawRefreshAuthSession().finally(() => {
      refreshPromise = null
    })
  }
  return refreshPromise
}

async function rawRefreshAuthSession() {
  try {
    const response = await fetchJson('/auth/refresh', { method: 'POST' })
    const body = await response.json().catch(() => ({ code: -1 }))
    return response.ok && body.code === API_CODE_OK
  } catch {
    return false
  }
}

async function fetchWithAuthRefresh(url: string, options: RequestInit, allowRefresh = true): Promise<Response> {
  await waitForActiveRefresh(url)
  const response = await fetch(url, {
    ...options,
    credentials: 'include',
  })
  if (allowRefresh && response.status === 401 && shouldRefreshAuth(url)) {
    const refreshed = await refreshAuthSession()
    if (refreshed) return fetchWithAuthRefresh(url, options, false)
  }
  return response
}

export function meApi() {
  return request<{ user: AuthUser }>('/auth/me', {
    silentError: true,
  })
}

export function getSettingsApi() {
  return request<SettingsResponse>('/settings')
}

export function updateSettingsApi(settings: AppSettingsPayload) {
  return request<void>('/settings', {
    method: 'PUT',
    body: JSON.stringify({ settings }),
  })
}

export function createNovelApi(setupData?: NovelSetupData) {
  return request<CreateNovelResponse>('/novels', {
    method: 'POST',
    body: JSON.stringify({ setupData }),
  })
}

export function saveNovelSetupDraftApi(setupData: NovelSetupData) {
  return request<ApiNovel>('/novels/setup/drafts', {
    method: 'POST',
    body: JSON.stringify({ setupData }),
  })
}

export function updateNovelSetupDraftApi(novelId: number, setupData: NovelSetupData) {
  return request<void>(`/novels/${novelId}/setup/draft`, {
    method: 'POST',
    body: JSON.stringify({ setupData }),
  })
}

export function startNovelSetupDraftApi(novelId: number, setupData: NovelSetupData) {
  return request<CreateNovelResponse>(`/novels/${novelId}/setup/start`, {
    method: 'POST',
    body: JSON.stringify({ setupData }),
  })
}

export function completeNovelSetupApi(payload: { text: string; files?: File[]; modelId?: number }, signal?: AbortSignal) {
  return completeNovelSetupStreamApi(payload, undefined, signal)
}

export function completeNovelSetupStreamApi(
  payload: { text: string; files?: File[]; modelId?: number },
  onEvent?: (event: StreamEvent) => void,
  signal?: AbortSignal,
) {
  const formData = new FormData()
  formData.append('text', payload.text)
  if (payload.modelId) formData.append('modelId', String(payload.modelId))
  for (const file of payload.files || []) {
    formData.append('files', file)
  }
  return setupStreamRequest(formData, onEvent, signal)
}

async function setupStreamRequest(
  body: FormData,
  onEvent?: (event: StreamEvent) => void,
  signal?: AbortSignal,
): Promise<NovelSetupData> {
  let setup: NovelSetupData | null = null
  for await (const event of multipartSSERequest('/novels/setup/complete', body, signal)) {
    onEvent?.(event)
    const a2ui = streamA2UIData(event)
    if (a2ui?.data?.kind === 'novel_setup_complete' && a2ui.data) {
      setup = a2ui.data.setup || null
    }
    if (event.type === 'error') {
      const message = streamErrorData(event)?.message
      if (message) throw new Error(message)
    }
  }
  if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
  if (!setup) throw new Error('生成小说模板失败')
  return setup
}

async function* multipartSSERequest(
  path: string,
  body: FormData,
  signal?: AbortSignal,
): AsyncGenerator<StreamEvent> {
  try {
    const response = await fetchWithAuthRefresh(`${API_BASE_URL}${path}`, {
      method: 'POST',
      body,
      signal,
    })
    if (!response.ok) {
      const payload = await response.json().catch(() => ({ msg: '请求失败' }))
      throw new Error(normalizeErrorMessage(payload.msg || '请求失败'))
    }
    yield* streamEventsFromResponse(response, false, signal)
  } catch (err) {
    if (
      (err instanceof DOMException && err.name === 'AbortError') ||
      (err instanceof Error && err.name === 'AbortError')
    ) {
      throw err
    }
    if (err instanceof Error) {
      const message = normalizeErrorMessage(err.message)
      emitApiError(message)
      throw new Error(message)
    }
    emitApiError('网络请求失败，请检查后端服务是否正常')
    throw err
  }
}

export function getNovelsApi(status?: 'archived') {
  const query = status ? `?status=${status}` : ''
  return request<ApiNovel[]>(`/novels${query}`)
}

export function getNovelOverviewApi(novelId: number) {
  return request<ApiNovelOverview>(`/novels/${novelId}/overview`)
}

export function getDashboardApi() {
  return request<ApiWorkspaceDashboard>('/dashboard')
}

export function archiveNovelApi(novelId: number) {
  return request<void>(`/novels/${novelId}/archive`, {
    method: 'POST',
  })
}

export function restoreNovelApi(novelId: number) {
  return request<void>(`/novels/${novelId}/restore`, {
    method: 'POST',
  })
}

export function getVolumesApi(novelId: number) {
  return request<ApiVolume[]>(`/novels/${novelId}/volumes`)
}

export function getChaptersApi(volumeId: number) {
  return request<ApiChapter[]>(`/volumes/${volumeId}/chapters`)
}

export function applyVolumePlanApi(novelId: number, plans: Record<string, unknown>[], force = false) {
  return request<ApiVolume[]>(`/novels/${novelId}/volumes/apply-plan`, {
    method: 'POST',
    body: JSON.stringify({ plans, force }),
    silentError: true,
  })
}

export function applyChapterPlanApi(volumeId: number, plans: Record<string, unknown>[], force = false) {
  return request<ApiChapter[]>(`/volumes/${volumeId}/chapters/apply-plan`, {
    method: 'POST',
    body: JSON.stringify({ plans, force }),
    silentError: true,
  })
}

export function createShareLinkApi(payload: CreateSharePayload) {
  return request<CreateShareResponse>('/shares', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function getSharedContentApi(type: string, token: string, password = '') {
  const query = password ? `?pwd=${encodeURIComponent(password)}` : ''
  return request<ApiSharedContent>(`/shares/${encodeURIComponent(type)}/${encodeURIComponent(token)}${query}`)
}

export function createDownloadApi(payload: { type: 'all' | 'novel' | 'volume' | 'chapter'; id?: number; format: string; layout?: 'volume' | 'chapter' }) {
  return request<DownloadJob>('/downloads', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function listDownloadsApi() {
  return request<DownloadJob[]>('/downloads')
}

export function getDownloadApi(jobId: string) {
  return request<DownloadJob>(`/downloads/${encodeURIComponent(jobId)}`)
}

export async function downloadFileApi(jobId: string) {
  try {
    const response = await fetchWithAuthRefresh(`${API_BASE_URL}/downloads/${encodeURIComponent(jobId)}/file`, {})
    if (!response.ok) {
      const body = await response.json().catch(() => ({ msg: '下载失败' }))
      const message = normalizeErrorMessage(body.msg || '下载失败')
      throw new Error(message)
    }
    const blob = await response.blob()
    const disposition = response.headers.get('Content-Disposition') || ''
    const filename = decodeDownloadFilename(disposition) || 'download'
    return { blob, filename }
  } catch (err) {
    if (err instanceof Error) {
      const message = normalizeErrorMessage(err.message)
      emitApiError(message)
      throw new Error(message)
    }
    emitApiError('下载失败')
    throw err
  }
}

function decodeDownloadFilename(disposition: string) {
  const match = disposition.match(/filename\*=UTF-8''([^;]+)/i)
  if (!match) return ''
  try {
    return decodeURIComponent(match[1].replace(/\+/g, '%20'))
  } catch {
    return match[1]
  }
}

export interface ApiMessage {
  id: number
  sessionId: number
  draftId?: number
  role: 'user' | 'assistant'
  content: string
  renderData?: RenderData
  temporary?: boolean
  createdAt: string
  updatedAt?: string
}

export interface ApiChatSessionMeta {
  id: number
  scopeType: number
  scopeId: number
}

export interface ApiMessagesResponse {
  messages: ApiMessage[]
  session: ApiChatSessionMeta
}

export interface ApiModel {
  id: number
  userId: number
  name: string
  provider: string
  modelId: string
  apiUrl: string
  apiKey: string
  topP: number
  temperature: number
  status: number
  createdAt: string
  updatedAt: string
}

export interface CreateModelPayload {
  name: string
  provider: string
  modelId: string
  apiUrl: string
  apiKey: string
  status: number
}

export function getMessagesApi(
  type: 'novel' | 'volume' | 'chapter',
  entityId: number,
) {
  const prefix = scopedApiPrefix(type)
  return request<ApiMessagesResponse>(`${prefix}/${entityId}/messages`)
}

export function cancelChatStreamApi(
  type: 'novel' | 'volume' | 'chapter',
  entityId: number,
) {
  const prefix = scopedApiPrefix(type)
  return request<void>(`${prefix}/${entityId}/stream/cancel`, {
    method: 'POST',
  })
}

export function useChapterDraftApi(chapterId: number, draftId: number) {
  return request<void>(
    `/chapters/${chapterId}/drafts/${draftId}/use`,
    {
      method: 'POST',
    },
  )
}

export interface ApiChapterContentDraft {
  id: number
  chapterId: number
  sourceMessageId: number
  draftType: number
  originDraftId: number
  draftName: string
  content: string
  status: number
  wordCount: number
  usedAt: string | null
  createdAt: string
  updatedAt: string
}

export function toChapterContentDraft(apiDraft: ApiChapterContentDraft): ChapterContentDraft {
  return {
    id: apiDraft.id,
    chapterId: apiDraft.chapterId,
    sourceMessageId: apiDraft.sourceMessageId,
    draftType: apiDraft.draftType,
    originDraftId: apiDraft.originDraftId,
    draftName: apiDraft.draftName || '',
    content: apiDraft.content,
    status: apiDraft.status,
    wordCount: apiDraft.wordCount,
    usedAt: apiDraft.usedAt ? new Date(apiDraft.usedAt) : null,
    createdAt: new Date(apiDraft.createdAt),
    updatedAt: new Date(apiDraft.updatedAt),
  }
}

export function createChapterDraftFromContentApi(chapterId: number, content: string) {
  return request<ApiChapterContentDraft>(`/chapters/${chapterId}/drafts`, {
    method: 'POST',
    body: JSON.stringify({ content }),
  })
}

export function listChapterDraftsApi(chapterId: number) {
  return request<ApiChapterContentDraft[]>(`/chapters/${chapterId}/drafts`)
}

export function joinChapterDraftApi(chapterId: number, draftId: number) {
  return request<ApiChapterContentDraft>(`/chapters/${chapterId}/drafts/${draftId}/join`, {
    method: 'POST',
  })
}

export function updateChapterDraftApi(chapterId: number, draftId: number, content: string, draftName?: string) {
  return request<void>(`/chapters/${chapterId}/drafts/${draftId}`, {
    method: 'PATCH',
    body: JSON.stringify({ content, draftName }),
  })
}

export function deleteChapterDraftApi(chapterId: number, draftId: number) {
  return request<void>(`/chapters/${chapterId}/drafts/${draftId}`, {
    method: 'DELETE',
  })
}

export interface ApiChapterHumanizeResult {
  content: string
  report: string
}

export interface ApiChapterProofreadSuggestion {
  originalText: string
  suggestedText: string
  reason: string
}

export function humanizeChapterApi(chapterId: number, content: string, draftId?: number, signal?: AbortSignal) {
  return request<ApiChapterHumanizeResult>(`/chapters/${chapterId}/humanize`, {
    method: 'POST',
    body: JSON.stringify({ content, draftId }),
    signal,
  })
}

export function proofreadChapterApi(chapterId: number, content: string, draftId?: number, signal?: AbortSignal) {
  return request<ApiChapterProofreadSuggestion[]>(`/chapters/${chapterId}/proofread`, {
    method: 'POST',
    body: JSON.stringify({ content, draftId }),
    signal,
  })
}

export function getModelsApi() {
  return request<ApiModel[]>('/models')
}

export function createModelApi(payload: CreateModelPayload) {
  return request<ApiModel>('/models', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function deleteModelApi(modelId: number) {
  return request<void>(`/models/${encodeURIComponent(String(modelId))}`, {
    method: 'DELETE',
  })
}

export function testModelApi(payload: CreateModelPayload) {
  return request<{ ok: boolean; message: string }>('/models/test', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

// ---- SSE 流式 ----

export interface StreamEvent {
  type: 'delta' | 'done' | 'error' | 'sync' | 'a2ui' | 'tool_call' | 'tool_result'
  data?: StreamEventData
}

export type StreamEventData =
  | StreamDelta
  | StreamSync
  | StreamDone
  | StreamError
  | StreamA2UI
  | StreamToolCall
  | StreamToolResult

export interface StreamDelta {
  text?: string
}

export interface StreamSync {
  content?: string
  renderData?: RenderData
}

export interface StreamDone {
  tokenCount?: number
  finishReason?: string
  params?: StreamDoneParams
}

export interface StreamDoneParams {
  draftId?: number
}

export interface StreamError {
  message?: string
}

export interface StreamA2UI {
  data?: RenderData
}

export interface StreamToolCall {
  name?: string
}

export interface StreamToolResult {
  name?: string
  result?: string
}

export type A2UIEvent = StreamA2UI

export type RenderData = {
  kind?: string
  optionType?: string
  options?: PlanOption[]
  complete?: boolean
  draft?: RawChapterDraft
  stage?: string
  text?: string
  attempt?: number
  steps?: string[]
  preview?: string
  issues?: string[]
  step_outputs?: Array<{
    step?: unknown
    attempt?: unknown
    type?: unknown
    text?: unknown
    items?: unknown
  }>
  step_output_delta?: {
    step?: unknown
    attempt?: unknown
    type?: unknown
    text?: unknown
    items?: unknown
  }
  current_step_label?: string
  current_step_started_at?: string
  step_timings?: Array<{
    key?: unknown
    label?: unknown
    startedAt?: unknown
    endedAt?: unknown
  }>
  collapsed?: boolean
  failed?: boolean
  setup?: NovelSetupData
}

export interface PlanOption {
  id: string
  title: string
  description: string
  optionType?: string
  custom?: boolean
  details?: Record<string, unknown>
}

type RawChapterDraft = {
  title?: unknown
  content?: unknown
  draft_id?: unknown
  draftId?: unknown
  revision_notes?: unknown
}

function numberID(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  return undefined
}

export function toChapterDraft(raw: RawChapterDraft | undefined): ChapterDraft | undefined {
  if (!raw || typeof raw !== 'object') return undefined
  const content = typeof raw.content === 'string' ? raw.content : ''
  const title = typeof raw.title === 'string' && raw.title.trim() ? raw.title.trim() : '章节正文'
  if (!content.trim() && !title) return undefined
  return {
    draftId: numberID(raw.draft_id ?? raw.draftId),
    title,
    content,
    revisionNotes: typeof raw.revision_notes === 'string' ? raw.revision_notes.trim() : '',
  }
}

export async function* streamChatApi(
  type: 'novel' | 'volume' | 'chapter',
  entityId: number,
  content: string,
  graphMode = false,
): AsyncGenerator<StreamEvent> {
  const prefix = scopedApiPrefix(type)
  try {
    const response = await fetchWithAuthRefresh(`${API_BASE_URL}${prefix}/${entityId}/stream`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ content, graphMode }),
    })

    if (!response.ok) {
      const body = await response.json().catch(() => ({ msg: '请求失败' }))
      const message = normalizeErrorMessage(body.msg || '请求失败')
      throw new Error(message)
    }

    yield* streamEventsFromResponse(response)
  } catch (err) {
    if (err instanceof Error) {
      const message = normalizeErrorMessage(err.message)
      emitApiError(message)
      throw new Error(message)
    }
    emitApiError('网络请求失败，请检查后端服务是否正常')
    throw err
  }
}

export async function* resumeChatStreamApi(
  type: 'novel' | 'volume' | 'chapter',
  entityId: number,
): AsyncGenerator<StreamEvent> {
  const prefix = scopedApiPrefix(type)
  yield* readSSE(`${API_BASE_URL}${prefix}/${entityId}/stream/resume`)
}

async function* readSSE(url: string): AsyncGenerator<StreamEvent> {
  try {
    const response = await fetchWithAuthRefresh(url, { method: 'GET' })
    if (!response.ok) {
      const body = await response.json().catch(() => ({ msg: '请求失败' }))
      const message = normalizeErrorMessage(body.msg || '请求失败')
      throw new Error(message)
    }
    yield* streamEventsFromResponse(response)
  } catch (err) {
    if (err instanceof Error) {
      const message = normalizeErrorMessage(err.message)
      emitApiError(message)
      throw new Error(message)
    }
    emitApiError('网络请求失败，请检查后端服务是否正常')
    throw err
  }
}

async function* streamEventsFromResponse(response: Response, emitErrors = true, signal?: AbortSignal): AsyncGenerator<StreamEvent> {
  const reader = response.body?.getReader()
  if (!reader) throw new Error('浏览器不支持流式响应')
  const abortReader = () => {
    void reader.cancel().catch(() => {})
  }
  if (signal?.aborted) {
    abortReader()
    throw new DOMException('Aborted', 'AbortError')
  }
  signal?.addEventListener('abort', abortReader, { once: true })
  const decoder = new TextDecoder()
  let buffer = ''
  let eventType = ''
  try {
    while (true) {
      if (signal?.aborted) throw new DOMException('Aborted', 'AbortError')
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      for (const line of lines) {
        if (line.startsWith('event: ')) {
          eventType = line.slice(7).trim()
          continue
        }
        if (!line.startsWith('data: ')) continue
        try {
          const payload = JSON.parse(line.slice(6))
          const event = streamEventFromPayload(eventType, payload)
          eventType = ''
          if (!event) continue
          const message = streamErrorData(event)?.message
          if (emitErrors && event.type === 'error' && message) emitApiError(message)
          yield event
        } catch {
          eventType = ''
        }
      }
    }
  } finally {
    signal?.removeEventListener('abort', abortReader)
    reader.releaseLock()
  }
}

function streamEventFromPayload(eventType: string, payload: unknown): StreamEvent | undefined {
  if (!isStreamEventType(eventType)) return undefined
  return {
    type: eventType,
    data: payload && typeof payload === 'object' ? (payload as StreamEventData) : undefined,
  }
}

function isStreamEventType(value: string): value is StreamEvent['type'] {
  return ['delta', 'done', 'error', 'sync', 'a2ui', 'tool_call', 'tool_result'].includes(value)
}

export function streamDeltaData(event: StreamEvent): StreamDelta | undefined {
  return event.type === 'delta' ? event.data as StreamDelta | undefined : undefined
}

export function streamSyncData(event: StreamEvent): StreamSync | undefined {
  return event.type === 'sync' ? event.data as StreamSync | undefined : undefined
}

export function streamDoneData(event: StreamEvent): StreamDone | undefined {
  return event.type === 'done' ? event.data as StreamDone | undefined : undefined
}

export function streamErrorData(event: StreamEvent): StreamError | undefined {
  return event.type === 'error' ? event.data as StreamError | undefined : undefined
}

export function streamA2UIData(event: StreamEvent): StreamA2UI | undefined {
  return event.type === 'a2ui' ? event.data as StreamA2UI | undefined : undefined
}

function scopedApiPrefix(type: 'novel' | 'volume' | 'chapter'): string {
  if (type === 'novel') return '/novels'
  if (type === 'volume') return '/volumes'
  return '/chapters'
}
