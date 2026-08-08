import type { OverviewData } from '@/types'

const LABELS: Record<string, string> = {
  summary: '梗概',
  timeline: '所处时间',
  locations: '主要地点',
  characters: '出场人物',
  character_plan: '人物使用',
  key_selling_points: '主要看点',
  risk_control: '雷点处理',
  forces: '势力设定',
  other_settings: '其他设定',
  genre: '题材',
  protagonist: '主角',
  world: '世界观',
  main_goal: '主线目标',
  core_conflict: '核心冲突',
  tone: '风格情绪',
  style: '风格',
  character_settings: '重点人物',
  current_state: '当前状态',
  end_state: '结束状态',
  character_development: '角色成长',
  setting_development: '设定发展',
  chapter_count: '章节数量',
  key_events: '关键事件',
  intertextual_links: '跨章关联',
  foreshadowing: '伏笔',
  writing_focus: '写作重点',
  other_highlights: '其他亮点',
  setting_boundaries: '设定边界',
  temporary_settings: '临时设定',
}

const PRIMARY_FIELDS = ['summary']
const HIDDEN_FIELDS = new Set(['title', 'sort_order', 'relationships', 'references', 'setting_boundaries'])
const SUMMARY_DETAIL_FIELDS = new Set(PRIMARY_FIELDS)
const DETAIL_ORDER = [
  'summary',
  'timeline',
  'locations',
  'characters',
  'world',
  'core_conflict',
  'character_plan',
  'character_settings',
  'forces',
  'other_settings',
  'key_selling_points',
  'risk_control',
  'current_state',
  'end_state',
  'character_development',
  'setting_development',
  'setting_boundaries',
  'temporary_settings',
  'chapter_count',
  'foreshadowing',
  'other_highlights',
  'key_events',
  'intertextual_links',
  'writing_focus',
]

export interface OverviewDetail {
  key: string
  label: string
  value: string | string[]
}

export function overviewSummary(overview: OverviewData): string {
  for (const key of PRIMARY_FIELDS) {
    const value = formatOverviewValue(overview[key])
    if (value) return value
  }
  return ''
}

export function overviewDetails(overview: OverviewData): OverviewDetail[] {
  return Object.entries(overview)
    .filter(([key, value]) => !HIDDEN_FIELDS.has(key) && !SUMMARY_DETAIL_FIELDS.has(key) && hasOverviewValue(value))
    .sort(([left], [right]) => fieldOrder(left) - fieldOrder(right))
    .map(([key, value]) => ({
      key,
      label: LABELS[key] || key,
      value: key === 'chapter_count' ? `${formatOverviewValue(value)}章左右` : formatOverviewDetailValue(value),
    }))
}

function fieldOrder(key: string): number {
  const index = DETAIL_ORDER.indexOf(key)
  return index >= 0 ? index : DETAIL_ORDER.length
}

export function formatOverviewValue(value: unknown): string {
  if (Array.isArray(value)) {
    return value.map(formatOverviewArrayItem).filter(Boolean).join('、')
  }
  if (value && typeof value === 'object') {
    return JSON.stringify(value)
  }
  if (value === null || value === undefined) return ''
  return String(value).trim()
}

function formatOverviewDetailValue(value: unknown): string | string[] {
  if (Array.isArray(value)) {
    return value.map(formatOverviewArrayItem).filter(Boolean)
  }
  return formatOverviewValue(value)
}

function hasOverviewValue(value: unknown): boolean {
  if (Array.isArray(value)) {
    return value.some((item) => formatOverviewArrayItem(item))
  }
  return Boolean(formatOverviewValue(value))
}

function formatOverviewArrayItem(item: unknown): string {
  if (item === null || item === undefined) return ''
  if (typeof item !== 'object') return String(item).trim()
  if (Array.isArray(item)) return item.map(formatOverviewArrayItem).filter(Boolean).join('、')
  return formatOverviewObjectItem(item as Record<string, unknown>)
}

function formatOverviewObjectItem(item: Record<string, unknown>): string {
  const name = textValue(item.name)
  const stateBefore = textValue(item.state_before)
  const stateAfter = textValue(item.state_after)
  const allowedProgress = textValue(item.allowed_progress)
  const forbiddenProgress = textValue(item.forbidden_progress)
  const description = textValue(item.description || item.notes || item.summary)
  const parts = [
    name,
    stateBefore || stateAfter ? `${stateBefore || '未写'} -> ${stateAfter || '未写'}` : '',
    allowedProgress ? `允许：${allowedProgress}` : '',
    forbiddenProgress ? `禁止：${forbiddenProgress}` : '',
    description && !name ? description : '',
  ].filter(Boolean)
  return parts.length > 0 ? parts.join('；') : JSON.stringify(item)
}

function textValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}
