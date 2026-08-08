<script setup lang="ts">
import { ref, watch, nextTick, computed, onMounted, onUnmounted } from "vue";
import {
  MoreHorizontal,
  AlignLeft,
  ArrowLeft,
  ArrowRight,
  ChevronRight,
  ChevronDown,
  ArrowUp,
  Square,
  Pencil,
  Wand2,
  Search,
  Replace,
  BarChart3,
  Eraser,
  Bot,
  UserSearch,
  Network,
  Info,
  FileText,
  BookOpen,
  Clock,
  Calendar,
  Lightbulb,
  AlertTriangle,
  Puzzle,
  Target,
  Check,
  CheckCircle2,
  ShieldCheck,
  Users,
  MapPinned,
  Plus,
  Trash2,
  RefreshCw,
  Loader2,
  Mic,
  X,
} from "lucide-vue-next";
import MarkdownIt from "markdown-it";
import DOMPurify from "dompurify";
import type { Message, NovelSetupData, PlanOption } from "@/types";
import { useAppStore } from "@/stores/app";
import {
  completeNovelSetupStreamApi,
  createChapterDraftFromContentApi,
  humanizeChapterApi,
  proofreadChapterApi,
  streamA2UIData,
  toChapterContentDraft,
  type StreamEvent,
  type ApiChapterProofreadSuggestion,
} from "@/services/api";
import BackToTopButton from "@/components/common/BackToTopButton.vue";
import CharacterRelationGraph from "@/components/common/CharacterRelationGraph.vue";
import welcomeImage from "@/assets/img/welcome.webp";
import emptyEditImage from "@/assets/img/edit.webp";

const store = useAppStore();

const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
});

const viewBreadcrumb = computed(() =>
  store.viewMode === "editor" ? store.editorBreadcrumb : store.chatBreadcrumb
);

const inputText = ref("");
const inputRef = ref<HTMLTextAreaElement | null>(null);
const messagesContainer = ref<HTMLElement | null>(null);
const editorContainer = ref<HTMLElement | null>(null);
const previewContainer = ref<HTMLElement | null>(null);
const streamingDraftContainer = ref<HTMLElement | null>(null);
const editorTextarea = ref<HTMLTextAreaElement | null>(null);
const draftTitleInput = ref<HTMLInputElement | null>(null);
const editorContent = ref("");
const editorDraftName = ref("");
const isEditingDraftName = ref(false);
const showFindPanel = ref(false);
const showReplacePanel = ref(false);
const showWordStats = ref(false);
const showDraftPicker = ref(false);
const showAITools = ref(false);
const isHumanizing = ref(false);
const humanizedContent = ref("");
const humanizeReport = ref("");
const showHumanizeReport = ref(false);
const showHumanizeMenu = ref(false);
const isProofreading = ref(false);
const proofreadSuggestions = ref<ApiChapterProofreadSuggestion[]>([]);
const ignoredProofreadIndexes = ref<number[]>([]);
const editorScrollTop = ref(0);
const isVoiceRecognizing = ref(false);
const canSubmitInput = computed(
  () => inputText.value.trim().length > 0 || isVoiceRecognizing.value
);
const voiceTranscript = ref("");
const chapterGraphMode = ref(false);
const findText = ref("");
const replaceText = ref("");
const highlightedWord = ref("");
const activeFindIndex = ref(0);
const findInput = ref<HTMLInputElement | null>(null);
const thinkingNow = ref(Date.now());
const messageThinkingStarts = ref<Record<string, number>>({});
const chapterProgressStarts = ref<Record<string, number>>({});
const chapterProgressStepLabels = ref<Record<string, string>>({});
const chapterProgressSteps = ref<Record<string, string[]>>({});
const chapterProgressCompletedAt = ref<Record<string, number>>({});
const expandedChapterStepOutputKeys = ref<Record<string, boolean>>({});
let editorSaveTimer: ReturnType<typeof setTimeout> | null = null;
let draftPickerCloseTimer: ReturnType<typeof setTimeout> | null = null;
let aiToolsCloseTimer: ReturnType<typeof setTimeout> | null = null;
let thinkingTimer: ReturnType<typeof setInterval> | null = null;
let humanizeAbortController: AbortController | null = null;
let proofreadAbortController: AbortController | null = null;
let setupAssistAbortController: AbortController | null = null;
let messageAutoScrollTimer: ReturnType<typeof setTimeout> | null = null;
let planApplyConfirmTimer: ReturnType<typeof setInterval> | null = null;
let speechRecognition: any = null;
let voiceOriginalInput = "";
let voiceInputBase = "";
let lastVoiceTranscript = "";
let voiceRestartTimer: ReturnType<typeof setTimeout> | null = null;
let programmaticMessageScroll = false;
const setupTitle = ref("");
const setupDirection = ref("");
const setupOriginalText = ref("");
const setupLength = ref("中篇");
const setupPerspective = ref("第三人称");
const setupTags = ref<string[]>([]);
const setupCustomTags = ref<Record<string, string>>({});
const setupCustomAddedTags = ref<Record<string, string[]>>({});
type SetupCharacter = {
  id: string;
  name: string;
  notes: string;
  appearanceTime: string;
};

type SetupCharacterRelationship = {
  characterA: string;
  characterB: string;
  description?: string;
};

type SetupMap = {
  id: string;
  name: string;
  appearanceTime: string;
  notes: string;
};

type SetupForce = {
  id: string;
  name: string;
  appearanceTime: string;
  notes: string;
};

type SetupOtherItem = {
  id: string;
  name: string;
  notes: string;
  appearanceTime: string;
};

type CharacterGraphNode = SetupCharacter & {
  x?: number;
  y?: number;
};

type CharacterGraphEdge = {
  id: string;
  index: number;
  source: string;
  target: string;
  description: string;
};

type AppearanceTimeTarget = "character" | "map" | "setup-item";

type SetupOtherSetting = {
  id: string;
  title: string;
  description: string;
  items: SetupOtherItem[];
};

type ProofreadTextSegment = {
  text: string;
  suggestionIndex: number | null;
};

type ConfirmDialogState = {
  message: string;
  onConfirm: () => void;
};

type PlanApplyConfirmState = {
  messageId: string;
  optionType: "volume" | "chapter";
  options: PlanOption[];
  message: string;
};

const setupCharacters = ref<SetupCharacter[]>([]);
const setupRelationships = ref<SetupCharacterRelationship[]>([]);
const setupMaps = ref<SetupMap[]>([]);
const setupForces = ref<SetupForce[]>([]);
const setupOtherSettings = ref<SetupOtherSetting[]>([]);
const isCharacterListOpen = ref(false);
const isCharacterGraphOpen = ref(false);
const isMapListOpen = ref(false);
const isFixedOtherItemsOpen = ref(false);
const isOtherSettingsOpen = ref(false);
const isCharacterModalOpen = ref(false);
const isMapModalOpen = ref(false);
const isOtherSettingModalOpen = ref(false);
const isOtherItemModalOpen = ref(false);
const isSetupAssistOpen = ref(false);
const isSetupAssistLoading = ref(false);
const setupAssistThinkingText = ref("");
const setupAssistStartedAt = ref<number | null>(null);
const setupAssistText = ref("");
const setupAssistFiles = ref<File[]>([]);
const setupAssistModelId = ref(0);
const setupAssistModelOpen = ref(false);
const confirmDialog = ref<ConfirmDialogState | null>(null);
const planApplyConfirm = ref<PlanApplyConfirmState | null>(null);
const planApplyConfirmRemaining = ref(0);
const setupAssistLastPayload = ref<{
  text: string;
  files: File[];
  modelId: number;
} | null>(null);
const editingCharacterId = ref<string | null>(null);
const editingMapId = ref<string | null>(null);
const editingForceId = ref<string | null>(null);
const isForceItemModal = ref(false);
const editingOtherSettingId = ref<string | null>(null);
const activeOtherSettingId = ref<string | null>(null);
const editingOtherItemId = ref<string | null>(null);
const characterGraphNodePositions = ref<
  Record<string, { x: number; y: number }>
>({});
const selectedCharacterGraphNodeId = ref<string | null>(null);
const editingGraphRelationshipIndex = ref<number | null>(null);
const pendingGraphRelationshipDeleteIndex = ref<number | null>(null);
const relationshipHistory = ref<SetupCharacterRelationship[][]>([]);

function pushRelationshipHistory() {
  relationshipHistory.value = [
    ...relationshipHistory.value.slice(-49),
    setupRelationships.value.map((r) => ({ ...r })),
  ];
}

function undoRelationship() {
  const prev = relationshipHistory.value.pop();
  if (!prev) return;
  setupRelationships.value = prev;
}
const characterForm = ref({ name: "", appearanceTime: "前期", notes: "" });
const mapForm = ref({ name: "", appearanceTime: "前期", notes: "" });
const forceForm = ref({ name: "", appearanceTime: "前期", notes: "" });
const otherSettingForm = ref({ title: "", description: "" });
const otherItemForm = ref({ name: "", notes: "", appearanceTime: "前期" });
const appearanceTimeOptions = ["前期", "中期", "后期"];
const appearanceTimeMenuOpen = ref<AppearanceTimeTarget | null>(null);
const savedNovelSetupSignature = ref("");
const activeCustomOption = ref<string | null>(null);
const customInputText = ref("");
const modelOpen = ref(false);
const previewMessageId = ref<string | null>(null);
const previewDraftCache = ref<NonNullable<Message["chapterDraft"]> | null>(
  null
);
const expandedPlanDetailKey = ref<string | null>(null);
const activeQuestionMessageId = ref<string>("");
const shouldScrollToLatestQuestion = ref(false);
const shouldAutoScrollMessages = ref(true);
const shouldAutoScrollDraft = ref(true);
const messageAutoScrollPausedByWheel = ref(false);

const currentNovelSetupSignature = computed(() =>
  JSON.stringify(buildNovelSetupData())
);
const shouldShowScrollToLatestReply = computed(
  () => Boolean(store.activeStream) && !shouldAutoScrollMessages.value
);
const isNovelSetupDirty = computed(
  () => currentNovelSetupSignature.value !== savedNovelSetupSignature.value
);
const hasNovelSetupPlanData = computed(
  () =>
    setupCharacters.value.length > 0 ||
    setupMaps.value.length > 0 ||
    setupForces.value.length > 0 ||
    setupOtherSettings.value.some((setting) => setting.items.length > 0)
);

const editorTitle = computed(() =>
  store.editorChapter ? store.chapterTitle(store.editorChapter.chapter) : ""
);
const wordCount = computed(() => {
  const content = editorContent.value;
  if (!content) return 0;
  return content.replace(/\s/g, "").length;
});
const hasEditorContent = computed(() => editorContent.value.trim().length > 0);
const editorToolButtonClass = computed(() => [
  "rounded-lg p-2 transition-colors disabled:cursor-not-allowed",
  hasEditorContent.value
    ? "text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
    : "text-gray-300 dark:text-gray-600",
]);
const findMatches = computed(() => {
  if (!findText.value) return 0;
  return editorContent.value.split(findText.value).length - 1;
});
const editorSaveLabel = computed(() => {
  if (store.editorSaveStatus === "saving") return "保存中";
  if (store.editorSaveStatus === "saved") return "已保存";
  if (store.editorSaveStatus === "error") return "保存失败";
  return "未保存";
});
const setupAssistSelectedModel = computed(() =>
  store.models.find((model) => model.id === setupAssistModelId.value)
);
const frequentWords = computed(() => {
  return extractFrequentWords(editorContent.value);
});
const activeOtherSetting = computed(() =>
  setupOtherSettings.value.find(
    (item) => item.id === activeOtherSettingId.value
  )
);

const setupItemModalTitle = computed(() => {
  if (isForceItemModal.value) {
    return editingForceId.value ? "编辑势力" : "添加势力";
  }
  return editingOtherItemId.value ? "编辑设定内容" : "添加设定内容";
});
const setupItemModalSubtitle = computed(() =>
  isForceItemModal.value
    ? "势力设定"
    : activeOtherSetting.value?.title || "其他设定"
);
const setupItemNotesPlaceholder = computed(() =>
  isForceItemModal.value
    ? "说明类型、地位、目标、阵营关系、控制资源、冲突用途和出场阶段。"
    : "说明具体规则、来源、等级、用途、限制或剧情作用。"
);
const graphRelationshipCount = computed(() => setupRelationships.value.length);
function characterNameById(id: string): string {
  return setupCharacters.value.find((c) => c.id === id)?.name || "未知人物";
}
const editingGraphRelationship = computed(() =>
  editingGraphRelationshipIndex.value === null
    ? null
    : setupRelationships.value[editingGraphRelationshipIndex.value] || null
);
const pendingGraphRelationshipDelete = computed(() =>
  pendingGraphRelationshipDeleteIndex.value === null
    ? null
    : characterGraphEdges.value.find(
        (edge) => edge.index === pendingGraphRelationshipDeleteIndex.value
      ) || null
);
const selectedCharacterGraphCharacter = computed(() =>
  setupCharacters.value.find(
    (character) => character.id === selectedCharacterGraphNodeId.value
  )
);
const characterGraphNodes = computed<CharacterGraphNode[]>(() => {
  return setupCharacters.value.map((character) => {
    const customPosition = characterGraphNodePositions.value[character.id];
    return customPosition ? { ...character, ...customPosition } : character;
  });
});
const characterGraphEdges = computed<CharacterGraphEdge[]>(() => {
  const nodeIds = new Set(characterGraphNodes.value.map((node) => node.id));
  return setupRelationships.value.flatMap((relationship, index) => {
    if (
      !nodeIds.has(relationship.characterA) ||
      !nodeIds.has(relationship.characterB)
    )
      return [];
    return [
      {
        id: `relationship-${index}`,
        index,
        source: relationship.characterA,
        target: relationship.characterB,
        description: relationship.description?.trim() || "",
      },
    ];
  });
});

const visibleProofreadSuggestions = computed(() =>
  proofreadSuggestions.value
    .map((suggestion, index) => ({ ...suggestion, index }))
    .filter((item) => !ignoredProofreadIndexes.value.includes(item.index))
);

const proofreadTextSegments = computed<ProofreadTextSegment[]>(() => {
  const content = editorContent.value;
  if (!content || visibleProofreadSuggestions.value.length === 0) return [];

  const matches: { start: number; end: number; index: number }[] = [];
  for (const suggestion of visibleProofreadSuggestions.value) {
    const start = content.indexOf(suggestion.originalText);
    if (start < 0) continue;
    const end = start + suggestion.originalText.length;
    const overlaps = matches.some(
      (match) => start < match.end && end > match.start
    );
    if (!overlaps) matches.push({ start, end, index: suggestion.index });
  }

  matches.sort((a, b) => a.start - b.start);
  const segments: ProofreadTextSegment[] = [];
  let cursor = 0;
  for (const match of matches) {
    if (match.start > cursor) {
      segments.push({
        text: content.slice(cursor, match.start),
        suggestionIndex: null,
      });
    }
    segments.push({
      text: content.slice(match.start, match.end),
      suggestionIndex: match.index,
    });
    cursor = match.end;
  }
  if (cursor < content.length) {
    segments.push({ text: content.slice(cursor), suggestionIndex: null });
  }
  return segments;
});

const activeMessageScrollKey = computed(() => {
  const latest = store.activeMessages[store.activeMessages.length - 1];
  return [
    store.activeMessages.length,
    latest?.id || "",
    latest?.content.length || 0,
    latest?.planOptions?.length || 0,
    latest?.chapterDraft?.content.length || 0,
    latest?.isThinking ? 1 : 0,
  ].join(":");
});

const wordStatsStopWords = new Set([
  "一个",
  "一种",
  "这个",
  "那个",
  "这些",
  "那些",
  "自己",
  "他们",
  "我们",
  "你们",
  "什么",
  "没有",
  "不是",
  "已经",
  "还是",
  "只是",
  "然后",
  "因为",
  "所以",
  "但是",
  "并且",
  "而且",
  "或者",
  "以及",
  "可以",
  "需要",
  "开始",
  "出来",
  "起来",
  "下去",
  "过去",
  "现在",
  "这里",
  "那里",
  "之中",
  "之间",
  "之后",
  "之前",
  "一切",
  "所有",
  "任何",
  "突然",
  "终于",
  "如果",
  "时候",
  "里面",
  "外面",
]);

function extractFrequentWords(content: string): [string, number][] {
  const counts = new Map<string, number>();
  for (const word of segmentWords(content)) {
    if (wordStatsStopWords.has(word)) continue;
    counts.set(word, (counts.get(word) || 0) + 1);
  }
  return Array.from(counts.entries())
    .filter(([, count]) => count > 1)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 20);
}

function segmentWords(content: string): string[] {
  const normalized = content
    .replace(/[^\p{L}\p{N}]+/gu, " ")
    .replace(/\s+/g, " ")
    .trim();
  if (!normalized) return [];
  const segmenterCtor = (
    Intl as unknown as {
      Segmenter?: new (locale: string, options: { granularity: "word" }) => {
        segment(
          text: string
        ): Iterable<{ segment: string; isWordLike?: boolean }>;
      };
    }
  ).Segmenter;
  if (!segmenterCtor) {
    return fallbackSegmentWords(normalized);
  }
  const segmenter = new segmenterCtor("zh-CN", { granularity: "word" });
  const words: string[] = [];
  for (const item of segmenter.segment(normalized)) {
    const word = normalizeCountableWord(item.segment);
    if (!item.isWordLike || !word) continue;
    words.push(word);
  }
  return words;
}

function fallbackSegmentWords(content: string): string[] {
  return (content.match(/[A-Za-z]{3,}|[\u4e00-\u9fa5]{2,6}/g) || [])
    .map(normalizeCountableWord)
    .filter(Boolean);
}

function normalizeCountableWord(word: string): string {
  word = word.trim();
  if (!isCountableWord(word)) return "";
  return /[A-Za-z]/.test(word) ? word.toLowerCase() : word;
}

function isCountableWord(word: string): boolean {
  if (/^\d+$/.test(word)) return false;
  if (/^[A-Za-z]{3,}$/.test(word)) return true;
  return /^[\u4e00-\u9fa5]{2,6}$/.test(word);
}

const setupTagGroups = [
  {
    title: "题材",
    tags: [
      "都市",
      "玄幻",
      "仙侠",
      "科幻",
      "悬疑",
      "灵异",
      "历史",
      "游戏",
      "校园",
      "末世",
    ],
  },
  {
    title: "类型",
    tags: [
      "男频",
      "女频",
      "重生",
      "系统",
      "群像",
      "单女主",
      "种田流",
      "升级流",
      "无限流",
      "经营流",
    ],
  },
  {
    title: "基调",
    tags: [
      "热血",
      "轻松",
      "紧张",
      "黑暗",
      "治愈",
      "爽感",
      "温馨",
      "压抑",
      "搞笑",
      "史诗",
    ],
  },
  {
    title: "文风",
    tags: [
      "番茄爽文",
      "晋江慢热",
      "轻小说",
      "现实主义",
      "群像史诗",
      "短句风",
      "知乎",
      "快节奏",
    ],
  },
  {
    title: "雷点",
    tags: [
      "虐主",
      "降智",
      "后宫",
      "烂尾",
      "误会拖剧情",
      "水文",
      "说教",
      "圣母",
    ],
  },
];

const setupLengthOptions = [
  { label: "短篇", desc: "约 20-50 章" },
  { label: "中篇", desc: "约 200-400 章" },
  { label: "长篇", desc: "约 600-900 章" },
];

const setupPerspectiveOptions = [
  { label: "第一人称", desc: "以“我”的视角叙述故事" },
  { label: "第二人称", desc: "以“你”的视角叙述故事" },
  { label: "第三人称", desc: "以旁观者视角叙述故事" },
];

const setupModeOptions = [
  {
    label: "Plan",
    title: "规划创作",
    desc: "先完成核心设定，再进入卷、章、正文创作",
    enabled: true,
    icon: Wand2,
  },
  {
    label: "Open",
    title: "开放写作",
    desc: "开放式写作，不预设规划流程",
    enabled: false,
    icon: Pencil,
  },
];

const dashboardData = computed(() => store.dashboard);

const dashboardTotalWords = computed(
  () => dashboardData.value?.totalWords || 0
);

const dashboardTotalChapters = computed(
  () => dashboardData.value?.completedChapters || 0
);

const dashboardLastEdited = computed(() =>
  dashboardData.value?.lastEditedAt
    ? formatRelativeTime(dashboardData.value.lastEditedAt)
    : "暂无"
);

const dashboardWritingHours = computed(() => {
  const hours = dashboardData.value?.writingHours || 0;
  return hours >= 10 ? String(Math.round(hours)) : hours.toFixed(1);
});

const dashboardWordTrend = computed(() => {
  const trend = dashboardData.value?.wordTrend || [];
  if (trend.length > 0) return trend;
  const today = new Date();
  return Array.from({ length: 7 }, (_, index) => {
    const date = new Date(today);
    date.setDate(today.getDate() - (6 - index));
    return {
      date: date.toISOString().slice(0, 10),
      weekday: "日一二三四五六"[date.getDay()],
      words: 0,
      wordLabel: "0 字",
    };
  });
});

const dashboardTrendMaxWords = computed(() =>
  Math.max(1000, ...dashboardWordTrend.value.map((point) => point.words))
);

const dashboardTrendPoints = computed(() => {
  const width = 620;
  const height = 176;
  const left = 54;
  const top = 18;
  const plotWidth = width - left - 24;
  const plotHeight = height - top - 34;
  const maxWords = dashboardTrendMaxWords.value;
  return dashboardWordTrend.value.map((point, index, list) => {
    const x =
      list.length <= 1 ? left : left + (plotWidth * index) / (list.length - 1);
    const y = top + plotHeight - (point.words / maxWords) * plotHeight;
    return {
      ...point,
      x,
      y,
      dayLabel: point.date.slice(5).replace("-", "/"),
      tooltip: `${point.date}（周${point.weekday}）\n完成：${point.wordLabel}`,
    };
  });
});

const dashboardTrendPath = computed(() =>
  dashboardTrendPoints.value
    .map((point, index) => `${index === 0 ? "M" : "L"} ${point.x} ${point.y}`)
    .join(" ")
);

const dashboardTrendAreaPath = computed(() => {
  const points = dashboardTrendPoints.value;
  if (points.length === 0) return "";
  const baseline = 150;
  return `${dashboardTrendPath.value} L ${
    points[points.length - 1].x
  } ${baseline} L ${points[0].x} ${baseline} Z`;
});

const dashboardTrendTicks = computed(() => {
  const maxWords = dashboardTrendMaxWords.value;
  return [1, 0.75, 0.5, 0.25, 0].map((ratio) => ({
    y: 18 + (1 - ratio) * 124,
    label: formatDashboardNumber(Math.round(maxWords * ratio)),
  }));
});

const dashboardTrendSummary = computed(() => {
  const total = dashboardWordTrend.value.reduce(
    (sum, point) => sum + point.words,
    0
  );
  const average = Math.round(
    total / Math.max(1, dashboardWordTrend.value.length)
  );
  return [
    { label: "总字数", value: `${formatDashboardNumber(total)} 字` },
    { label: "日均字数", value: `${formatDashboardNumber(average)} 字` },
    { label: "总创作时长", value: `${dashboardWritingHours.value} 小时` },
  ];
});

const dashboardTipCards = [
  {
    title: "先把想法说完整",
    desc: "新建小说时，尽量一次说清题材、主角、冲突和雷点。",
    icon: BookOpen,
  },
  {
    title: "按章节顺序生成正文",
    desc: "生成正文，最好按照顺序生成，前后一致性会更高。",
    icon: Lightbulb,
  },
  {
    title: "最多同时跑三项",
    desc: "你可以并行发起最多三个 AI 任务，超过后需要等待。",
    icon: Puzzle,
  },
  {
    title: "覆盖规划前慎重考虑",
    desc: "应用新规划会覆盖旧规划、正文草稿和相关对话，确认前先检查清楚。",
    icon: AlertTriangle,
  },
];

const quickPrompts = computed(() => {
  if (store.selectedChapterId) {
    return ["直接按照规划开始生成正文", "重新生成"];
  }
  if (store.selectedVolumeId) {
    return [
      "说说你的想法",
      "换个思路",
      "按照这个思路，给我章节规划",
      "重新生成章节规划",
    ];
  }
  if (store.selectedNovelId) {
    return [
      "说说你的想法",
      "换个思路",
      "按照这个思路，生成卷规划",
      "重新生成卷规划",
    ];
  }
  return [];
});

function selectSetupMode(option: (typeof setupModeOptions)[number]) {
  if (!option.enabled) {
    store.notifyInfo("敬请期待");
    return;
  }
  store.openNovelSetupForm();
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesContainer.value) {
      programmaticMessageScroll = true;
      messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight;
      if (messageAutoScrollTimer) clearTimeout(messageAutoScrollTimer);
      messageAutoScrollTimer = setTimeout(() => {
        programmaticMessageScroll = false;
      }, 80);
    }
  });
}

function scrollToLatestAIReply() {
  shouldAutoScrollMessages.value = true;
  messageAutoScrollPausedByWheel.value = false;
  scrollToBottom();
}

function isMessagesNearBottom() {
  const container = messagesContainer.value;
  if (!container) return true;
  return (
    container.scrollHeight - container.scrollTop - container.clientHeight < 120
  );
}

function handleMessagesScroll() {
  updateActiveQuestion();
  if (programmaticMessageScroll) return;
  const nearBottom = isMessagesNearBottom();
  shouldAutoScrollMessages.value = nearBottom;
  if (nearBottom) {
    messageAutoScrollPausedByWheel.value = false;
  }
}

function handleMessagesWheel() {
  messageAutoScrollPausedByWheel.value = true;
  shouldAutoScrollMessages.value = false;
}

function handleStreamingDraftWheel() {
  shouldAutoScrollDraft.value = false;
}

function scrollToLatestQuestion() {
  nextTick(() => {
    const latest = [...store.activeMessages]
      .reverse()
      .find((message) => message.role === "user");
    if (latest) {
      scrollToMessage(latest.id, "center");
      return;
    }
    scrollToBottom();
  });
}

function prepareMessageAutoScroll() {
  shouldScrollToLatestQuestion.value = true;
  shouldAutoScrollMessages.value = true;
  messageAutoScrollPausedByWheel.value = false;
}

function toggleSetupTag(tag: string) {
  setupTags.value = setupTags.value.includes(tag)
    ? setupTags.value.filter((item) => item !== tag)
    : [...setupTags.value, tag];
}

function addSetupCustomTag(groupTitle: string) {
  const tag = setupCustomTags.value[groupTitle]?.trim();
  if (!tag) return;
  const added = setupCustomAddedTags.value[groupTitle] || [];
  setupCustomAddedTags.value = {
    ...setupCustomAddedTags.value,
    [groupTitle]: Array.from(new Set([...added, tag])),
  };
  setupCustomTags.value = { ...setupCustomTags.value, [groupTitle]: "" };
}

function openCharacterModal(character?: SetupCharacter) {
  editingCharacterId.value = character?.id || null;
  characterForm.value = {
    name: character?.name || "",
    appearanceTime: character?.appearanceTime || "前期",
    notes: character?.notes || "",
  };
  isCharacterModalOpen.value = true;
}

function closeCharacterModal() {
  isCharacterModalOpen.value = false;
  editingCharacterId.value = null;
  characterForm.value = { name: "", appearanceTime: "前期", notes: "" };
  appearanceTimeMenuOpen.value = null;
}

function confirmSetupDelete(message: string, onConfirm: () => void) {
  confirmDialog.value = { message, onConfirm };
}

function closeConfirmDialog() {
  confirmDialog.value = null;
}

function runConfirmDialogAction() {
  const action = confirmDialog.value?.onConfirm;
  confirmDialog.value = null;
  action?.();
}

function requestDeleteEditorDraft(draftId: number) {
  showDraftPicker.value = false;
  confirmSetupDelete("确定删除这个草稿吗？删除后不会影响已应用的正文。", () => {
    void store.deleteEditorDraft(draftId);
  });
}

function replaceCharacterNameInText(
  text: string,
  oldName: string,
  newName: string
) {
  if (!oldName || oldName === newName) return text;
  return text.split(oldName).join(newName);
}

function renameCharacterReferences(oldName: string, newName: string) {
  if (!oldName || oldName === newName) return 0;
  let changed = 0;
  const replaceText = (text: string) => {
    const next = replaceCharacterNameInText(text, oldName, newName);
    if (next !== text) changed++;
    return next;
  };
  setupCharacters.value = setupCharacters.value.map((character) => ({
    ...character,
    notes: replaceText(character.notes),
  }));
  setupRelationships.value = setupRelationships.value.map((relationship) => ({
    ...relationship,
    description: relationship.description
      ? replaceText(relationship.description)
      : relationship.description,
  }));
  setupMaps.value = setupMaps.value.map((map) => ({
    ...map,
    notes: replaceText(map.notes),
  }));
  setupForces.value = setupForces.value.map((force) => ({
    ...force,
    notes: replaceText(force.notes),
  }));
  setupOtherSettings.value = setupOtherSettings.value.map((setting) => ({
    ...setting,
    description: replaceText(setting.description),
    items: setting.items.map((item) => ({
      ...item,
      name: replaceText(item.name),
      notes: replaceText(item.notes),
    })),
  }));
  return changed;
}

function saveSetupCharacter() {
  const name = characterForm.value.name.trim();
  const appearanceTime = characterForm.value.appearanceTime.trim();
  const notes = characterForm.value.notes.trim();
  if (!name) {
    store.notifyError("请填写人物名称");
    return;
  }
  if (!appearanceTime) {
    store.notifyError("请填写人物出场时间");
    return;
  }
  if (!notes) {
    store.notifyError("请填写人物详细信息");
    return;
  }
  const character: SetupCharacter = {
    id: editingCharacterId.value || `c${Date.now()}`,
    name,
    appearanceTime,
    notes,
  };
  if (editingCharacterId.value) {
    const previous = setupCharacters.value.find(
      (item) => item.id === editingCharacterId.value
    );
    const changedReferences = previous
      ? renameCharacterReferences(previous.name.trim(), name)
      : 0;
    setupCharacters.value = setupCharacters.value.map((item) =>
      item.id === editingCharacterId.value ? { ...item, ...character } : item
    );
    if (previous && previous.name.trim() !== name) {
      store.notifyInfo(
        changedReferences > 0
          ? "已同步替换相关设定中的人物名，可能存在语义冲突，请自行检查。"
          : "人物名已修改，请自行检查相关关系和设定是否仍然正确。"
      );
    }
  } else {
    setupCharacters.value = [...setupCharacters.value, character];
  }
  closeCharacterModal();
}

function openRelationshipByGraphEdge(edgeId: string) {
  const index = Number(edgeId.replace("relationship-", ""));
  if (!setupRelationships.value[index]) return;
  selectedCharacterGraphNodeId.value = null;
  editingGraphRelationshipIndex.value = index;
}

function handleCharacterGraphNodeClick(id: string) {
  const node = characterGraphNodes.value.find((item) => item.id === id);
  if (node) selectCharacterGraphNode(node);
}

function handleCharacterGraphEdgeContextMenu(
  edgeId: string,
  event: MouseEvent | TouchEvent
) {
  const edge = characterGraphEdges.value.find((item) => item.id === edgeId);
  if (edge && event instanceof MouseEvent)
    showGraphRelationshipDelete(edge, event);
}

function updateCharacterGraphNodePosition(
  id: string,
  position: { x: number; y: number }
) {
  characterGraphNodePositions.value = {
    ...characterGraphNodePositions.value,
    [id]: position,
  };
}

function openCharacterGraph() {
  selectedCharacterGraphNodeId.value = null;
  editingGraphRelationshipIndex.value = null;
  pendingGraphRelationshipDeleteIndex.value = null;
  relationshipHistory.value = [];
  isCharacterGraphOpen.value = true;
}

function handleGraphKeydown(event: KeyboardEvent) {
  if (event.ctrlKey && event.key === "z") {
    event.preventDefault();
    undoRelationship();
  }
}

function clearCharacterGraphSelection() {
  selectedCharacterGraphNodeId.value = null;
  editingGraphRelationshipIndex.value = null;
  pendingGraphRelationshipDeleteIndex.value = null;
}

function selectCharacterGraphNode(node: CharacterGraphNode) {
  editingGraphRelationshipIndex.value = null;
  if (!selectedCharacterGraphNodeId.value) {
    selectedCharacterGraphNodeId.value = node.id;
    return;
  }
  if (selectedCharacterGraphNodeId.value === node.id) {
    selectedCharacterGraphNodeId.value = null;
    return;
  }
  const exists = setupRelationships.value.some(
    (relationship) =>
      relationship.characterA === selectedCharacterGraphNodeId.value &&
      relationship.characterB === node.id
  );
  if (!exists) {
    pushRelationshipHistory();
    setupRelationships.value = [
      ...setupRelationships.value,
      {
        characterA: selectedCharacterGraphNodeId.value,
        characterB: node.id,
        description: "",
      },
    ];
  }
  selectedCharacterGraphNodeId.value = null;
}

function updateSelectedCharacterGraphCharacter(
  field: keyof Pick<SetupCharacter, "name" | "appearanceTime" | "notes">,
  value: string
) {
  const id = selectedCharacterGraphNodeId.value;
  if (!id) return;
  setupCharacters.value = setupCharacters.value.map((character) =>
    character.id === id ? { ...character, [field]: value } : character
  );
}

function showGraphRelationshipDelete(
  edge: CharacterGraphEdge,
  event: MouseEvent
) {
  event.preventDefault();
  pendingGraphRelationshipDeleteIndex.value = edge.index;
}

function deletePendingGraphRelationship() {
  const index = pendingGraphRelationshipDeleteIndex.value;
  if (index === null) return;
  pushRelationshipHistory();
  setupRelationships.value = setupRelationships.value.filter(
    (_relationship, relationshipIndex) => relationshipIndex !== index
  );
  pendingGraphRelationshipDeleteIndex.value = null;
}

function closeGraphRelationshipEditor() {
  editingGraphRelationshipIndex.value = null;
}

function updateCharacterRelationship(
  index: number,
  field: keyof SetupCharacterRelationship,
  value: string
) {
  pushRelationshipHistory();
  setupRelationships.value = setupRelationships.value.map(
    (relationship, relationshipIndex) =>
      relationshipIndex === index
        ? { ...relationship, [field]: value }
        : relationship
  );
}

function removeSetupCharacter(id: string) {
  confirmSetupDelete("确定删除这个人物设定吗？", () => {
    setupCharacters.value = setupCharacters.value.filter(
      (item) => item.id !== id
    );
    setupRelationships.value = setupRelationships.value.filter(
      (relationship) =>
        relationship.characterA !== id && relationship.characterB !== id
    );
  });
}

function openMapModal(map?: SetupMap) {
  editingMapId.value = map?.id || null;
  mapForm.value = {
    name: map?.name || "",
    appearanceTime: map?.appearanceTime || "前期",
    notes: map?.notes || "",
  };
  isMapModalOpen.value = true;
}

function closeMapModal() {
  isMapModalOpen.value = false;
  editingMapId.value = null;
  mapForm.value = { name: "", appearanceTime: "前期", notes: "" };
  appearanceTimeMenuOpen.value = null;
}

function saveSetupMap() {
  const name = mapForm.value.name.trim();
  const appearanceTime = mapForm.value.appearanceTime.trim();
  const notes = mapForm.value.notes.trim();
  if (!name) {
    store.notifyError("请填写地点名称");
    return;
  }
  if (!appearanceTime) {
    store.notifyError("请填写地点出场时间");
    return;
  }
  if (!notes) {
    store.notifyError("请填写地点详细信息");
    return;
  }
  const map: SetupMap = {
    id: editingMapId.value || `m${Date.now()}`,
    name,
    appearanceTime,
    notes,
  };
  if (editingMapId.value) {
    setupMaps.value = setupMaps.value.map((item) =>
      item.id === editingMapId.value ? map : item
    );
  } else {
    setupMaps.value = [...setupMaps.value, map];
  }
  closeMapModal();
}

function removeSetupMap(id: string) {
  confirmSetupDelete("确定删除这个地点设定吗？", () => {
    setupMaps.value = setupMaps.value.filter((item) => item.id !== id);
  });
}

function normalizedOtherSettingTitle(title: string): string {
  const value = title.trim();
  if (["货币", "货币设定", "钱币", "金钱"].includes(value)) {
    return "货币";
  }
  return value;
}

function openCharacterList() {
  isCharacterListOpen.value = true;
}

function openMapList() {
  isMapListOpen.value = true;
}

function openOtherSettingsList() {
  isOtherSettingsOpen.value = true;
}

function openFixedOtherItems() {
  isFixedOtherItemsOpen.value = true;
}

function openFixedOtherItemModal(item?: SetupForce) {
  isForceItemModal.value = true;
  editingForceId.value = item?.id || null;
  forceForm.value = {
    name: item?.name || "",
    appearanceTime: item?.appearanceTime || "前期",
    notes: item?.notes || "",
  };
  isOtherItemModalOpen.value = true;
}

function openOtherSettingModal(setting?: SetupOtherSetting) {
  editingOtherSettingId.value = setting?.id || null;
  otherSettingForm.value = {
    title: setting?.title || "",
    description: setting?.description || "",
  };
  isOtherSettingModalOpen.value = true;
}

function closeOtherSettingModal() {
  isOtherSettingModalOpen.value = false;
  editingOtherSettingId.value = null;
  otherSettingForm.value = { title: "", description: "" };
}

function saveSetupOtherSetting() {
  const title = normalizedOtherSettingTitle(otherSettingForm.value.title);
  if (!title) {
    store.notifyError("请填写设定名称");
    return;
  }
  const nextSetting: SetupOtherSetting = {
    id: editingOtherSettingId.value || `os${Date.now()}`,
    title,
    description: otherSettingForm.value.description.trim(),
    items:
      setupOtherSettings.value.find(
        (item) => item.id === editingOtherSettingId.value
      )?.items || [],
  };
  if (editingOtherSettingId.value) {
    setupOtherSettings.value = setupOtherSettings.value.map((item) =>
      item.id === editingOtherSettingId.value ? nextSetting : item
    );
  } else {
    setupOtherSettings.value = [...setupOtherSettings.value, nextSetting];
    activeOtherSettingId.value = nextSetting.id;
  }
  closeOtherSettingModal();
}

function removeSetupOtherSetting(id: string) {
  confirmSetupDelete(
    "确定删除这个设定类型吗？其中的设定内容也会一并删除。",
    () => {
      setupOtherSettings.value = setupOtherSettings.value.filter(
        (item) => item.id !== id
      );
      if (activeOtherSettingId.value === id) {
        activeOtherSettingId.value = setupOtherSettings.value[0]?.id || null;
      }
    }
  );
}

function selectOtherSetting(id: string) {
  activeOtherSettingId.value = activeOtherSettingId.value === id ? null : id;
}

function openOtherItemModal(settingId: string, item?: SetupOtherItem) {
  activeOtherSettingId.value = settingId;
  editingOtherItemId.value = item?.id || null;
  otherItemForm.value = {
    name: item?.name || "",
    notes: item?.notes || "",
    appearanceTime: item?.appearanceTime || "前期",
  };
  isOtherItemModalOpen.value = true;
}

function closeOtherItemModal() {
  isOtherItemModalOpen.value = false;
  isForceItemModal.value = false;
  editingForceId.value = null;
  editingOtherItemId.value = null;
  forceForm.value = { name: "", appearanceTime: "前期", notes: "" };
  otherItemForm.value = { name: "", notes: "", appearanceTime: "前期" };
  appearanceTimeMenuOpen.value = null;
}

// setupItemFormName 读取当前设定内容弹窗正在编辑的名称。
function setupItemFormName() {
  return isForceItemModal.value
    ? forceForm.value.name
    : otherItemForm.value.name;
}

// setupItemFormNotes 读取当前设定内容弹窗正在编辑的详细信息。
function setupItemFormNotes() {
  return isForceItemModal.value
    ? forceForm.value.notes
    : otherItemForm.value.notes;
}

// setupItemFormAppearanceTime 读取当前设定内容弹窗正在编辑的出场时间。
function setupItemFormAppearanceTime() {
  return isForceItemModal.value
    ? forceForm.value.appearanceTime
    : otherItemForm.value.appearanceTime;
}

function appearanceTimeValue(target: AppearanceTimeTarget) {
  if (target === "character") return characterForm.value.appearanceTime;
  if (target === "map") return mapForm.value.appearanceTime;
  return setupItemFormAppearanceTime();
}

function toggleAppearanceTimeMenu(target: AppearanceTimeTarget) {
  appearanceTimeMenuOpen.value =
    appearanceTimeMenuOpen.value === target ? null : target;
}

function selectAppearanceTime(target: AppearanceTimeTarget, value: string) {
  if (target === "character") {
    characterForm.value.appearanceTime = value;
  } else if (target === "map") {
    mapForm.value.appearanceTime = value;
  } else {
    updateSetupItemFormAppearanceTime(value);
  }
  appearanceTimeMenuOpen.value = null;
}

// updateSetupItemFormName 根据弹窗类型更新势力或其他设定名称。
function updateSetupItemFormName(value: string) {
  if (isForceItemModal.value) {
    forceForm.value = { ...forceForm.value, name: value };
  } else {
    otherItemForm.value = { ...otherItemForm.value, name: value };
  }
}

// updateSetupItemFormNotes 根据弹窗类型更新势力或其他设定详细信息。
function updateSetupItemFormNotes(value: string) {
  if (isForceItemModal.value) {
    forceForm.value = { ...forceForm.value, notes: value };
  } else {
    otherItemForm.value = { ...otherItemForm.value, notes: value };
  }
}

// updateSetupItemFormAppearanceTime 根据弹窗类型更新势力或其他设定出场时间。
function updateSetupItemFormAppearanceTime(value: string) {
  if (isForceItemModal.value) {
    forceForm.value = { ...forceForm.value, appearanceTime: value };
  } else {
    otherItemForm.value = { ...otherItemForm.value, appearanceTime: value };
  }
}

function saveSetupOtherItem() {
  if (isForceItemModal.value) {
    saveSetupForce();
    return;
  }
  const settingId = activeOtherSettingId.value;
  const name = otherItemForm.value.name.trim();
  const notes = otherItemForm.value.notes.trim();
  const appearanceTime = otherItemForm.value.appearanceTime.trim();
  if (!settingId) return;
  if (!name) {
    store.notifyError("请填写设定内容名称");
    return;
  }
  if (!notes) {
    store.notifyError("请填写设定详细信息");
    return;
  }
  if (!appearanceTime) {
    store.notifyError("请填写设定出场时间");
    return;
  }
  const nextItem: SetupOtherItem = {
    id: editingOtherItemId.value || `oi${Date.now()}`,
    name,
    notes,
    appearanceTime,
  };
  setupOtherSettings.value = setupOtherSettings.value.map((setting) => {
    if (setting.id !== settingId) return setting;
    const items = editingOtherItemId.value
      ? setting.items.map((item) =>
          item.id === editingOtherItemId.value ? nextItem : item
        )
      : [...setting.items, nextItem];
    return { ...setting, items };
  });
  closeOtherItemModal();
}

// saveSetupForce 保存独立的势力设定项。
function saveSetupForce() {
  const name = forceForm.value.name.trim();
  const appearanceTime = forceForm.value.appearanceTime.trim();
  const notes = forceForm.value.notes.trim();
  if (!name) {
    store.notifyError("请填写势力名称");
    return;
  }
  if (!appearanceTime) {
    store.notifyError("请填写势力出场时间");
    return;
  }
  if (!notes) {
    store.notifyError("请填写势力详细信息");
    return;
  }
  const force: SetupForce = {
    id: editingForceId.value || `f${Date.now()}`,
    name,
    appearanceTime,
    notes,
  };
  if (editingForceId.value) {
    setupForces.value = setupForces.value.map((item) =>
      item.id === editingForceId.value ? force : item
    );
  } else {
    setupForces.value = [...setupForces.value, force];
  }
  closeOtherItemModal();
}

// removeSetupForce 删除独立的势力设定项。
function removeSetupForce(id: string) {
  confirmSetupDelete("确定删除这个势力设定吗？", () => {
    setupForces.value = setupForces.value.filter((item) => item.id !== id);
  });
}

function removeSetupOtherItem(settingId: string, itemId: string) {
  confirmSetupDelete("确定删除这个设定内容吗？", () => {
    setupOtherSettings.value = setupOtherSettings.value.map((setting) =>
      setting.id === settingId
        ? {
            ...setting,
            items: setting.items.filter((item) => item.id !== itemId),
          }
        : setting
    );
  });
}

function openSetupAssistModal() {
  if (!setupAssistText.value.trim() && setupOriginalText.value.trim()) {
    setupAssistText.value = setupOriginalText.value;
  }
  setupAssistModelId.value = store.generalSettings.modelId;
  isSetupAssistOpen.value = true;
  if (store.models.length === 0) void store.loadModels();
}

function closeSetupAssistModal() {
  if (isSetupAssistLoading.value) {
    cancelSetupAssistGeneration();
    return;
  }
  isSetupAssistOpen.value = false;
  setupAssistModelOpen.value = false;
}

// closeSetupAssistModalByBackdrop 在生成中忽略遮罩点击，避免误取消流式生成。
function closeSetupAssistModalByBackdrop() {
  if (isSetupAssistLoading.value) return;
  closeSetupAssistModal();
}

function cancelSetupAssistGeneration() {
  setupAssistAbortController?.abort();
  setupAssistAbortController = null;
  isSetupAssistLoading.value = false;
  setupAssistThinkingText.value = "";
  setupAssistStartedAt.value = null;
  isSetupAssistOpen.value = false;
  setupAssistModelOpen.value = false;
}

function onSetupAssistStreamEvent(event: StreamEvent) {
  const ui = streamA2UIData(event);
  if (ui?.data?.kind !== "novel_setup_step") return;
  const text = ui.data?.text?.trim();
  if (text) setupAssistThinkingText.value = text;
}

function onSetupAssistFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  appendSetupAssistFiles(input.files);
  input.value = "";
}

function onSetupAssistDrop(event: DragEvent) {
  appendSetupAssistFiles(event.dataTransfer?.files);
}

function appendSetupAssistFiles(files?: FileList | File[] | null) {
  if (!files) return;
  const allowed = new Set([".txt", ".md", ".docx"]);
  const next = [...setupAssistFiles.value];
  for (const file of Array.from(files)) {
    const suffix = file.name.slice(file.name.lastIndexOf(".")).toLowerCase();
    if (!allowed.has(suffix)) {
      store.notifyError("暂时只支持 txt、md 和 docx 文件");
      continue;
    }
    const exists = next.some(
      (item) =>
        item.name === file.name &&
        item.size === file.size &&
        item.lastModified === file.lastModified
    );
    if (!exists) next.push(file);
  }
  setupAssistFiles.value = next;
}

function removeSetupAssistFile(index: number) {
  setupAssistFiles.value = setupAssistFiles.value.filter((_, i) => i !== index);
}

function downloadSetupAssistFile(file: File) {
  const url = URL.createObjectURL(file);
  const link = document.createElement("a");
  link.href = url;
  link.download = file.name;
  link.click();
  URL.revokeObjectURL(url);
}

async function completeSetupFromAssist(payload?: {
  text: string;
  files: File[];
  modelId: number;
}) {
  const nextPayload = payload || {
    text: setupAssistText.value.trim(),
    files: [...setupAssistFiles.value],
    modelId: setupAssistModelId.value || store.generalSettings.modelId,
  };
  if (!nextPayload.text && nextPayload.files.length === 0) {
    store.notifyError("请先输入想法或上传文件");
    return;
  }
  isSetupAssistLoading.value = true;
  setupAssistStartedAt.value = Date.now();
  setupAssistThinkingText.value = "正在启动小说模板生成";
  setupAssistAbortController?.abort();
  const controller = new AbortController();
  setupAssistAbortController = controller;
  try {
    const setup = await completeNovelSetupStreamApi(
      nextPayload,
      onSetupAssistStreamEvent,
      controller.signal
    );
    syncSetupAssistPayload(nextPayload, setup.originalText);
    applyCompletedSetup(setup);
    isSetupAssistOpen.value = false;
    store.notifyInfo("已生成具体模板");
  } catch (err) {
    if (
      (err instanceof DOMException && err.name === "AbortError") ||
      (err instanceof Error && err.name === "AbortError")
    ) {
      return;
    }
    store.notifyError(err instanceof Error ? err.message : "生成小说模板失败");
  } finally {
    if (setupAssistAbortController === controller) {
      setupAssistAbortController = null;
      isSetupAssistLoading.value = false;
      setupAssistThinkingText.value = "";
      setupAssistStartedAt.value = null;
    }
  }
}

function regenerateSetupTemplate() {
  if (isSetupAssistLoading.value) {
    cancelSetupAssistGeneration();
    return;
  }
  if (!setupAssistLastPayload.value) {
    openSetupAssistModal();
    return;
  }
  void completeSetupFromAssist(setupAssistLastPayload.value);
}

function reopenSetupAssistModal() {
  if (setupAssistLastPayload.value) {
    setupAssistText.value = setupAssistLastPayload.value.text;
    setupAssistFiles.value = [...setupAssistLastPayload.value.files];
  } else if (setupOriginalText.value.trim()) {
    setupAssistText.value = setupOriginalText.value;
    setupAssistFiles.value = [];
  }
  setupAssistModelId.value =
    setupAssistLastPayload.value?.modelId || store.generalSettings.modelId;
  isSetupAssistOpen.value = true;
  if (store.models.length === 0) void store.loadModels();
}

function selectSetupAssistModel(modelId: number) {
  setupAssistModelId.value = modelId;
  setupAssistModelOpen.value = false;
}

function openSetupAssistMoreModels() {
  setupAssistModelOpen.value = false;
  store.openCustomModelSettings();
}

function selectedEditorText() {
  const textarea = editorTextarea.value;
  if (!textarea) return "";
  const start = textarea.selectionStart;
  const end = textarea.selectionEnd;
  if (end <= start) return "";
  return textarea.value.slice(start, end).trim();
}

function fillFindTextFromSelection() {
  const selected = selectedEditorText();
  if (selected) {
    findText.value = selected;
    activeFindIndex.value = 0;
  }
}

function applyCompletedSetup(setup: NovelSetupData) {
  if (setup.originalText?.trim()) {
    setupOriginalText.value = setup.originalText.trim();
    setupAssistText.value = setupOriginalText.value;
  }
  setupTitle.value = setup.title || setupTitle.value;
  setupDirection.value = setup.direction || setupDirection.value;
  if (setupLengthOptions.some((item) => item.label === setup.length)) {
    setupLength.value = setup.length;
  }
  if (
    setupPerspectiveOptions.some((item) => item.label === setup.perspective)
  ) {
    setupPerspective.value = setup.perspective || setupPerspective.value;
  }
  applyCompletedTags(setup.tagGroups || {});
  setupCharacters.value = (setup.characters || [])
    .filter((item) => item.name?.trim())
    .map((item, index) => ({
      id: setupCharacterStableId(index),
      name: item.name.trim(),
      appearanceTime: item.appearanceTime?.trim() || "",
      notes: item.notes?.trim() || "",
    }));
  setupRelationships.value = normalizeCompletedSetupRelationships(
    setup.relationships || []
  );
  setupMaps.value = (setup.maps || [])
    .filter((item) => item.name?.trim())
    .map((item, index) => ({
      id: `map-${Date.now()}-${index}`,
      name: item.name.trim(),
      appearanceTime: item.appearanceTime?.trim() || "",
      notes: item.notes?.trim() || "",
    }));
  setupForces.value = (setup.forces || [])
    .filter((item) => item.name?.trim())
    .map((item, index) => ({
      id: `force-${Date.now()}-${index}`,
      name: item.name.trim(),
      appearanceTime: item.appearanceTime?.trim() || "",
      notes: item.notes?.trim() || "",
    }));
  setupOtherSettings.value = (setup.other_settings || [])
    .map((item, index) => ({
      id: `other-${Date.now()}-${index}`,
      title: item.title.trim(),
      description: item.description?.trim() || "",
      items: item.items
        .filter((child) => child.name?.trim())
        .map((child, childIndex) => ({
          id: `other-item-${Date.now()}-${index}-${childIndex}`,
          name: child.name.trim(),
          notes: child.notes?.trim() || "",
          appearanceTime: child.appearanceTime?.trim() || "",
        })),
    }))
    .filter((item) => item.title && item.items.length > 0);
}

function applyCompletedTags(tagGroups: Record<string, string[]>) {
  setupTags.value = [];
  setupCustomAddedTags.value = {};
  setupCustomTags.value = {};
  for (const group of setupTagGroups) {
    const values = tagGroups[group.title] || [];
    const unknown: string[] = [];
    for (const value of values) {
      if (group.tags.includes(value)) {
        setupTags.value.push(value);
      } else if (value.trim()) {
        unknown.push(value.trim());
      }
    }
    if (unknown.length > 0) {
      setupCustomTags.value[group.title] = unknown.join("、");
    }
  }
}

function setupCharacterStableId(index: number): string {
  return `char_${String(index + 1).padStart(3, "0")}`;
}

function normalizeCompletedSetupRelationships(
  relationships: Array<Record<string, unknown>>
): SetupCharacterRelationship[] {
  const characterById = new Set(
    setupCharacters.value.map((character) => character.id)
  );
  const characterIdByName = new Map(
    setupCharacters.value.map((character) => [
      character.name.trim(),
      character.id,
    ])
  );
  return relationships.flatMap((relationship) => {
    const characterA = normalizeRelationshipCharacterId(
      setupString(
        relationship.characterA ||
          relationship.character_a ||
          relationship.source ||
          relationship.from
      ),
      characterById,
      characterIdByName
    );
    const characterB = normalizeRelationshipCharacterId(
      setupString(
        relationship.characterB ||
          relationship.character_b ||
          relationship.target ||
          relationship.to
      ),
      characterById,
      characterIdByName
    );
    if (!characterA || !characterB || characterA === characterB) return [];
    return [
      {
        characterA,
        characterB,
        description: setupString(relationship.description),
      },
    ];
  });
}

function normalizeRelationshipCharacterId(
  value: string,
  characterById: Set<string>,
  characterIdByName: Map<string, string>
) {
  const trimmed = value.trim();
  if (!trimmed) return "";
  if (characterById.has(trimmed)) return trimmed;
  return characterIdByName.get(trimmed) || "";
}

function resetNovelSetupForm() {
  setupTitle.value = "";
  setupDirection.value = "";
  setupOriginalText.value = "";
  setupLength.value = "中篇";
  setupPerspective.value = "第三人称";
  setupTags.value = [];
  setupCustomTags.value = {};
  setupCustomAddedTags.value = {};
  setupCharacters.value = [];
  setupRelationships.value = [];
  setupMaps.value = [];
  setupForces.value = [];
  setupOtherSettings.value = [];
  setupAssistLastPayload.value = null;
  savedNovelSetupSignature.value = currentNovelSetupSignature.value;
}

function formatSetupNamedList(
  items: Array<{ name: string; appearanceTime: string; notes: string }>
) {
  if (items.length === 0) return "";
  return items
    .map((item) => {
      const notes = item.notes.trim();
      const appearanceTime = item.appearanceTime.trim();
      const suffix = [
        appearanceTime ? `出场时间：${appearanceTime}` : "",
        notes,
      ]
        .filter(Boolean)
        .join("；");
      return `- ${item.name.trim()}${suffix ? `：${suffix}` : ""}`;
    })
    .join("\n");
}

// formatSetupForcesMarkdown 将势力设定整理为提交给小说规划的 Markdown。
function formatSetupForcesMarkdown() {
  if (setupForces.value.length === 0) return "";
  return formatSetupNamedList(setupForces.value);
}

function formatSetupRelationshipsMarkdown() {
  if (setupRelationships.value.length === 0) return "";
  const characters = new Map(
    setupCharacters.value.map((character) => [character.id, character.name])
  );
  return setupRelationships.value
    .map((relationship) => {
      const characterA = characters.get(relationship.characterA);
      const characterB = characters.get(relationship.characterB);
      if (!characterA || !characterB) return "";
      const description = relationship.description?.trim();
      return `- ${characterA} ↔ ${characterB}${
        description ? `：${description}` : ""
      }`;
    })
    .filter(Boolean)
    .join("\n");
}

function formatCustomOtherSettingsMarkdown() {
  if (setupOtherSettings.value.length === 0) return "";
  const lines: string[] = [];
  for (const setting of setupOtherSettings.value) {
    const title = setting.title.trim();
    if (!title) continue;
    const description = setting.description.trim();
    lines.push(`- ${title}${description ? `：${description}` : ""}`);
    for (const item of setting.items) {
      const name = item.name.trim();
      if (!name) continue;
      const notes = item.notes.trim();
      const appearanceTime = item.appearanceTime?.trim();
      const suffix = [
        notes,
        appearanceTime ? `出场时间：${appearanceTime}` : "",
      ]
        .filter(Boolean)
        .join("；");
      lines.push(`  - ${name}${suffix ? `：${suffix}` : ""}`);
    }
  }
  return lines.length > 0 ? lines.join("\n") : "";
}

function buildNovelSetupPrompt() {
  const length = setupLengthOptions.find(
    (item) => item.label === setupLength.value
  );
  const tagLines = setupTagGroupLines();
  const charactersText = formatSetupNamedList(setupCharacters.value);
  const relationshipsText = formatSetupRelationshipsMarkdown();
  const mapsText = formatSetupNamedList(setupMaps.value);
  const forceText = formatSetupForcesMarkdown();
  const otherText = formatCustomOtherSettingsMarkdown();
  const sections: string[] = [
    "请根据下面的新建小说设定，先简单说说你的规划思路，然后直接给出全书分卷规划。",
    "",
    "## 基础信息",
    `- 小说标题：${setupTitle.value.trim()}`,
    `- 叙事视角：${setupPerspective.value}`,
    `- 篇幅：${setupLength.value}（${length?.desc || ""}）`,
    "",
    "## 核心设定",
    setupDirection.value.trim(),
  ];
  if (charactersText) {
    sections.push("", "## 人物设定", charactersText);
  }
  if (relationshipsText) {
    sections.push("", "## 人物关系", relationshipsText);
  }
  if (mapsText) {
    sections.push("", "## 地点设定", mapsText);
  }
  if (forceText) {
    sections.push("", "## 势力设定", forceText);
  }
  if (otherText) {
    sections.push("", "## 其他设定", otherText);
  }
  if (tagLines.length > 0) {
    sections.push("", "## 标签分组", ...tagLines.map((line) => `- ${line}`));
  }
  return sections.join("\n");
}

function setupTagsByGroup() {
  const result: Record<string, string[]> = {};
  for (const group of setupTagGroups) {
    const selected = group.tags.filter((tag) => setupTags.value.includes(tag));
    const added = setupCustomAddedTags.value[group.title] || [];
    const editing = setupCustomTags.value[group.title]?.trim();
    const values = Array.from(
      new Set([...selected, ...added, ...(editing ? [editing] : [])])
    );
    if (values.length > 0) result[group.title] = values;
  }
  return result;
}

function setupTagGroupLines() {
  return Object.entries(setupTagsByGroup()).map(
    ([group, tags]) => `${group}：${tags.join("、")}`
  );
}

function buildNovelSetupData() {
  const length = setupLengthOptions.find(
    (item) => item.label === setupLength.value
  );
  return {
    originalText: setupOriginalText.value.trim(),
    title: setupTitle.value.trim(),
    direction: setupDirection.value.trim(),
    tagGroups: setupTagsByGroup(),
    characters: setupCharacters.value.map(
      ({ name, appearanceTime, notes }) => ({
        name,
        appearanceTime,
        notes,
      })
    ),
    relationships: setupRelationships.value
      .filter(
        (relationship) => relationship.characterA && relationship.characterB
      )
      .map(({ characterA, characterB, description }) => ({
        characterA,
        characterB,
        description,
      })),
    maps: setupMaps.value.map(({ name, appearanceTime, notes }) => ({
      name,
      appearanceTime,
      notes,
    })),
    forces: setupForces.value.map(({ name, appearanceTime, notes }) => ({
      name,
      appearanceTime,
      notes,
    })),
    other_settings: setupOtherSettings.value
      .filter((setting) => setting.items.length > 0)
      .map(({ title, description, items }) => ({
        title: normalizedOtherSettingTitle(title),
        description,
        items: items.map(({ name, notes, appearanceTime }) => ({
          name,
          notes,
          appearanceTime,
        })),
      })),
    perspective: setupPerspective.value,
    length: setupLength.value,
    lengthRange: length?.desc || "",
  };
}

function setupString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function setupAppearanceTime(value: unknown): string {
  const text = setupString(value).trim();
  return ["前期", "中期", "后期"].includes(text) ? text : "前期";
}

function setupArray(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value)
    ? value.filter(
        (item): item is Record<string, unknown> =>
          !!item && typeof item === "object"
      )
    : [];
}

function applyNovelSetupPlan(
  planData: Record<string, unknown>,
  originalText = ""
) {
  setupOriginalText.value = originalText;
  setupAssistText.value = setupOriginalText.value;
  setupAssistFiles.value = [];
  setupAssistLastPayload.value = setupOriginalText.value.trim()
    ? {
        text: setupOriginalText.value,
        files: [],
        modelId: store.generalSettings.modelId,
      }
    : null;
  setupTitle.value = setupString(planData.title);
  setupDirection.value = setupString(planData.direction || planData.summary);
  setupLength.value = setupString(planData.length) || "中篇";
  setupPerspective.value = setupString(planData.perspective) || "第三人称";
  setupTags.value = [];
  setupCustomTags.value = {};
  setupCustomAddedTags.value = {};
  applyCompletedTags(setupTagGroupsFromPlan(planData.tag_groups));
  setupCharacters.value = setupArray(planData.characters).map(
    (item, index) => ({
      id: setupCharacterStableId(index),
      name: setupString(item.name),
      appearanceTime: setupAppearanceTime(item.appearance_time),
      notes: setupString(item.notes),
    })
  );
  setupRelationships.value = setupArray(planData.relationships).map((item) => ({
    characterA: setupString(item.character_a || item.characterA),
    characterB: setupString(item.character_b || item.characterB),
    description: setupString(item.description),
  }));
  setupMaps.value = setupArray(planData.maps).map((item, index) => ({
    id: `draft-map-${index}`,
    name: setupString(item.name),
    appearanceTime: setupAppearanceTime(item.appearance_time),
    notes: setupString(item.notes),
  }));
  setupForces.value = setupArray(planData.forces).map((item, index) => ({
    id: `draft-force-${index}`,
    name: setupString(item.name),
    appearanceTime: setupAppearanceTime(item.appearance_time),
    notes: setupString(item.notes),
  }));
  setupOtherSettings.value = setupArray(planData.other_settings).map(
    (setting, index) => ({
      id: `draft-other-${index}`,
      title: setupString(setting.title),
      description: setupString(setting.description),
      items: setupArray(setting.items).map((item, itemIndex) => ({
        id: `draft-other-item-${index}-${itemIndex}`,
        name: setupString(item.name),
        appearanceTime: setupAppearanceTime(item.appearance_time),
        notes: setupString(item.notes),
      })),
    })
  );
}

function setupTagGroupsFromPlan(value: unknown): Record<string, string[]> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  const result: Record<string, string[]> = {};
  for (const [key, item] of Object.entries(value)) {
    if (Array.isArray(item)) {
      result[key] = item.filter(
        (tag): tag is string => typeof tag === "string"
      );
    }
  }
  return result;
}

async function submitNovelSetup() {
  if (isSetupAssistLoading.value) {
    store.notifyInfo("模板还在生成中，请稍后再开始创建");
    return;
  }
  if (!setupTitle.value.trim()) {
    store.notifyError("请填写小说名称");
    return;
  }
  if (!setupDirection.value.trim()) {
    store.notifyError("请填写小说的具体走向");
    return;
  }
  await store.createNovelFromSetup(
    buildNovelSetupPrompt(),
    buildNovelSetupData()
  );
  resetNovelSetupForm();
}

async function saveNovelSetupDraft() {
  if (isSetupAssistLoading.value) {
    store.notifyInfo("模板还在生成中，请稍后再暂存");
    return;
  }
  if (!setupTitle.value.trim()) {
    store.notifyError("请填写小说名称");
    return;
  }
  const savedId = await store.saveNovelSetupDraft(buildNovelSetupData());
  if (savedId) {
    savedNovelSetupSignature.value = currentNovelSetupSignature.value;
  }
}

watch(
  () =>
    [
      store.isNovelSetupOpen,
      store.setupDraftNovelId,
      store.selectedNovel?.updatedAt?.getTime(),
    ] as const,
  () => {
    if (!store.isNovelSetupOpen) return;
    if (!store.setupDraftNovelId) {
      savedNovelSetupSignature.value = currentNovelSetupSignature.value;
      return;
    }
    if (!store.selectedNovel) return;
    applyNovelSetupPlan(
      store.selectedNovel.planData as Record<string, unknown>,
      store.selectedNovel.setupOriginalText || ""
    );
    nextTick(() => {
      savedNovelSetupSignature.value = currentNovelSetupSignature.value;
    });
  },
  { immediate: true }
);

watch(
  () => store.novelSetupFormResetTick,
  () => {
    if (!store.isNovelSetupOpen || store.setupDraftNovelId) return;
    resetNovelSetupForm();
  }
);

watch(activeMessageScrollKey, () => {
  if (shouldScrollToLatestQuestion.value) {
    shouldScrollToLatestQuestion.value = false;
    scrollToLatestQuestion();
  } else if (shouldAutoScrollMessages.value) {
    scrollToBottom();
  }
  nextTick(updateActiveQuestion);
});

const chatInputPlaceholder = computed(() => {
  if (!store.selectedChapterId) return store.activeChatPlaceholder;
  return chapterGraphMode.value
    ? "高一致性模式：会根据上下文校验正文一致性"
    : "输入你的想法";
});

watch(
  () =>
    store.activeMessages.map((message) => ({
      id: message.id,
      thinking: shouldShowThinkingStatus(message),
      timestamp: message.timestamp.getTime(),
    })),
  (states) => {
    const nextStarts: Record<string, number> = {};
    for (const state of states) {
      if (!state.thinking) continue;
      nextStarts[state.id] =
        messageThinkingStarts.value[state.id] || state.timestamp;
    }
    messageThinkingStarts.value = nextStarts;
  },
  { immediate: true }
);

watch(
  () =>
    store.activeMessages.map((message) => ({
      id: message.id,
      hasProgress: Boolean(message.chapterGeneration),
      steps: message.chapterGeneration?.steps || [],
      text: message.chapterGeneration?.text || "",
      stage: message.chapterGeneration?.stage || "",
      currentStepLabel: message.chapterGeneration?.currentStepLabel || "",
      currentStepStartedAt:
        message.chapterGeneration?.currentStepStartedAt?.getTime(),
      complete: message.chapterGeneration?.complete === true,
      collapsed: message.chapterGeneration?.collapsed === true,
    })),
  (states) => {
    const nextStarts: Record<string, number> = {};
    const nextStepLabels: Record<string, string> = {};
    const nextSteps: Record<string, string[]> = {};
    const nextCompleted: Record<string, number> = {};
    for (const state of states) {
      if (!state.hasProgress || state.collapsed) continue;
      const incomingSteps =
        state.steps.length > 0 ? state.steps : [state.text || "正在处理"];
      const currentStepLabel =
        state.currentStepLabel ||
        chapterGenerationStageStep(state.stage) ||
        incomingSteps[incomingSteps.length - 1] ||
        "";
      nextSteps[state.id] = incomingSteps;
      nextStepLabels[state.id] = currentStepLabel;
      nextStarts[state.id] =
        state.currentStepStartedAt ||
        (chapterProgressStepLabels.value[state.id] === currentStepLabel
          ? chapterProgressStarts.value[state.id] || thinkingNow.value
          : thinkingNow.value);
      if (state.complete) {
        nextCompleted[state.id] =
          chapterProgressCompletedAt.value[state.id] || thinkingNow.value;
      }
    }
    chapterProgressStarts.value = nextStarts;
    chapterProgressStepLabels.value = nextStepLabels;
    chapterProgressSteps.value = nextSteps;
    chapterProgressCompletedAt.value = nextCompleted;
  },
  { immediate: true }
);

function formatTime(date: Date): string {
  const now = new Date();
  const isToday = date.toDateString() === now.toDateString();
  const isThisYear = date.getFullYear() === now.getFullYear();
  const time = date.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });

  if (isToday) return time;
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (date.toDateString() === yesterday.toDateString()) return `昨天 ${time}`;
  if (isThisYear) return `${date.getMonth() + 1}月${date.getDate()}日 ${time}`;
  return `${date.getFullYear()}年${
    date.getMonth() + 1
  }月${date.getDate()}日 ${time}`;
}

function formatRelativeTime(date: Date): string {
  const seconds = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000));
  if (seconds < 60) return "刚刚";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} 分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days} 天前`;
  return formatTime(date);
}

function formatDashboardNumber(value: number): string {
  return new Intl.NumberFormat("zh-CN").format(value);
}

const markdownCache = new Map<string, string>();
const MARKDOWN_CACHE_MAX = 200;

function convertMarkdownTables(content: string): string {
  const lines = content.split("\n");
  const result: string[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (!/^\|.+\|$/.test(line.trim())) {
      result.push(line);
      i++;
      continue;
    }
    if (i + 1 >= lines.length || !/^\|[\s\-:]+\|$/.test(lines[i + 1].trim())) {
      result.push(line);
      i++;
      continue;
    }
    // 找到完整的表格
    const tableStart = i;
    i += 2;
    while (i < lines.length && /^\|.+\|$/.test(lines[i].trim())) {
      i++;
    }
    const headerCells = lines[tableStart]
      .split("|")
      .filter((c) => c.trim() !== "")
      .map((c) => c.trim());
    const bodyLines = lines.slice(tableStart + 2, i);
    const html: string[] = ["<table>", "<thead><tr>"];
    for (const cell of headerCells) {
      html.push(`<th>${cell}</th>`);
    }
    html.push("</tr></thead><tbody>");
    for (const row of bodyLines) {
      const cells = row
        .split("|")
        .filter((c) => c.trim() !== "")
        .map((c) => c.trim());
      html.push("<tr>");
      for (const cell of cells) {
        html.push(`<td>${cell}</td>`);
      }
      html.push("</tr>");
    }
    html.push("</tbody></table>");
    result.push(html.join(""));
  }
  return result.join("\n");
}

function renderMarkdown(content: string): string {
  const cached = markdownCache.get(content);
  if (cached) return cached;
  const formatted = (content || "")
    .replace(/([。！？?：:])(?=\d+[.．、]\s*\*\*)/g, "$1\n\n")
    .replace(/([。！？?：:])(?=\d+[.．、]\s*[\u4e00-\u9fa5])/g, "$1\n\n")
    .replace(/(?<!^)(?=\d+[.．、]\s*(?:\*\*|[\u4e00-\u9fa5]))/gm, "\n");
  const result = DOMPurify.sanitize(
    markdown.render(convertMarkdownTables(formatted))
  );
  if (markdownCache.size >= MARKDOWN_CACHE_MAX) {
    const firstKey = markdownCache.keys().next().value;
    if (firstKey !== undefined) markdownCache.delete(firstKey);
  }
  markdownCache.set(content, result);
  return result;
}

const questionNavItems = computed(() =>
  store.activeMessages
    .map((message, index) => ({ message, index }))
    .filter(
      (item) => item.message.role === "user" && item.message.content.trim()
    )
    .map((item, questionIndex) => ({
      id: item.message.id,
      index: item.index,
      label: `${questionIndex + 1}. ${item.message.content
        .trim()
        .replace(/\s+/g, " ")
        .slice(0, 42)}`,
    }))
);

const showQuestionNav = computed(
  () =>
    store.viewMode === "chat" &&
    !store.isNovelSetupChoiceOpen &&
    !store.isNovelSetupOpen &&
    !previewDraft.value &&
    questionNavItems.value.length >= 4
);

function scrollToMessage(
  messageId: string,
  block: ScrollLogicalPosition = "center"
) {
  const element = document.querySelector(`[data-message-id="${messageId}"]`);
  element?.scrollIntoView({ behavior: "smooth", block });
}

function updateActiveQuestion() {
  if (!messagesContainer.value || questionNavItems.value.length === 0) return;
  const containerRect = messagesContainer.value.getBoundingClientRect();
  const anchorY = containerRect.top + containerRect.height * 0.35;
  let active = questionNavItems.value[0].id;
  for (const item of questionNavItems.value) {
    const element = document.querySelector(`[data-message-id="${item.id}"]`);
    if (!element) continue;
    const rect = element.getBoundingClientRect();
    if (rect.top <= anchorY) {
      active = item.id;
    } else {
      break;
    }
  }
  activeQuestionMessageId.value = active;
}

const previewMessage = computed(() =>
  store.activeMessages.find((item) => item.id === previewMessageId.value)
);
const previewDraft = computed(
  () => previewDraftCache.value || previewMessage.value?.chapterDraft
);
const previewWordCount = computed(
  () => (previewDraft.value?.content || "").replace(/\s/g, "").length
);
const previewMessageIndex = computed(() =>
  store.activeMessages.findIndex((item) => item.id === previewMessageId.value)
);
const previousDraftMessage = computed(() => {
  for (let i = previewMessageIndex.value - 1; i >= 0; i--) {
    if (store.activeMessages[i]?.chapterDraft) return store.activeMessages[i];
  }
  return null;
});
const nextDraftMessage = computed(() => {
  for (
    let i = previewMessageIndex.value + 1;
    i < store.activeMessages.length;
    i++
  ) {
    if (store.activeMessages[i]?.chapterDraft) return store.activeMessages[i];
  }
  return null;
});
const latestDraftMessage = computed(() => {
  for (let i = store.activeMessages.length - 1; i >= 0; i--) {
    if (store.activeMessages[i]?.chapterDraft) return store.activeMessages[i];
  }
  return null;
});

watch(
  () => latestDraftMessage.value?.chapterDraft?.content.length || 0,
  () => {
    if (!shouldAutoScrollDraft.value) return;
    nextTick(() => {
      if (streamingDraftContainer.value) {
        streamingDraftContainer.value.scrollTop =
          streamingDraftContainer.value.scrollHeight;
      }
    });
  }
);

const activeTargetKey = computed(
  () =>
    store.selectedChapterId ||
    store.selectedVolumeId ||
    store.selectedNovelId ||
    ""
);

watch(activeTargetKey, () => {
  closeDraftPreview();
  shouldScrollToLatestQuestion.value = false;
  shouldAutoScrollMessages.value = true;
  messageAutoScrollPausedByWheel.value = false;
  shouldAutoScrollDraft.value = true;
  nextTick(scrollToBottom);
});

watch(
  () => latestDraftMessage.value?.id || "",
  () => {
    shouldAutoScrollDraft.value = true;
  }
);

watch(
  () => store.currentEditorDraft?.id,
  () => {
    editorContent.value = store.currentEditorDraft?.content || "";
    editorDraftName.value = store.currentEditorDraft?.draftName || "";
    humanizedContent.value = "";
    humanizeReport.value = "";
    showHumanizeReport.value = false;
    proofreadSuggestions.value = [];
    ignoredProofreadIndexes.value = [];
    isEditingDraftName.value = false;
    editorScrollTop.value = 0;
    store.editorSaveStatus = store.currentEditorDraft ? "saved" : "idle";
    nextTick(() => {
      if (editorTextarea.value) editorTextarea.value.scrollTop = 0;
    });
  },
  { immediate: true }
);

watch(isNovelSetupDirty, (value) => {
  store.novelSetupDirty = value;
});

watch(hasEditorContent, (value) => {
  if (value) return;
  showFindPanel.value = false;
  showReplacePanel.value = false;
  showWordStats.value = false;
  showAITools.value = false;
});

watch(
  isSetupAssistLoading,
  (value) => {
    store.novelSetupGenerating = value;
  },
  { immediate: true }
);

async function handleSaveAndLeave() {
  await saveNovelSetupDraft();
  store.executeStashedLeaveAction();
}

function openDraftPreview(message: Message) {
  if (!message.chapterDraft || !isDraftReady(message)) return;
  previewMessageId.value = message.id;
  previewDraftCache.value = message.chapterDraft;
}

function closeDraftPreview() {
  previewMessageId.value = null;
  previewDraftCache.value = null;
}

function openAdjacentDraft(message: Message | null) {
  if (!message) return;
  openDraftPreview(message);
}

async function joinPreviewDraft() {
  if (!previewMessageId.value) return;
  const id = previewMessageId.value;
  await store.joinChapterDraft(id);
  previewMessageId.value = null;
  previewDraftCache.value = null;
}

function isDraftReady(message: Message) {
  return Boolean(message.chapterDraft?.draftId) && !message.temporary;
}

function shouldShowThinkingStatus(message: Message) {
  const pending =
    store.activeStream?.id === message.id || Boolean(message.temporary);
  const hasContent = Boolean(message.content.trim());
  return (
    message.role === "assistant" &&
    pending &&
    (!hasContent || isWaitingForNextToken(message)) &&
    !message.chapterGeneration &&
    !message.chapterDraft &&
    !message.planOptions
  );
}

function isWaitingForNextToken(message: Message) {
  if (!message.content.trim()) return true;
  const lastTextAt =
    message.lastTextAt?.getTime() || message.timestamp.getTime();
  return thinkingNow.value - lastTextAt >= 900;
}

function shouldShowAssistantBubble(message: Message) {
  return (
    shouldShowAssistantContent(message) ||
    hasVisibleChapterDraft(message) ||
    Boolean(message.planOptions)
  );
}

function shouldShowAssistantContent(message: Message) {
  return Boolean(message.content.trim()) && !message.chapterGeneration;
}

function hasVisibleChapterDraft(message: Message) {
  return Boolean(message.chapterDraft?.content.trim());
}

function shouldShowPlanOptionsPlaceholder(message: Message) {
  return Boolean(message.temporary && message.planOptionsPlaceholder);
}

function planApplyType(message: Message): "volume" | "chapter" | null {
  const options = message.planOptions?.filter((option) => !option.custom) || [];
  if (options.length === 0) return null;
  const type = options[0].optionType;
  if (type !== "volume" && type !== "chapter") return null;
  return options.every((option) => option.optionType === type) ? type : null;
}

function planApplyOptions(message: Message): PlanOption[] {
  const type = planApplyType(message);
  if (!type) return [];
  return (message.planOptions || []).filter(
    (option) => !option.custom && option.optionType === type
  );
}

function shouldShowApplyPlan(message: Message) {
  return (
    Boolean(planApplyType(message)) &&
    planApplyOptions(message).length > 0 &&
    !message.temporary
  );
}

function shouldShowPlanApplyHeader(message: Message) {
  return (
    Boolean(planApplyType(message)) || shouldShowPlanOptionsPlaceholder(message)
  );
}

function planApplyHeaderTitle(message: Message) {
  const type = planApplyType(message);
  if (type === "volume") return "卷规划";
  if (type === "chapter") return "章节规划";
  return "规划生成";
}

function isPlanApplyButtonDisabled(message: Message) {
  return (
    Boolean(message.temporary) ||
    store.applyingPlanMessageId === message.id ||
    !shouldShowApplyPlan(message)
  );
}

function planApplyButtonText(message: Message) {
  if (store.applyingPlanMessageId === message.id) return "应用中...";
  return "应用规划";
}

function planApplyConfirmMessage(type: "volume" | "chapter") {
  if (type === "volume") {
    return "应用新的卷规划会让旧卷、旧章节、正文草稿和相关对话不可见。确认覆盖吗？";
  }
  return "应用新的章节规划会让旧章节、正文草稿和相关对话不可见。确认覆盖吗？";
}

function openPlanApplyConfirm(state: PlanApplyConfirmState) {
  planApplyConfirm.value = state;
  planApplyConfirmRemaining.value = 3;
  if (planApplyConfirmTimer) clearInterval(planApplyConfirmTimer);
  planApplyConfirmTimer = setInterval(() => {
    planApplyConfirmRemaining.value = Math.max(
      0,
      planApplyConfirmRemaining.value - 1
    );
    if (planApplyConfirmRemaining.value === 0 && planApplyConfirmTimer) {
      clearInterval(planApplyConfirmTimer);
      planApplyConfirmTimer = null;
    }
  }, 1000);
}

function clearPlanApplyConfirmTimer() {
  if (planApplyConfirmTimer) {
    clearInterval(planApplyConfirmTimer);
    planApplyConfirmTimer = null;
  }
  planApplyConfirmRemaining.value = 0;
}

async function handleApplyPlan(message: Message) {
  const type = planApplyType(message);
  if (!type) return;
  const options = planApplyOptions(message);
  await runApplyPlan(message.id, type, options, false);
}

async function runApplyPlan(
  messageId: string,
  type: "volume" | "chapter",
  options: PlanOption[],
  force: boolean
) {
  const result = await store.applyGeneratedPlan(
    messageId,
    type,
    options,
    force
  );
  if (result === "overwrite_required") {
    openPlanApplyConfirm({
      messageId,
      optionType: type,
      options,
      message: planApplyConfirmMessage(type),
    });
  }
}

function syncSetupAssistPayload(
  payload: { text: string; files: File[]; modelId: number },
  fallbackText = ""
) {
  const text = payload.text.trim() || fallbackText.trim();
  setupOriginalText.value = text;
  setupAssistText.value = text;
  setupAssistFiles.value = [...payload.files];
  setupAssistLastPayload.value = {
    text,
    files: [...payload.files],
    modelId: payload.modelId || store.generalSettings.modelId,
  };
}

function closePlanApplyConfirm() {
  clearPlanApplyConfirmTimer();
  planApplyConfirm.value = null;
}

async function confirmApplyPlan() {
  if (planApplyConfirmRemaining.value > 0) return;
  const state = planApplyConfirm.value;
  if (!state) return;
  clearPlanApplyConfirmTimer();
  planApplyConfirm.value = null;
  await runApplyPlan(state.messageId, state.optionType, state.options, true);
}

function chapterGenerationSteps(message: Message) {
  const cached = chapterProgressSteps.value[message.id];
  if (cached?.length) return visibleChapterGenerationSteps(cached);
  const progress = message.chapterGeneration;
  if (!progress) return [];
  if (progress.steps.length > 0)
    return visibleChapterGenerationSteps(progress.steps);
  return visibleChapterGenerationSteps([progress.text || "正在处理"]);
}

function visibleChapterGenerationSteps(steps: string[]) {
  return steps.filter(
    (step) => step !== "梳理上下文" && step !== "说明写作切入"
  );
}

function shouldShowChapterGeneration(message: Message) {
  const progress = message.chapterGeneration;
  if (!progress || progress.collapsed) return false;
  if (progress.failed) return true;
  if (!progress.complete) return true;
  const completedAt = chapterProgressCompletedAt.value[message.id];
  return !completedAt || thinkingNow.value - completedAt < 1000;
}

function isActiveChapterGenerationStep(message: Message, index: number) {
  if (!message.chapterGeneration || message.chapterGeneration.failed)
    return false;
  return (
    index === activeChapterGenerationStepIndex(message) &&
    !message.chapterGeneration.complete
  );
}

function chapterGenerationStageStep(stage: string) {
  if (stage === "thinking") return "梳理上下文";
  if (stage === "note") return "说明写作切入";
  if (stage === "validating") return "校验一致性";
  if (stage === "failed_once" || stage === "failed") return "校验不通过";
  if (stage === "passed" || stage === "collapsed") return "校验通过";
  return "";
}

function activeChapterGenerationStepIndex(message: Message) {
  const steps = chapterGenerationSteps(message);
  const current = chapterProgressStepLabels.value[message.id];
  if (!current) return steps.length - 1;
  const index = steps.lastIndexOf(current);
  return index >= 0 ? index : steps.length - 1;
}

function chapterGenerationElapsed(message: Message) {
  const serverStartedAt = message.chapterGeneration?.currentStepStartedAt;
  if (serverStartedAt) return formatElapsedSeconds(serverStartedAt.getTime());
  return formatElapsedSeconds(
    chapterProgressStarts.value[message.id] || message.timestamp.getTime()
  );
}

function chapterGenerationStepElapsed(
  message: Message,
  step: string,
  stepIndex: number
) {
  const timing = visibleChapterGenerationTimings(message)[stepIndex];
  if (!timing || timing.label !== step) return "";
  return formatDurationMilliseconds(stepTimingDurationMs(timing));
}

function visibleChapterGenerationTimings(message: Message) {
  return (message.chapterGeneration?.stepTimings || []).filter(
    (item) =>
      item.label !== "梳理上下文" &&
      item.label !== "说明写作切入" &&
      stepTimingDurationMs(item) > 0
  );
}

function chapterGenerationFailedHint(message: Message) {
  return message.chapterGeneration?.failed
    ? "校验仍未通过，本次生成已结束。请重试，或先调整章节规划后再生成。"
    : "";
}

function chapterGenerationStepOutput(
  message: Message,
  step: string,
  stepIndex: number
) {
  if (
    isActiveChapterGenerationStep(message, stepIndex) &&
    (step === "生成正文" || step === "按校验意见重写") &&
    message.content.trim()
  ) {
    return {
      step,
      attempt: message.chapterGeneration?.attempt || 0,
      type: "text",
      text: message.content.trim(),
      items: [],
    };
  }
  const outputs = message.chapterGeneration?.stepOutputs || [];
  const steps = chapterGenerationSteps(message);
  let outputIndex = 0;
  for (let i = 0; i <= stepIndex; i++) {
    const output = outputs[outputIndex];
    if (output?.step === steps[i]) {
      if (i === stepIndex) return output;
      outputIndex++;
      continue;
    }
    if (i === stepIndex) return undefined;
  }
  return undefined;
}

function chapterGenerationStepOutputKey(
  message: Message,
  step: string,
  stepIndex: number
) {
  return `${message.id}:${stepIndex}:${step}`;
}

function isChapterGenerationStepOutputExpanded(
  message: Message,
  step: string,
  stepIndex: number
) {
  return (
    expandedChapterStepOutputKeys.value[
      chapterGenerationStepOutputKey(message, step, stepIndex)
    ] === true
  );
}

function isChapterGenerationStepOutputCollapsed(
  message: Message,
  step: string,
  stepIndex: number
) {
  if (isActiveChapterGenerationStep(message, stepIndex)) return false;
  return !isChapterGenerationStepOutputExpanded(message, step, stepIndex);
}

function toggleChapterGenerationStepOutput(
  message: Message,
  step: string,
  stepIndex: number
) {
  const key = chapterGenerationStepOutputKey(message, step, stepIndex);
  expandedChapterStepOutputKeys.value = {
    ...expandedChapterStepOutputKeys.value,
    [key]: !expandedChapterStepOutputKeys.value[key],
  };
}

function shouldShowMessageTime(message: Message) {
  return message.role !== "assistant" || !message.temporary;
}

function thinkingElapsed(message: Message) {
  return formatElapsedSeconds(thinkingStartTime(message));
}

function thinkingStartTime(message: Message) {
  const cached = messageThinkingStarts.value[message.id];
  if (cached) return cached;
  const startedAt = message.timestamp.getTime();
  messageThinkingStarts.value = {
    ...messageThinkingStarts.value,
    [message.id]: startedAt,
  };
  return startedAt;
}

function setupAssistElapsed() {
  return setupAssistStartedAt.value
    ? formatElapsedSeconds(setupAssistStartedAt.value)
    : "0s";
}

function formatElapsedSeconds(startedAt: number) {
  const elapsedSeconds = Math.max(
    0,
    Math.floor((thinkingNow.value - startedAt) / 1000)
  );
  const minutes = Math.floor(elapsedSeconds / 60);
  const seconds = elapsedSeconds % 60;
  return minutes > 0 ? `${minutes}m ${seconds}s` : `${seconds}s`;
}

function formatDurationMilliseconds(durationMs: number) {
  const elapsedSeconds = Math.max(0, Math.floor(durationMs / 1000));
  const minutes = Math.floor(elapsedSeconds / 60);
  const seconds = elapsedSeconds % 60;
  return minutes > 0 ? `${minutes}m ${seconds}s` : `${seconds}s`;
}

function stepTimingDurationMs(timing: { startedAt?: Date; endedAt?: Date }) {
  if (!timing.startedAt || !timing.endedAt) return 0;
  return Math.max(0, timing.endedAt.getTime() - timing.startedAt.getTime());
}

function scheduleEditorSave() {
  store.editorSaveStatus = "idle";
  if (editorSaveTimer) clearTimeout(editorSaveTimer);
  editorSaveTimer = setTimeout(() => {
    void store.saveEditorDraft(editorContent.value);
  }, 800);
}

function saveEditorNow() {
  if (editorSaveTimer) clearTimeout(editorSaveTimer);
  void store.saveEditorDraft(editorContent.value);
}

function startEditingDraftName() {
  if (!store.currentEditorDraft) return;
  isEditingDraftName.value = true;
  nextTick(() => {
    draftTitleInput.value?.focus();
    draftTitleInput.value?.select();
  });
}

function saveDraftName() {
  if (!store.currentEditorDraft) return;
  const name = editorDraftName.value.trim();
  if (!name) {
    editorDraftName.value = store.currentEditorDraft.draftName || "未命名草稿";
    isEditingDraftName.value = false;
    return;
  }
  isEditingDraftName.value = false;
  if (name !== store.currentEditorDraft.draftName) {
    void store.saveEditorDraftName(name, editorContent.value);
  }
}

function openDraftPicker() {
  if (draftPickerCloseTimer) clearTimeout(draftPickerCloseTimer);
  showDraftPicker.value = true;
}

function closeDraftPickerSoon() {
  if (draftPickerCloseTimer) clearTimeout(draftPickerCloseTimer);
  draftPickerCloseTimer = setTimeout(() => {
    showDraftPicker.value = false;
  }, 180);
}

function openAITools() {
  if (aiToolsCloseTimer) clearTimeout(aiToolsCloseTimer);
  showAITools.value = true;
}

function closeAIToolsSoon() {
  if (aiToolsCloseTimer) clearTimeout(aiToolsCloseTimer);
  aiToolsCloseTimer = setTimeout(() => {
    showAITools.value = false;
  }, 180);
}

function closeAITools() {
  if (aiToolsCloseTimer) clearTimeout(aiToolsCloseTimer);
  showAITools.value = false;
}

function toggleFind() {
  if (!hasEditorContent.value) return;
  fillFindTextFromSelection();
  showFindPanel.value = !showFindPanel.value;
  if (!showFindPanel.value) showReplacePanel.value = false;
  if (showFindPanel.value) focusFindInput();
}

function toggleReplace() {
  if (!hasEditorContent.value) return;
  fillFindTextFromSelection();
  showReplacePanel.value = !showReplacePanel.value;
  if (showReplacePanel.value) showFindPanel.value = true;
  if (showFindPanel.value) focusFindInput();
}

function focusFindInput() {
  nextTick(() => {
    findInput.value?.focus();
    findInput.value?.select();
  });
}

function findNext() {
  if (!findText.value) return;
  const textarea = editorTextarea.value;
  if (!textarea) return;
  const start = textarea.selectionEnd;
  const next = editorContent.value.indexOf(findText.value, start);
  const index = next >= 0 ? next : editorContent.value.indexOf(findText.value);
  if (index >= 0) {
    selectEditorRange(index, index + findText.value.length);
  }
}

function findPrevious() {
  if (!findText.value) return;
  const textarea = editorTextarea.value;
  if (!textarea) return;
  const before = editorContent.value.slice(0, textarea.selectionStart);
  const prev = before.lastIndexOf(findText.value);
  const index =
    prev >= 0 ? prev : editorContent.value.lastIndexOf(findText.value);
  if (index >= 0) {
    selectEditorRange(index, index + findText.value.length);
  }
}

function selectEditorRange(start: number, end: number) {
  const textarea = editorTextarea.value;
  if (!textarea) return;
  textarea.focus();
  textarea.setSelectionRange(start, end);
  activeFindIndex.value = findText.value
    ? editorContent.value.slice(0, start).split(findText.value).length
    : 0;
  textarea.scrollTop = caretScrollTop(textarea, start);
  handleEditorScroll();
}

function caretScrollTop(textarea: HTMLTextAreaElement, index: number): number {
  const style = window.getComputedStyle(textarea);
  const mirror = document.createElement("div");
  const marker = document.createElement("span");
  mirror.style.position = "absolute";
  mirror.style.visibility = "hidden";
  mirror.style.pointerEvents = "none";
  mirror.style.whiteSpace = "pre-wrap";
  mirror.style.overflowWrap = "break-word";
  mirror.style.boxSizing = "border-box";
  mirror.style.width = `${textarea.clientWidth}px`;
  mirror.style.font = style.font;
  mirror.style.lineHeight = style.lineHeight;
  mirror.style.letterSpacing = style.letterSpacing;
  mirror.style.padding = `${style.paddingTop} ${style.paddingRight} ${style.paddingBottom} ${style.paddingLeft}`;
  mirror.style.border = style.border;
  mirror.textContent = editorContent.value.slice(0, index);
  marker.textContent = "\u200b";
  mirror.appendChild(marker);
  document.body.appendChild(mirror);
  const markerTop = marker.offsetTop;
  document.body.removeChild(mirror);
  return Math.max(0, markerTop - textarea.clientHeight * 0.35);
}

function commitTextareaReplacement(start: number, end: number, value: string) {
  const textarea = editorTextarea.value;
  if (!textarea) return;
  textarea.focus();
  textarea.setSelectionRange(start, end);
  const inserted = document.execCommand("insertText", false, value);
  if (!inserted) {
    textarea.setRangeText(value, start, end, "select");
    textarea.dispatchEvent(
      new InputEvent("input", {
        bubbles: true,
        inputType: "insertText",
        data: value,
      })
    );
  }
  editorContent.value = textarea.value;
}

function replaceCurrent() {
  const textarea = editorTextarea.value;
  if (!textarea || !findText.value) return;
  const selected = editorContent.value.slice(
    textarea.selectionStart,
    textarea.selectionEnd
  );
  if (selected !== findText.value) {
    findNext();
    return;
  }
  const start = textarea.selectionStart;
  commitTextareaReplacement(start, textarea.selectionEnd, replaceText.value);
  nextTick(() => selectEditorRange(start, start + replaceText.value.length));
}

function replaceAll() {
  const textarea = editorTextarea.value;
  if (!textarea || !findText.value) return;
  const nextContent = editorContent.value
    .split(findText.value)
    .join(replaceText.value);
  commitTextareaReplacement(0, editorContent.value.length, nextContent);
}

function formatEditorContent() {
  if (!hasEditorContent.value) return;
  const textarea = editorTextarea.value;
  if (!textarea) return;
  const textareaScrollTop = textarea.scrollTop;
  const textareaScrollLeft = textarea.scrollLeft;
  const containerScrollTop = editorContainer.value?.scrollTop ?? 0;
  const selectionStart = textarea.selectionStart;
  const selectionEnd = textarea.selectionEnd;
  const formatted = editorContent.value
    .replace(/\r\n/g, "\n")
    .split("\n")
    .map((line) => line.trimEnd())
    .filter((line) => line.trim())
    .map((line) => `　　${line.trimStart()}`)
    .join("\n");
  if (formatted === editorContent.value) return;
  commitTextareaReplacement(0, editorContent.value.length, formatted);
  nextTick(() => {
    textarea.scrollTop = textareaScrollTop;
    textarea.scrollLeft = textareaScrollLeft;
    if (editorContainer.value)
      editorContainer.value.scrollTop = containerScrollTop;
    selectEditorRange(
      Math.min(selectionStart, formatted.length),
      Math.min(selectionEnd, formatted.length)
    );
    textarea.scrollTop = textareaScrollTop;
    textarea.scrollLeft = textareaScrollLeft;
  });
}

async function runHumanize() {
  if (!store.editorChapterId || !hasEditorContent.value) {
    store.notifyError("正文内容不能为空");
    return;
  }
  closeAITools();
  if (humanizeAbortController) humanizeAbortController.abort();
  humanizeAbortController = new AbortController();
  isHumanizing.value = true;
  humanizedContent.value = "";
  humanizeReport.value = "";
  showHumanizeReport.value = false;
  try {
    const result = await humanizeChapterApi(
      store.editorChapterId,
      editorContent.value,
      store.editorDraftId || undefined,
      humanizeAbortController.signal
    );
    humanizedContent.value = result.content || "";
    humanizeReport.value =
      result.report || "## AI 消痕报告\n\n本次未返回报告。";
    store.notifyInfo("AI 消痕完成");
  } catch (err) {
    if (
      (err instanceof DOMException && err.name === "AbortError") ||
      (err instanceof Error && err.name === "AbortError")
    )
      return;
    store.notifyError(err instanceof Error ? err.message : "AI 消痕失败");
    humanizedContent.value = "";
  } finally {
    isHumanizing.value = false;
    humanizeAbortController = null;
  }
}

function cancelHumanize() {
  humanizeAbortController?.abort();
  humanizeAbortController = null;
  isHumanizing.value = false;
  humanizedContent.value = "";
}

function applyHumanizedContent() {
  if (!humanizedContent.value) return;
  commitTextareaReplacement(
    0,
    editorContent.value.length,
    humanizedContent.value
  );
  humanizedContent.value = "";
  scheduleEditorSave();
}

async function joinHumanizedAsDraft() {
  if (!humanizedContent.value || !store.editorChapterId) return;
  const content = humanizedContent.value;
  try {
    const draft = await createChapterDraftFromContentApi(
      store.editorChapterId,
      content
    );
    const nextDraft = toChapterContentDraft(draft);
    store.upsertEditorDraft(nextDraft);
    store.editorDraftId = nextDraft.id;
    showHumanizeMenu.value = false;
    store.notifyInfo("已新加草稿");
  } catch (err) {
    store.notifyError(err instanceof Error ? err.message : "新加草稿失败");
  }
}

function downloadHumanizeReport() {
  const blob = new Blob([humanizeReport.value || ""], {
    type: "text/markdown;charset=utf-8",
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${
    editorDraftName.value || editorTitle.value || "ai-humanize-report"
  }-消痕报告.md`;
  link.click();
  URL.revokeObjectURL(url);
}

async function runProofread() {
  if (!store.editorChapterId || !hasEditorContent.value) {
    store.notifyError("正文内容不能为空");
    return;
  }
  closeAITools();
  if (proofreadAbortController) proofreadAbortController.abort();
  proofreadAbortController = new AbortController();
  isProofreading.value = true;
  proofreadSuggestions.value = [];
  ignoredProofreadIndexes.value = [];
  try {
    const suggestions = await proofreadChapterApi(
      store.editorChapterId,
      editorContent.value,
      store.editorDraftId || undefined,
      proofreadAbortController.signal
    );
    proofreadSuggestions.value = suggestions.filter((item) =>
      editorContent.value.includes(item.originalText)
    );
    store.notifyInfo(
      proofreadSuggestions.value.length > 0
        ? "AI 校审完成"
        : "AI 校审完成，暂未发现明显问题"
    );
  } catch (err) {
    if (
      (err instanceof DOMException && err.name === "AbortError") ||
      (err instanceof Error && err.name === "AbortError")
    )
      return;
    store.notifyError(err instanceof Error ? err.message : "AI 校审失败");
  } finally {
    isProofreading.value = false;
    proofreadAbortController = null;
  }
}

function cancelProofread() {
  proofreadAbortController?.abort();
  proofreadAbortController = null;
  isProofreading.value = false;
}

function ignoreProofreadSuggestion(index: number) {
  ignoredProofreadIndexes.value = Array.from(
    new Set([...ignoredProofreadIndexes.value, index])
  );
}

function applyProofreadSuggestion(index: number) {
  const suggestion = proofreadSuggestions.value[index];
  if (!suggestion) return;
  const start = editorContent.value.indexOf(suggestion.originalText);
  if (start < 0) {
    ignoreProofreadSuggestion(index);
    return;
  }
  commitTextareaReplacement(
    start,
    start + suggestion.originalText.length,
    suggestion.suggestedText
  );
  scheduleEditorSave();
  ignoreProofreadSuggestion(index);
}

watch(findText, () => {
  activeFindIndex.value = 0;
});

function handleEditorKeydown(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "f") {
    event.preventDefault();
    fillFindTextFromSelection();
    showFindPanel.value = true;
    focusFindInput();
  } else if (
    (event.ctrlKey || event.metaKey) &&
    event.key.toLowerCase() === "h"
  ) {
    event.preventDefault();
    fillFindTextFromSelection();
    showFindPanel.value = true;
    showReplacePanel.value = true;
    focusFindInput();
  } else if (
    (event.ctrlKey || event.metaKey) &&
    event.key.toLowerCase() === "s"
  ) {
    event.preventDefault();
    saveEditorNow();
  } else if (event.key === "Escape") {
    showFindPanel.value = false;
    showReplacePanel.value = false;
    showWordStats.value = false;
    showDraftPicker.value = false;
    showAITools.value = false;
  }
}

function handleEditorScroll() {
  if (!editorTextarea.value) return;
  editorScrollTop.value = editorTextarea.value.scrollTop;
}

function focusWord(word: string) {
  highlightedWord.value = word;
  findText.value = word;
  showFindPanel.value = false;
  showReplacePanel.value = false;
  nextTick(findNext);
}

function selectInputModel(modelId: number) {
  const model = store.models.find((item) => item.id === modelId);
  if (!model) return;
  store.generalSettings.modelId = model.id;
  store.generalSettings.model = model.name;
  modelOpen.value = false;
  void store.saveSettings();
}

function openMoreModels() {
  modelOpen.value = false;
  store.openCustomModelSettings();
}

function closeModelMenu() {
  modelOpen.value = false;
}

function onDocumentClick(e: MouseEvent) {
  if (!modelOpen.value && !setupAssistModelOpen.value) return;
  const target = e.target as HTMLElement;
  if (!target.closest(".model-selector")) {
    closeModelMenu();
    setupAssistModelOpen.value = false;
  }
}

const PLAN_DETAIL_LABELS: Record<string, string> = {
  world: "世界观",
  core_conflict: "核心冲突",
  character_plan: "人物安排",
  character_settings: "重点人物",
  forces: "势力设定",
  other_settings: "其他设定",
  key_selling_points: "主要看点",
  risk_control: "雷点处理",
  core_focus: "核心重点",
  suggested_tone: "推荐风格",
  timeline: "所处时间",
  locations: "主要地点",
  current_state: "当前状态",
  end_state: "结束状态",
  character_development: "角色成长",
  setting_development: "设定发展",
  setting_boundaries: "设定边界",
  characters: "出场人物",
  key_events: "关键事件",
  intertextual_links: "跨章关联",
  foreshadowing: "伏笔",
  writing_focus: "写作重点",
  other_highlights: "其他亮点",
  temporary_settings: "临时设定",
  chapter_count: "章节数量",
};

function planDetailKeys(optionType?: string): string[] {
  if (optionType === "volume") {
    return [
      "timeline",
      "locations",
      "characters",
      "current_state",
      "end_state",
      "character_development",
      "setting_development",
      "temporary_settings",
      "chapter_count",
      "key_events",
      "foreshadowing",
      "other_highlights",
    ];
  }
  if (optionType === "chapter") {
    return ["intertextual_links", "writing_focus"];
  }
  return [
    "world",
    "core_conflict",
    "character_plan",
    "character_settings",
    "forces",
    "other_settings",
    "key_selling_points",
    "risk_control",
  ];
}

function planDetails(
  option: PlanOption
): { key: string; label: string; value: string | string[] }[] {
  if (option.custom) return [];

  const details = option.details || {};
  return planDetailKeys(option.optionType)
    .map((key) => {
      const value = details[key];
      return {
        key,
        label: PLAN_DETAIL_LABELS[key] || key,
        value: formatPlanDetailValue(key, value),
      };
    })
    .filter((item) =>
      Array.isArray(item.value) ? item.value.length > 0 : item.value.trim()
    );
}

function formatPlanDetailValue(key: string, value: unknown): string | string[] {
  if (Array.isArray(value)) {
    return value.map((item) => formatPlanDetailArrayItem(item)).filter(Boolean);
  }
  if (key === "chapter_count" && value != null) {
    return `${value}章左右`;
  }
  if (value == null) {
    return "";
  }
  if (typeof value === "object") {
    return formatPlanDetailObject(value as Record<string, unknown>);
  }
  return String(value);
}

function formatPlanDetailArrayItem(item: unknown): string {
  if (item == null) return "";
  if (typeof item !== "object" || item === null) return String(item).trim();
  const raw = item as Record<string, unknown>;
  const name = String(raw.name || "").trim();
  const stateBefore = String(raw.state_before || "").trim();
  const stateAfter = String(raw.state_after || "").trim();
  const allowedProgress = String(raw.allowed_progress || "").trim();
  const forbiddenProgress = String(raw.forbidden_progress || "").trim();
  const parts = [
    name,
    stateBefore || stateAfter
      ? `${stateBefore || "未写"} -> ${stateAfter || "未写"}`
      : "",
    allowedProgress ? `允许：${allowedProgress}` : "",
    forbiddenProgress ? `禁止：${forbiddenProgress}` : "",
  ].filter(Boolean);
  return parts.join("；");
}

function formatPlanDetailObject(value: Record<string, unknown>): string {
  const parts = Object.entries(value)
    .map(([key, item]) => formatPlanDetailObjectEntry(key, item))
    .filter(Boolean);
  return parts.join("；");
}

function formatPlanDetailObjectEntry(key: string, value: unknown): string {
  const label = PLAN_DETAIL_LABELS[key] || key;
  const text = formatNestedPlanDetailValue(value);
  return text ? `${label}：${text}` : "";
}

function formatNestedPlanDetailValue(value: unknown): string {
  if (Array.isArray(value)) {
    return value.map(formatPlanDetailArrayItem).filter(Boolean).join("；");
  }
  if (value && typeof value === "object") {
    return formatPlanDetailObject(value as Record<string, unknown>);
  }
  return value == null ? "" : String(value).trim();
}

function planCharacterSettings(option: PlanOption): string[] {
  const value = option.details?.character_settings;
  if (!Array.isArray(value)) return [];
  return value
    .filter((item): item is string => typeof item === "string" && !!item.trim())
    .map((item) => item.trim());
}

function normalPlanDetails(option: PlanOption) {
  return planDetails(option).filter(
    (detail) =>
      detail.key !== "character_settings" && detail.key !== "temporary_settings"
  );
}

function planDetailItems(value: string | string[]): string[] {
  return Array.isArray(value) ? value : [];
}

function isCompactPlanDetail(detail: {
  key: string;
  value: string | string[];
}): boolean {
  return (
    Array.isArray(detail.value) &&
    (detail.key === "locations" || detail.key === "characters")
  );
}

function isMarkdownPlanDetail(detail: { value: string | string[] }): boolean {
  return !Array.isArray(detail.value);
}

type PlanTemporarySetupSection = {
  key: string;
  label: string;
  items: string[];
};

function planTemporarySetupSections(
  option: PlanOption
): PlanTemporarySetupSection[] {
  const value = option.details?.temporary_settings;
  if (!value || typeof value !== "object") return [];
  const raw = value as Record<string, unknown>;
  return [
    planTemporaryNamedSection("characters", "人物设定", raw.characters),
    planTemporaryNamedSection("maps", "地点设定", raw.maps),
    planTemporaryNamedSection("forces", "势力设定", raw.forces),
    planTemporaryOtherSettingsSection(raw.other_settings),
  ].filter((section): section is PlanTemporarySetupSection => !!section);
}

function planTemporaryNamedSection(
  key: string,
  label: string,
  value: unknown
): PlanTemporarySetupSection | null {
  if (!Array.isArray(value)) return null;
  const items = value
    .map((item) => {
      if (!item || typeof item !== "object") return "";
      const raw = item as Record<string, unknown>;
      return planTemporaryItemText(
        setupString(raw.name),
        setupString(raw.appearanceTime || raw.appearance_time),
        setupString(raw.notes)
      );
    })
    .filter(Boolean);
  return items.length > 0 ? { key, label, items } : null;
}

function planTemporaryOtherSettingsSection(
  value: unknown
): PlanTemporarySetupSection | null {
  if (!Array.isArray(value)) return null;
  const items = value
    .map((setting) => {
      if (!setting || typeof setting !== "object") return "";
      const raw = setting as Record<string, unknown>;
      const title = setupString(raw.title);
      const children = Array.isArray(raw.items)
        ? raw.items
            .map((item) => {
              if (!item || typeof item !== "object") return "";
              const itemRaw = item as Record<string, unknown>;
              return planTemporaryItemText(
                setupString(itemRaw.name),
                setupString(itemRaw.appearanceTime || itemRaw.appearance_time),
                setupString(itemRaw.notes)
              );
            })
            .filter(Boolean)
        : [];
      const description = setupString(raw.description);
      return [title, description, children.join("；")]
        .filter(Boolean)
        .join("：");
    })
    .filter(Boolean);
  return items.length > 0
    ? { key: "other_settings", label: "其他设定", items }
    : null;
}

function planTemporaryItemText(
  name: string,
  appearanceTime: string,
  notes: string
): string {
  return [name, appearanceTime ? `出场时间：${appearanceTime}` : "", notes]
    .filter(Boolean)
    .join("；");
}

function planDetailKey(messageId: string, optionId: string): string {
  return `${messageId}:${optionId}`;
}

function planDisplayTitle(option: PlanOption, index: number): string {
  if (option.optionType === "volume")
    return `第${index + 1}卷：${option.title}`;
  if (option.optionType === "chapter")
    return `第${index + 1}章：${option.title}`;
  return option.title;
}

function isSelectablePlan(option: PlanOption): boolean {
  return (
    !option.custom &&
    option.optionType !== "volume" &&
    option.optionType !== "chapter"
  );
}

function togglePlanDetails(messageId: string, option: PlanOption) {
  const key = planDetailKey(messageId, option.id);
  expandedPlanDetailKey.value =
    expandedPlanDetailKey.value === key ? null : key;
}

function handleUsePlan(option: PlanOption) {
  if (option.custom) {
    activeCustomOption.value = null;
    inputText.value = "自定义输入：";
    nextTick(() => {
      inputRef.value?.focus();
      resizeInput();
    });
    return;
  }
  if (!isSelectablePlan(option)) return;
  prepareMessageAutoScroll();
  store.selectPlanOption(option.id);
  activeCustomOption.value = null;
}

function handleCustomSend() {
  const text = customInputText.value.trim();
  if (text) {
    activeCustomOption.value = null;
    prepareMessageAutoScroll();
    store.sendMessage(text);
    customInputText.value = "";
    nextTick(() => scrollToBottom());
  }
}

function handleSend() {
  if (isVoiceRecognizing.value) {
    confirmVoiceRecognition();
    return;
  }
  const content = inputText.value.trim();
  if (content) {
    const useGraphMode = Boolean(
      store.selectedChapterId && chapterGraphMode.value
    );
    prepareMessageAutoScroll();
    store.sendMessage(content, {
      graphMode: useGraphMode,
    });
    if (useGraphMode) chapterGraphMode.value = false;
    inputText.value = "";
    resizeInput();
    nextTick(() => scrollToBottom());
  }
}

function toggleChapterGraphMode() {
  chapterGraphMode.value = !chapterGraphMode.value;
}

function speechRecognitionConstructor() {
  if (typeof window === "undefined") return null;
  return (
    (window as any).SpeechRecognition ||
    (window as any).webkitSpeechRecognition ||
    null
  );
}

function startVoiceRecognition() {
  if (isVoiceRecognizing.value) {
    cancelVoiceRecognition();
    return;
  }
  const Recognition = speechRecognitionConstructor();
  if (!Recognition) {
    store.notifyInfo("当前浏览器不支持语音识别");
    return;
  }
  voiceOriginalInput = inputText.value;
  voiceInputBase = inputText.value;
  voiceTranscript.value = "";
  lastVoiceTranscript = "";
  const recognition = new Recognition();
  speechRecognition = recognition;
  recognition.lang = "zh-CN";
  recognition.continuous = true;
  recognition.interimResults = true;
  recognition.onresult = (event: any) => {
    let text = "";
    for (let i = 0; i < event.results.length; i++) {
      text += event.results[i][0]?.transcript || "";
    }
    const nextTranscript = text.trim();
    if (nextTranscript && nextTranscript !== lastVoiceTranscript) {
      window.navigator.vibrate?.(25);
      lastVoiceTranscript = nextTranscript;
    }
    voiceTranscript.value = nextTranscript;
    const spacer = voiceInputBase.trim() && nextTranscript ? " " : "";
    inputText.value = `${voiceInputBase}${spacer}${nextTranscript}`;
    nextTick(resizeInput);
  };
  recognition.onerror = (event: any) => {
    if (event?.error === "no-speech" && isVoiceRecognizing.value) return;
    cancelVoiceRecognition();
    store.notifyInfo("语音识别已结束");
  };
  recognition.onend = () => {
    if (!isVoiceRecognizing.value || speechRecognition !== recognition) return;
    if (voiceTranscript.value.trim()) {
      voiceInputBase = inputText.value;
      voiceTranscript.value = "";
      lastVoiceTranscript = "";
    }
    voiceRestartTimer = setTimeout(() => {
      if (!isVoiceRecognizing.value || speechRecognition !== recognition)
        return;
      try {
        recognition.start();
      } catch {
        cancelVoiceRecognition();
        store.notifyInfo("语音识别已结束");
      }
    }, 120);
  };
  isVoiceRecognizing.value = true;
  recognition.start();
}

function stopSpeechRecognition() {
  if (voiceRestartTimer) {
    clearTimeout(voiceRestartTimer);
    voiceRestartTimer = null;
  }
  if (!speechRecognition) return;
  const current = speechRecognition;
  speechRecognition = null;
  current.onend = null;
  current.onresult = null;
  current.onerror = null;
  try {
    current.stop();
  } catch {
    // 识别对象可能已经自行结束，停止失败时无需影响输入内容。
  }
}

function cancelVoiceRecognition() {
  stopSpeechRecognition();
  isVoiceRecognizing.value = false;
  inputText.value = voiceOriginalInput;
  voiceTranscript.value = "";
  lastVoiceTranscript = "";
  nextTick(resizeInput);
}

function confirmVoiceRecognition() {
  const text = voiceTranscript.value.trim();
  stopSpeechRecognition();
  isVoiceRecognizing.value = false;
  if (text) {
    const spacer = voiceInputBase.trim() ? " " : "";
    inputText.value = `${voiceInputBase}${spacer}${text}`;
    nextTick(() => {
      inputRef.value?.focus();
      resizeInput();
    });
  } else {
    inputText.value = voiceInputBase;
    nextTick(resizeInput);
  }
  voiceTranscript.value = "";
  lastVoiceTranscript = "";
}

function applyQuickPrompt(prompt: string) {
  inputText.value = prompt;
  nextTick(() => {
    inputRef.value?.focus();
    resizeInput();
  });
}

function resizeInput() {
  nextTick(() => {
    const input = inputRef.value;
    if (!input) return;
    input.style.height = "auto";
    input.style.height = `${Math.min(input.scrollHeight, 180)}px`;
  });
}

function switchToEditor() {
  if (store.selectedChapterId) {
    store.openEditorMode(store.selectedChapterId);
  } else {
    store.switchToEditorMode();
  }
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    handleSend();
  }
}

onMounted(() => {
  document.addEventListener("click", onDocumentClick);
  thinkingTimer = setInterval(() => {
    thinkingNow.value = Date.now();
  }, 1000);
});
onUnmounted(() => {
  if (editorSaveTimer) clearTimeout(editorSaveTimer);
  if (draftPickerCloseTimer) clearTimeout(draftPickerCloseTimer);
  if (aiToolsCloseTimer) clearTimeout(aiToolsCloseTimer);
  if (thinkingTimer) clearInterval(thinkingTimer);
  if (messageAutoScrollTimer) clearTimeout(messageAutoScrollTimer);
  clearPlanApplyConfirmTimer();
  humanizeAbortController?.abort();
  proofreadAbortController?.abort();
  setupAssistAbortController?.abort();
  document.removeEventListener("click", onDocumentClick);
});
</script>

<template>
  <div
    class="relative flex h-screen flex-1 flex-col bg-gray-50 dark:bg-gray-900"
  >
    <!-- Header -->
    <header
      v-if="
        !store.isNovelSetupChoiceOpen &&
        !store.isNovelSetupOpen &&
        store.selectedNovelId
      "
      class="flex h-16 items-center justify-between border-b border-gray-200 bg-white px-6 dark:border-gray-800 dark:bg-gray-950"
    >
      <div class="flex items-center gap-1.5 text-sm">
        <template v-if="viewBreadcrumb.length > 0">
          <template v-for="(part, index) in viewBreadcrumb" :key="index">
            <span
              class="text-gray-500 dark:text-gray-400"
              :class="
                index === viewBreadcrumb.length - 1
                  ? 'font-medium text-gray-900 dark:text-white'
                  : ''
              "
              >{{ part }}</span
            >
            <ChevronRight
              v-if="index < viewBreadcrumb.length - 1"
              class="size-4 text-gray-400"
            />
          </template>
        </template>
        <template v-else>
          <span
            v-if="store.viewMode === 'editor'"
            class="text-gray-500 dark:text-gray-400"
            >选择章节开始编辑</span
          >
          <span v-else class="text-gray-500 dark:text-gray-400"
            >选择一部小说开始创作</span
          >
        </template>
      </div>
      <div class="flex items-center gap-3">
        <div
          v-if="store.selectedModel"
          class="model-selector relative shrink-0"
        >
          <button
            class="flex max-w-36 items-center gap-1 rounded-lg border border-gray-200 px-2.5 py-2 text-xs text-gray-600 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
            @click.stop="modelOpen = !modelOpen"
          >
            <span class="truncate">{{ store.selectedModel.name }}</span>
            <ChevronDown class="size-3.5" />
          </button>
          <div
            v-if="modelOpen"
            class="absolute right-0 top-full z-30 mt-1 w-40 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
          >
            <button
              v-for="model in store.models"
              :key="model.id"
              class="w-full truncate px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
              :class="
                store.generalSettings.modelId === model.id
                  ? 'bg-gray-100 dark:bg-gray-700'
                  : ''
              "
              @click="selectInputModel(model.id)"
            >
              {{ model.name }}
            </button>
            <button
              class="w-full border-t border-gray-100 px-3 py-2 text-left text-sm text-blue-500 hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-700"
              @click="openMoreModels"
            >
              添加更多
            </button>
          </div>
        </div>
        <div
          class="flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-gray-700 dark:bg-gray-800"
        >
          <button
            class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
            :class="
              store.viewMode === 'chat'
                ? 'bg-white text-gray-900 shadow-sm dark:bg-gray-700 dark:text-white'
                : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
            "
            @click="store.switchToChatMode()"
          >
            对话
          </button>
          <button
            class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
            :class="
              store.viewMode === 'editor'
                ? 'bg-white text-gray-900 shadow-sm dark:bg-gray-700 dark:text-white'
                : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
            "
            @click="switchToEditor()"
          >
            编辑
          </button>
        </div>
      </div>
    </header>

    <!-- Novel Setup Mode -->
    <div
      v-if="
        store.viewMode === 'chat' &&
        (store.isNovelSetupChoiceOpen ||
          (!store.isNovelSetupOpen &&
            !store.selectedNovelId &&
            store.activeMessages.length === 0 &&
            !store.isMessagesLoading))
      "
      class="flex flex-1 overflow-hidden px-5 py-3"
    >
      <div
        class="mx-auto grid h-full w-full max-w-7xl grid-rows-[auto_auto_minmax(0,1fr)] gap-3"
      >
        <div class="grid gap-3 lg:grid-cols-[minmax(420px,1.35fr)_2.65fr]">
          <div class="flex min-h-28 flex-col justify-center">
            <img
              :src="welcomeImage"
              alt="欢迎回来，继续书写你的世界"
              class="h-32 w-full max-w-[560px] object-contain object-left dark:invert dark:brightness-110"
            />
          </div>

          <div class="grid gap-3 md:grid-cols-4">
            <div
              class="min-w-0 rounded-xl border border-gray-200 bg-white px-3 py-4 shadow-sm dark:border-gray-800 dark:bg-gray-900"
            >
              <div
                class="flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400"
              >
                <FileText class="size-5" />
                <span>字数统计</span>
              </div>
              <div
                class="mt-2 truncate text-2xl font-bold text-gray-900 dark:text-white"
              >
                {{ formatDashboardNumber(dashboardTotalWords) }}
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">字数</p>
            </div>
            <div
              class="min-w-0 rounded-xl border border-gray-200 bg-white px-3 py-4 shadow-sm dark:border-gray-800 dark:bg-gray-900"
            >
              <div
                class="flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400"
              >
                <BookOpen class="size-5" />
                <span>章节数量</span>
              </div>
              <div
                class="mt-2 truncate text-2xl font-bold text-gray-900 dark:text-white"
              >
                {{ formatDashboardNumber(dashboardTotalChapters) }}
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">章节</p>
            </div>
            <div
              class="min-w-0 rounded-xl border border-gray-200 bg-white px-3 py-4 shadow-sm dark:border-gray-800 dark:bg-gray-900"
            >
              <div
                class="flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400"
              >
                <Clock class="size-5" />
                <span>创作时长</span>
              </div>
              <div
                class="mt-2 truncate text-2xl font-bold text-gray-900 dark:text-white"
              >
                {{ dashboardWritingHours }}
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">小时</p>
            </div>
            <div
              class="min-w-0 rounded-xl border border-gray-200 bg-white px-3 py-4 shadow-sm dark:border-gray-800 dark:bg-gray-900"
            >
              <div
                class="flex items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400"
              >
                <Calendar class="size-5" />
                <span>最后编辑</span>
              </div>
              <div
                class="mt-2 truncate text-xl font-bold text-gray-900 dark:text-white"
              >
                {{ dashboardLastEdited }}
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                在创作
              </p>
            </div>
          </div>
        </div>

        <section
          class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-gray-800 dark:bg-gray-900"
        >
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            快速开始
          </h3>
          <div class="mt-3 grid gap-3 md:grid-cols-2">
            <button
              v-for="option in setupModeOptions"
              :key="option.label"
              class="group flex min-h-[82px] items-center gap-4 rounded-lg border px-5 py-3 text-left transition-colors"
              :class="
                option.enabled
                  ? 'border-gray-200 bg-gray-50 text-gray-900 hover:border-gray-300 hover:bg-white dark:border-gray-700 dark:bg-gray-800/60 dark:text-white dark:hover:bg-gray-800'
                  : 'cursor-not-allowed border-gray-200 bg-gray-50 text-gray-400 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-500'
              "
              @click="selectSetupMode(option)"
            >
              <component
                :is="option.icon"
                class="size-9 shrink-0 text-gray-500 dark:text-gray-400"
              />
              <div class="min-w-0 flex-1">
                <div class="flex items-baseline gap-2">
                  <span class="text-base font-bold">{{ option.label }}</span>
                  <span class="text-sm font-medium">{{ option.title }}</span>
                </div>
                <p
                  class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400"
                >
                  {{
                    option.enabled ? option.desc : `${option.desc}，敬请期待`
                  }}
                </p>
              </div>
              <ArrowRight
                class="size-5 shrink-0 text-gray-400 transition-transform group-hover:translate-x-0.5"
              />
            </button>
          </div>
        </section>

        <div
          class="grid min-h-0 items-stretch gap-3 lg:grid-cols-[1.28fr_0.95fr]"
        >
          <section
            class="flex min-h-0 flex-col rounded-xl border border-gray-200 bg-white p-3 shadow-sm dark:border-gray-800 dark:bg-gray-900"
          >
            <div>
              <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                创作趋势（近7天字数）
              </h3>
            </div>
            <div class="mt-3 flex min-h-0 flex-1 flex-col gap-3">
              <div
                class="min-h-[200px] flex-1 rounded-lg bg-gray-50 px-3 py-3 dark:bg-gray-800/50"
              >
                <svg
                  class="h-full w-full overflow-visible"
                  viewBox="0 0 620 176"
                  role="img"
                  aria-label="近7天创作字数折线图，鼠标悬浮数据点可查看完成字数"
                >
                  <g v-for="tick in dashboardTrendTicks" :key="tick.label">
                    <line
                      x1="54"
                      x2="596"
                      :y1="tick.y"
                      :y2="tick.y"
                      class="stroke-gray-200 dark:stroke-gray-700"
                      stroke-dasharray="4 4"
                    />
                    <text
                      x="0"
                      :y="tick.y + 4"
                      class="fill-gray-500 text-[11px] dark:fill-gray-400"
                    >
                      {{ tick.label }}
                    </text>
                  </g>
                  <path
                    :d="dashboardTrendAreaPath"
                    class="fill-gray-900/5 dark:fill-gray-100/5"
                  />
                  <path
                    :d="dashboardTrendPath"
                    fill="none"
                    class="stroke-gray-900 dark:stroke-gray-100"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  />
                  <g
                    v-for="point in dashboardTrendPoints"
                    :key="point.date"
                    class="group outline-none"
                    tabindex="0"
                  >
                    <line
                      :x1="point.x"
                      :x2="point.x"
                      y1="18"
                      y2="150"
                      class="stroke-transparent transition-colors group-hover:stroke-gray-300 group-focus:stroke-gray-300 dark:group-hover:stroke-gray-600 dark:group-focus:stroke-gray-600"
                      stroke-width="1"
                    />
                    <circle
                      :cx="point.x"
                      :cy="point.y"
                      r="5"
                      class="fill-gray-900 stroke-white stroke-2 dark:fill-gray-100 dark:stroke-gray-900"
                    />
                    <g
                      class="opacity-0 transition-opacity group-hover:opacity-100 group-focus:opacity-100"
                      :transform="`translate(${Math.min(
                        Math.max(point.x - 58, 58),
                        500
                      )} ${Math.max(point.y - 72, 6)})`"
                    >
                      <rect
                        width="116"
                        height="54"
                        rx="8"
                        class="fill-gray-900 dark:fill-gray-100"
                      />
                      <text
                        x="12"
                        y="20"
                        class="fill-white text-[11px] dark:fill-gray-900"
                      >
                        {{ point.date }} 周{{ point.weekday }}
                      </text>
                      <text
                        x="12"
                        y="40"
                        class="fill-white text-[13px] font-semibold dark:fill-gray-900"
                      >
                        完成：{{ point.wordLabel }}
                      </text>
                    </g>
                    <text
                      :x="point.x"
                      y="172"
                      text-anchor="middle"
                      class="fill-gray-500 text-[11px] dark:fill-gray-400"
                    >
                      {{ point.dayLabel }}
                    </text>
                  </g>
                </svg>
              </div>
              <div class="grid gap-3 sm:grid-cols-3">
                <div
                  v-for="item in dashboardTrendSummary"
                  :key="item.label"
                  class="rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 dark:border-gray-800 dark:bg-gray-800/60"
                >
                  <div class="text-xs text-gray-500 dark:text-gray-400">
                    {{ item.label }}
                  </div>
                  <div
                    class="mt-1 text-base font-semibold text-gray-900 dark:text-white"
                  >
                    {{ item.value }}
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section
            class="flex min-h-0 flex-col rounded-xl border border-gray-200 bg-white p-3 shadow-sm dark:border-gray-800 dark:bg-gray-900"
          >
            <div class="flex items-center justify-between gap-3">
              <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                写作提示
              </h3>
              <span class="text-xs text-gray-400"
                >{{ dashboardTipCards.length }} 条建议</span
              >
            </div>
            <div class="mt-3 grid min-h-0 flex-1 gap-3">
              <div
                v-for="tip in dashboardTipCards"
                :key="tip.title"
                class="flex min-h-0 items-center gap-4 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 dark:border-gray-800 dark:bg-gray-800/60"
              >
                <div
                  class="flex size-12 shrink-0 items-center justify-center rounded-lg bg-white text-gray-900 shadow-sm dark:bg-gray-900 dark:text-white"
                >
                  <component :is="tip.icon" class="size-7" />
                </div>
                <div class="min-w-0">
                  <p
                    class="text-sm font-semibold text-gray-900 dark:text-white"
                  >
                    {{ tip.title }}
                  </p>
                  <p
                    class="mt-0.5 text-xs leading-5 text-gray-500 dark:text-gray-400"
                  >
                    {{ tip.desc }}
                  </p>
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>

    <!-- Novel Setup -->
    <div
      v-if="store.viewMode === 'chat' && store.isNovelSetupOpen"
      class="flex flex-1 overflow-hidden px-4 py-3 sm:px-6"
    >
      <div class="mx-auto flex h-full w-full max-w-6xl flex-col justify-center">
        <div class="mb-2 shrink-0">
          <h2 class="text-xl font-bold text-gray-900 dark:text-white">
            新建小说
          </h2>
          <div class="mt-0.5 flex items-center justify-between gap-3">
            <p class="text-xs text-gray-500 dark:text-gray-400">
              完善小说设定，一旦点击“开始创建”，将无法更改，AI
              将基于你的设定生成章节内容
            </p>
            <span
              class="shrink-0 text-xs"
              :class="
                isNovelSetupDirty
                  ? 'font-medium text-red-500 dark:text-red-400'
                  : 'text-gray-500 dark:text-gray-400'
              "
            >
              {{ isNovelSetupDirty ? "未保存" : "已保存" }}
            </span>
          </div>
        </div>

        <div class="flex min-h-0 flex-col gap-4">
          <section
            class="shrink-0 rounded-xl border border-gray-200 bg-white p-3.5 shadow-sm dark:border-gray-800 dark:bg-gray-900"
          >
            <div>
              <label
                class="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400"
              >
                <span
                  class="h-3 w-1 rounded-full bg-gray-900 dark:bg-gray-200"
                />
                小说名称 <span class="text-red-500">*</span>
              </label>
              <input
                v-model="setupTitle"
                class="mt-1.5 w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300"
                placeholder="例如：重回2010"
              />
            </div>

            <div class="mt-2.5">
              <label
                class="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400"
              >
                <span
                  class="h-3 w-1 rounded-full bg-gray-900 dark:bg-gray-200"
                />
                核心设定 <span class="text-red-500">*</span>
              </label>
              <div class="relative mt-1.5">
                <textarea
                  v-model="setupDirection"
                  rows="3"
                  class="w-full resize-none rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm leading-6 text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300"
                  placeholder="写下故事背景、核心冲突、目标、世界观等关键信息。"
                />
                <span class="absolute bottom-3 right-3 text-xs text-gray-400"
                  >{{ setupDirection.length }} 字</span
                >
              </div>
            </div>
          </section>

          <section
            class="grid shrink-0 items-stretch gap-3 lg:grid-cols-[300px_minmax(0,1fr)]"
          >
            <div
              class="flex flex-col rounded-xl border border-gray-200 bg-white p-3 shadow-sm dark:border-gray-800 dark:bg-gray-900"
            >
              <div
                class="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400"
              >
                <span
                  class="h-3 w-1 rounded-full bg-gray-900 dark:bg-gray-200"
                />
                关键设定
                <span
                  class="inline-flex shrink-0 cursor-help text-gray-400"
                  title="卷规划的时候会根据剧情新增临时设定"
                >
                  <Info class="size-3.5" />
                </span>
              </div>
              <div class="grid flex-1 auto-rows-fr gap-1.5">
                <button
                  class="group flex w-full items-center gap-3 rounded-lg border border-gray-200 px-2 py-2 text-left transition-colors hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"
                  @click="openCharacterList"
                >
                  <span
                    class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300"
                  >
                    <Users class="size-4" />
                  </span>
                  <span class="min-w-0 flex-1">
                    <span
                      class="block text-sm font-medium text-gray-900 dark:text-white"
                      >人物设定</span
                    >
                    <span class="mt-0.5 block text-xs text-gray-500">
                      {{ setupCharacters.length }} 人 · 管理角色信息与关系
                    </span>
                  </span>
                  <ChevronRight
                    class="size-4 shrink-0 text-gray-400 transition-transform group-hover:translate-x-0.5"
                  />
                </button>

                <button
                  class="group flex w-full items-center gap-3 rounded-lg border border-gray-200 px-2 py-2 text-left transition-colors hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"
                  @click="openMapList"
                >
                  <span
                    class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300"
                  >
                    <MapPinned class="size-4" />
                  </span>
                  <span class="min-w-0 flex-1">
                    <span
                      class="block text-sm font-medium text-gray-900 dark:text-white"
                      >地点设定</span
                    >
                    <span class="mt-0.5 block text-xs text-gray-500">
                      {{ setupMaps.length }} 处 · 管理场景与地理信息
                    </span>
                  </span>
                  <ChevronRight
                    class="size-4 shrink-0 text-gray-400 transition-transform group-hover:translate-x-0.5"
                  />
                </button>

                <button
                  class="group flex w-full items-center gap-3 rounded-lg border border-gray-200 px-2 py-2 text-left transition-colors hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"
                  @click="openFixedOtherItems"
                >
                  <span
                    class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300"
                  >
                    <Target class="size-4" />
                  </span>
                  <span class="min-w-0 flex-1">
                    <span
                      class="block text-sm font-medium text-gray-900 dark:text-white"
                      >势力设定</span
                    >
                    <span class="mt-0.5 block text-xs text-gray-500">
                      {{ setupForces.length }} 个 · 管理阵营、组织与势力关系
                    </span>
                  </span>
                  <ChevronRight
                    class="size-4 shrink-0 text-gray-400 transition-transform group-hover:translate-x-0.5"
                  />
                </button>

                <button
                  class="group flex w-full items-center gap-3 rounded-lg border border-gray-200 px-2 py-2 text-left transition-colors hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"
                  @click="openOtherSettingsList"
                >
                  <span
                    class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300"
                  >
                    <Puzzle class="size-4" />
                  </span>
                  <span class="min-w-0 flex-1">
                    <span
                      class="block text-sm font-medium text-gray-900 dark:text-white"
                      >其他设定</span
                    >
                    <span class="mt-0.5 block text-xs text-gray-500">
                      {{ setupOtherSettings.length }} 类 ·
                      货币、装备、规则等自定义设定
                    </span>
                  </span>
                  <ChevronRight
                    class="size-4 shrink-0 text-gray-400 transition-transform group-hover:translate-x-0.5"
                  />
                </button>
              </div>
            </div>

            <div
              class="rounded-xl border border-gray-200 bg-white p-3 shadow-sm dark:border-gray-800 dark:bg-gray-900"
            >
              <div>
                <div
                  class="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400"
                >
                  <span
                    class="h-3 w-1 rounded-full bg-gray-900 dark:bg-gray-200"
                  />
                  篇幅选择
                </div>
                <div class="grid gap-2 sm:grid-cols-3">
                  <button
                    v-for="option in setupLengthOptions"
                    :key="option.label"
                    class="relative rounded-lg border px-3 py-2 text-left transition-colors"
                    :class="
                      setupLength === option.label
                        ? 'border-gray-900 bg-white text-gray-900 dark:border-white dark:bg-gray-800 dark:text-white'
                        : 'border-gray-200 bg-gray-50 text-gray-600 hover:bg-white dark:border-gray-700 dark:bg-gray-800/60 dark:text-gray-400 dark:hover:bg-gray-800'
                    "
                    @click="setupLength = option.label"
                  >
                    <span class="block text-sm font-medium">{{
                      option.label
                    }}</span>
                    <span
                      class="mt-0.5 block text-xs text-gray-500 dark:text-gray-400"
                      >{{ option.desc }}</span
                    >
                    <Check
                      v-if="setupLength === option.label"
                      class="absolute right-2 top-2 size-4"
                    />
                  </button>
                </div>
              </div>

              <div class="mt-7">
                <div
                  class="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400"
                >
                  <span
                    class="h-3 w-1 rounded-full bg-gray-900 dark:bg-gray-200"
                  />
                  叙事视角
                </div>
                <div class="grid gap-2 sm:grid-cols-3">
                  <button
                    v-for="option in setupPerspectiveOptions"
                    :key="option.label"
                    class="relative rounded-lg border px-3 py-2 text-left transition-colors"
                    :class="
                      setupPerspective === option.label
                        ? 'border-gray-900 bg-white text-gray-900 dark:border-white dark:bg-gray-800 dark:text-white'
                        : 'border-gray-200 bg-gray-50 text-gray-600 hover:bg-white dark:border-gray-700 dark:bg-gray-800/60 dark:text-gray-400 dark:hover:bg-gray-800'
                    "
                    @click="setupPerspective = option.label"
                  >
                    <span class="block text-sm font-medium">{{
                      option.label
                    }}</span>
                    <span
                      class="mt-0.5 block text-xs text-gray-500 dark:text-gray-400"
                      >{{ option.desc }}</span
                    >
                    <Check
                      v-if="setupPerspective === option.label"
                      class="absolute right-2 top-2 size-4"
                    />
                  </button>
                </div>
              </div>

              <div class="mt-7 space-y-2.5">
                <div
                  class="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400"
                >
                  <span
                    class="h-3 w-1 rounded-full bg-gray-900 dark:bg-gray-200"
                  />
                  标签选择
                </div>
                <div
                  v-for="group in setupTagGroups"
                  :key="group.title"
                  class="flex items-start gap-1.5"
                >
                  <div
                    class="w-12 shrink-0 text-sm font-medium text-gray-800 dark:text-gray-200"
                  >
                    {{ group.title }}
                  </div>
                  <div
                    class="flex min-w-0 flex-1 flex-wrap items-center gap-1.5"
                  >
                    <button
                      v-for="tag in group.tags"
                      :key="tag"
                      class="rounded-full border px-2 py-0.5 text-xs transition-colors"
                      :class="
                        setupTags.includes(tag)
                          ? 'border-gray-900 bg-gray-900 text-white dark:border-gray-100 dark:bg-gray-100 dark:text-gray-900'
                          : 'border-gray-200 bg-gray-50 text-gray-600 hover:bg-white dark:border-gray-700 dark:bg-gray-900 dark:text-gray-400 dark:hover:bg-gray-800'
                      "
                      @click="toggleSetupTag(tag)"
                    >
                      {{ tag }}
                    </button>
                    <button
                      v-for="tag in setupCustomAddedTags[group.title] || []"
                      :key="`${group.title}-${tag}`"
                      class="max-w-28 truncate whitespace-nowrap rounded-full border border-gray-900 bg-gray-900 px-2 py-0.5 text-xs text-white transition-colors dark:border-gray-100 dark:bg-gray-100 dark:text-gray-900"
                      @click="
                        setupCustomAddedTags[group.title] = (
                          setupCustomAddedTags[group.title] || []
                        ).filter((item) => item !== tag)
                      "
                    >
                      {{ tag }}
                    </button>
                    <input
                      v-model="setupCustomTags[group.title]"
                      class="min-w-0 flex-1 basis-24 overflow-x-auto whitespace-nowrap rounded-full border border-gray-200 bg-gray-50 px-2 py-0.5 text-xs text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300"
                      placeholder="+ 自定义"
                      @keydown.enter.prevent="addSetupCustomTag(group.title)"
                    />
                  </div>
                </div>
              </div>
            </div>
          </section>

          <div class="flex shrink-0 items-center justify-between gap-2">
            <div
              class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400"
            >
              <template v-if="hasNovelSetupPlanData">
                <span>已生成具体模板</span>
                <button
                  class="font-medium text-gray-900 underline underline-offset-2 dark:text-gray-100"
                  :disabled="isSetupAssistLoading"
                  @click="reopenSetupAssistModal"
                >
                  重新描述/上传
                </button>
                <button
                  class="inline-flex items-center gap-1 font-medium text-gray-900 hover:underline dark:text-gray-100"
                  @click="regenerateSetupTemplate"
                >
                  <X v-if="isSetupAssistLoading" class="size-3.5" />
                  <RefreshCw v-else class="size-3.5" />
                  {{ isSetupAssistLoading ? "取消" : "重新生成" }}
                </button>
                <span
                  v-if="isSetupAssistLoading"
                  class="inline-flex min-w-0 items-center gap-1.5 text-xs text-gray-400 dark:text-gray-500"
                >
                  <Loader2 class="size-3.5 shrink-0 animate-spin" />
                  <Transition name="setup-thinking-slide" mode="out-in">
                    <span
                      :key="setupAssistThinkingText"
                      class="ai-thinking-placeholder truncate"
                    >
                      {{ setupAssistThinkingText }}
                    </span>
                  </Transition>
                  <span class="shrink-0 tabular-nums">{{
                    setupAssistElapsed()
                  }}</span>
                </span>
              </template>
              <template v-else>
                <button
                  class="font-medium text-gray-900 underline underline-offset-2 dark:text-gray-100"
                  @click="openSetupAssistModal"
                >
                  生成具体模板
                </button>
                <span>，一段话描述你的想法，AI 自动补全表单</span>
              </template>
            </div>
            <div class="flex shrink-0 justify-end gap-2">
              <button
                class="rounded-lg border border-gray-200 px-4 py-1.5 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-900"
                @click="store.cancelNovelSetup()"
              >
                取消
              </button>
              <button
                class="rounded-lg border border-gray-200 px-4 py-1.5 text-sm text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-900"
                :disabled="
                  store.isNovelCreating ||
                  isSetupAssistLoading ||
                  !isNovelSetupDirty
                "
                @click="saveNovelSetupDraft"
              >
                {{
                  store.isNovelCreating
                    ? "保存中..."
                    : isSetupAssistLoading
                    ? "生成中..."
                    : "暂存设定"
                }}
              </button>
              <button
                class="rounded-lg bg-gray-900 px-5 py-1.5 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
                :disabled="store.isNovelCreating || isSetupAssistLoading"
                @click="submitNovelSetup"
              >
                {{
                  store.isNovelCreating
                    ? "创建中..."
                    : isSetupAssistLoading
                    ? "生成中..."
                    : "开始创建"
                }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div
      v-if="isSetupAssistOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/30 px-4"
      @click.self="closeSetupAssistModalByBackdrop"
    >
      <div
        class="w-full max-w-xl rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900"
      >
        <div class="mb-4 flex items-center justify-between">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              自动生成具体模板
            </h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              写几句话，或上传 .txt / .docx / .md，AI 会识别并填充新建小说表单。
            </p>
          </div>
          <button
            class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
            @click="closeSetupAssistModal"
          >
            <X class="size-4" />
          </button>
        </div>

        <div
          class="flex h-52 flex-col rounded-lg border border-gray-200 bg-white p-2 focus-within:border-gray-400 dark:border-gray-700 dark:bg-gray-950"
          @dragover.prevent
          @drop.prevent="onSetupAssistDrop"
        >
          <div
            v-if="setupAssistFiles.length"
            class="mb-2 flex max-h-16 min-h-10 flex-wrap gap-2 overflow-y-auto"
          >
            <button
              v-for="(file, index) in setupAssistFiles"
              :key="`${file.name}-${file.size}-${file.lastModified}`"
              class="group inline-flex max-w-56 items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-2.5 py-1.5 text-xs text-gray-600 hover:bg-white dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300"
              :title="file.name"
              @click="downloadSetupAssistFile(file)"
            >
              <FileText class="size-3.5 shrink-0 text-gray-400" />
              <span class="truncate">{{ file.name }}</span>
              <span
                class="rounded p-0.5 text-gray-400 hover:bg-gray-200 hover:text-gray-700 dark:hover:bg-gray-800 dark:hover:text-gray-100"
                @click.stop="removeSetupAssistFile(index)"
              >
                <X class="size-3" />
              </span>
            </button>
          </div>
          <textarea
            v-model="setupAssistText"
            class="min-h-0 flex-1 resize-none bg-transparent px-1 py-1 text-sm leading-6 text-gray-700 outline-none placeholder:text-gray-400 dark:text-gray-300"
            placeholder="几句话描述，或者将你准备好的文件上传/拖拽到这里。"
          />
          <div class="mt-2 flex items-center">
            <label
              class="inline-flex size-8 cursor-pointer items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 hover:text-gray-800 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-100"
              title="添加文件"
            >
              <Plus class="size-4" />
              <input
                class="hidden"
                type="file"
                multiple
                accept=".txt,.md,.docx"
                @change="onSetupAssistFileChange"
              />
            </label>
          </div>
        </div>

        <div class="mt-5 flex items-center justify-between gap-3">
          <div class="model-selector relative">
            <button
              class="flex max-w-36 items-center gap-1 rounded-lg border border-gray-200 px-2.5 py-2 text-xs text-gray-600 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-700"
              :disabled="isSetupAssistLoading"
              @click.stop="setupAssistModelOpen = !setupAssistModelOpen"
            >
              <span class="truncate">{{
                setupAssistSelectedModel?.name || "选择模型"
              }}</span>
              <ChevronDown class="size-3.5" />
            </button>
            <div
              v-if="setupAssistModelOpen"
              class="absolute bottom-full left-0 z-30 mb-1 w-40 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
            >
              <button
                v-for="model in store.models"
                :key="model.id"
                class="w-full truncate px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                :class="
                  setupAssistModelId === model.id
                    ? 'bg-gray-100 dark:bg-gray-700'
                    : ''
                "
                @click="selectSetupAssistModel(model.id)"
              >
                {{ model.name }}
              </button>
              <button
                class="w-full border-t border-gray-100 px-3 py-2 text-left text-sm text-blue-500 hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-700"
                @click="openSetupAssistMoreModels"
              >
                添加更多
              </button>
            </div>
          </div>
          <div
            v-if="isSetupAssistLoading"
            class="flex min-w-0 flex-1 items-center justify-end gap-1.5 text-xs text-gray-400 dark:text-gray-500"
          >
            <Transition name="setup-thinking-slide" mode="out-in">
              <span
                :key="setupAssistThinkingText"
                class="ai-thinking-placeholder truncate"
              >
                {{ setupAssistThinkingText }}
              </span>
            </Transition>
            <span class="shrink-0 tabular-nums">{{
              setupAssistElapsed()
            }}</span>
            <Loader2 class="size-3.5 shrink-0 animate-spin" />
          </div>
          <div class="flex shrink-0 justify-end gap-2">
            <button
              class="rounded-lg border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
              @click="closeSetupAssistModal"
            >
              {{ isSetupAssistLoading ? "取消生成" : "取消" }}
            </button>
            <button
              class="inline-flex items-center gap-2 rounded-lg bg-gray-900 px-5 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
              :disabled="isSetupAssistLoading"
              @click="completeSetupFromAssist()"
            >
              确定
            </button>
          </div>
        </div>
      </div>
    </div>

    <div
      v-if="isCharacterListOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/30 px-4"
      @click.self="isCharacterListOpen = false"
    >
      <div
        class="flex max-h-[72vh] w-full max-w-lg flex-col rounded-xl bg-white p-4 shadow-xl dark:bg-gray-900"
      >
        <div class="mb-4 flex items-center justify-between">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              人物设定
            </h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              共 {{ setupCharacters.length }} 人，AI
              不会改动下方人物，只会按剧情适当新增人物。
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button
              class="rounded-lg border border-gray-200 p-2 text-gray-500 hover:bg-gray-50 disabled:cursor-not-allowed disabled:text-gray-300 disabled:hover:bg-transparent dark:border-gray-700 dark:text-gray-400 dark:hover:bg-gray-800 dark:disabled:text-gray-600 dark:disabled:hover:bg-transparent"
              :title="setupCharacters.length > 0 ? '查看人物关系图' : ''"
              :disabled="setupCharacters.length === 0"
              @click="openCharacterGraph"
            >
              <Network class="size-4" />
            </button>
            <button
              class="rounded-lg border border-gray-200 p-2 text-gray-500 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-400 dark:hover:bg-gray-800"
              title="创建人物"
              @click="openCharacterModal()"
            >
              <Plus class="size-4" />
            </button>
            <button
              class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
              @click="isCharacterListOpen = false"
            >
              <X class="size-4" />
            </button>
          </div>
        </div>
        <div
          v-if="setupCharacters.length === 0"
          class="flex min-h-48 flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 text-center text-gray-400 dark:border-gray-700"
        >
          <Users class="size-8" />
          <p class="mt-2 text-sm">还没有创建人物</p>
          <p class="mt-1 text-xs">点击右上角“+”添加人物</p>
        </div>
        <div v-else class="min-h-0 flex-1 space-y-2 overflow-y-auto pr-1">
          <button
            v-for="character in setupCharacters"
            :key="character.id"
            class="group flex w-full items-start gap-3 rounded-lg border border-gray-200 p-3 text-left hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"
            @click="openCharacterModal(character)"
          >
            <Users class="mt-0.5 size-4 shrink-0 text-gray-400" />
            <span class="min-w-0 flex-1">
              <span
                class="block truncate text-sm font-medium text-gray-900 dark:text-white"
              >
                {{ character.name }}
              </span>
              <span
                class="mt-1 block max-h-12 overflow-hidden break-words text-xs leading-5 text-gray-500 dark:text-gray-400"
              >
                {{
                  [
                    character.appearanceTime
                      ? `出场时间：${character.appearanceTime}`
                      : "",
                    character.notes || "未填写详细信息",
                  ]
                    .filter(Boolean)
                    .join("；")
                }}
              </span>
            </span>
            <span
              class="shrink-0 rounded p-1 text-gray-400 opacity-0 hover:bg-gray-100 hover:text-red-500 group-hover:opacity-100 dark:hover:bg-gray-700"
              @click.stop="removeSetupCharacter(character.id)"
            >
              <Trash2 class="size-3.5" />
            </span>
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="isCharacterGraphOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/30 px-4"
      tabindex="-1"
      @click.self="isCharacterGraphOpen = false"
      @keydown="handleGraphKeydown"
    >
      <div
        class="relative flex h-[86vh] max-h-[86vh] w-full max-w-3xl flex-col rounded-xl bg-white p-4 shadow-xl dark:bg-gray-900"
      >
        <div class="mb-4 flex items-center justify-between">
          <div>
            <div class="flex items-center gap-2">
              <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                人物关系图
              </h3>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              共 {{ setupCharacters.length }} 人 ·
              {{ graphRelationshipCount }} 条关系
            </p>
          </div>
          <button
            class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
            @click="isCharacterGraphOpen = false"
          >
            <X class="size-4" />
          </button>
        </div>
        <div
          v-if="setupCharacters.length === 0"
          class="flex min-h-56 flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 text-center text-gray-400 dark:border-gray-700"
        >
          <Network class="size-8" />
          <p class="mt-2 text-sm">暂无人物可视化数据</p>
        </div>
        <CharacterRelationGraph
          v-else
          class="min-h-0 flex-1"
          :nodes="characterGraphNodes"
          :edges="characterGraphEdges"
          :selected-node-id="selectedCharacterGraphNodeId"
          :selected-edge-id="
            editingGraphRelationship
              ? `relationship-${editingGraphRelationshipIndex}`
              : null
          "
          :pending-delete-edge-id="pendingGraphRelationshipDelete?.id || null"
          editable
          deletable-edges
          @stage-click="clearCharacterGraphSelection"
          @node-click="handleCharacterGraphNodeClick"
          @edge-click="openRelationshipByGraphEdge"
          @edge-context-menu="handleCharacterGraphEdgeContextMenu"
          @delete-pending-edge="deletePendingGraphRelationship"
          @node-position-change="updateCharacterGraphNodePosition"
        />
        <div
          v-if="selectedCharacterGraphCharacter"
          class="absolute right-6 top-20 z-10 w-72 rounded-lg border border-gray-200 bg-white/95 p-3 shadow-lg backdrop-blur dark:border-gray-700 dark:bg-gray-900/95"
          @click.stop
        >
          <div class="mb-3 flex items-start justify-between gap-2">
            <div class="min-w-0">
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                人物详情
              </h4>
            </div>
            <button
              class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
              @click="selectedCharacterGraphNodeId = null"
            >
              <X class="size-3.5" />
            </button>
          </div>
          <div class="space-y-2">
            <label
              class="block text-xs font-medium text-gray-500 dark:text-gray-400"
            >
              名称
            </label>
            <input
              :value="selectedCharacterGraphCharacter.name"
              class="w-full rounded-lg border border-gray-200 bg-white px-2.5 py-2 text-sm text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
              @input="
                updateSelectedCharacterGraphCharacter(
                  'name',
                  ($event.target as HTMLInputElement).value
                )
              "
            />
            <label
              class="block text-xs font-medium text-gray-500 dark:text-gray-400"
            >
              出场时间
            </label>
            <select
              :value="selectedCharacterGraphCharacter.appearanceTime"
              class="w-full rounded-lg border border-gray-200 bg-white px-2.5 py-2 text-sm text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
              @change="
                updateSelectedCharacterGraphCharacter(
                  'appearanceTime',
                  ($event.target as HTMLSelectElement).value
                )
              "
            >
              <option v-for="option in appearanceTimeOptions" :key="option">
                {{ option }}
              </option>
            </select>
            <label
              class="block text-xs font-medium text-gray-500 dark:text-gray-400"
            >
              详细信息
            </label>
            <textarea
              :value="selectedCharacterGraphCharacter.notes"
              rows="5"
              class="w-full resize-none rounded-lg border border-gray-200 bg-white px-2.5 py-2 text-sm leading-5 text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
              @input="
                updateSelectedCharacterGraphCharacter(
                  'notes',
                  ($event.target as HTMLTextAreaElement).value
                )
              "
            />
          </div>
        </div>
        <div
          v-if="editingGraphRelationship"
          class="absolute right-6 top-20 z-20 w-72 rounded-lg border border-gray-200 bg-white/95 p-3 shadow-lg backdrop-blur dark:border-gray-700 dark:bg-gray-900/95"
          @click.stop
        >
          <div class="mb-3 flex items-start justify-between gap-2">
            <div class="min-w-0">
              <p class="text-xs text-gray-400 dark:text-gray-500">关系说明</p>
              <h4
                class="mt-0.5 truncate text-sm font-semibold text-gray-900 dark:text-white"
              >
                {{ characterNameById(editingGraphRelationship.characterA) }}
                ↔
                {{ characterNameById(editingGraphRelationship.characterB) }}
              </h4>
            </div>
            <button
              class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
              @click="closeGraphRelationshipEditor"
            >
              <X class="size-3.5" />
            </button>
          </div>
          <textarea
            :value="editingGraphRelationship.description"
            rows="6"
            class="w-full resize-none rounded-lg border border-gray-200 bg-white px-2.5 py-2 text-sm leading-5 text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
            placeholder="写清这条人物关系在前期、中期、后期的变化。"
            @input="
              updateCharacterRelationship(
                editingGraphRelationshipIndex as number,
                'description',
                ($event.target as HTMLTextAreaElement).value
              )
            "
          />
        </div>
      </div>
    </div>

    <div
      v-if="isMapListOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/30 px-4"
      @click.self="isMapListOpen = false"
    >
      <div
        class="flex max-h-[72vh] w-full max-w-lg flex-col rounded-xl bg-white p-4 shadow-xl dark:bg-gray-900"
      >
        <div class="mb-4 flex items-center justify-between">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              地点设定
            </h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              共
              {{ setupMaps.length }} 处，用于管理关键场景、地理信息与剧情作用。
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button
              class="rounded-lg border border-gray-200 p-2 text-gray-500 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-400 dark:hover:bg-gray-800"
              title="创建地点"
              @click="openMapModal()"
            >
              <Plus class="size-4" />
            </button>
            <button
              class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
              @click="isMapListOpen = false"
            >
              <X class="size-4" />
            </button>
          </div>
        </div>
        <div
          v-if="setupMaps.length === 0"
          class="flex min-h-48 flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 text-center text-gray-400 dark:border-gray-700"
        >
          <MapPinned class="size-8" />
          <p class="mt-2 text-sm">还没有创建地点</p>
          <p class="mt-1 text-xs">点击右上角“+”添加地点</p>
        </div>
        <div v-else class="min-h-0 flex-1 space-y-2 overflow-y-auto pr-1">
          <button
            v-for="item in setupMaps"
            :key="item.id"
            class="group flex w-full items-start gap-3 rounded-lg border border-gray-200 p-3 text-left hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"
            @click="openMapModal(item)"
          >
            <MapPinned class="mt-0.5 size-4 shrink-0 text-gray-400" />
            <span class="min-w-0 flex-1">
              <span
                class="block truncate text-sm font-medium text-gray-900 dark:text-white"
              >
                {{ item.name }}
              </span>
              <span
                class="mt-1 block max-h-12 overflow-hidden break-words text-xs leading-5 text-gray-500 dark:text-gray-400"
              >
                {{
                  [
                    item.appearanceTime
                      ? `出场时间：${item.appearanceTime}`
                      : "",
                    item.notes || "未填写详细信息",
                  ]
                    .filter(Boolean)
                    .join("；")
                }}
              </span>
            </span>
            <span
              class="shrink-0 rounded p-1 text-gray-400 opacity-0 hover:bg-gray-100 hover:text-red-500 group-hover:opacity-100 dark:hover:bg-gray-700"
              @click.stop="removeSetupMap(item.id)"
            >
              <Trash2 class="size-3.5" />
            </span>
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="isFixedOtherItemsOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/30 px-4"
      @click.self="isFixedOtherItemsOpen = false"
    >
      <div
        class="flex max-h-[72vh] w-full max-w-lg flex-col rounded-xl bg-white p-4 shadow-xl dark:bg-gray-900"
      >
        <div class="mb-4 flex items-center justify-between">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              势力设定
            </h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              共
              {{ setupForces.length }}
              项，按故事需要填写，不必强行补全。
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button
              class="rounded-lg border border-gray-200 p-2 text-gray-500 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-400 dark:hover:bg-gray-800"
              title="添加内容"
              @click="openFixedOtherItemModal()"
            >
              <Plus class="size-4" />
            </button>
            <button
              class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
              @click="isFixedOtherItemsOpen = false"
            >
              <X class="size-4" />
            </button>
          </div>
        </div>
        <div
          v-if="setupForces.length === 0"
          class="flex min-h-48 flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 text-center text-gray-400 dark:border-gray-700"
        >
          <Target class="size-8" />
          <p class="mt-2 text-sm">还没有势力设定</p>
          <p class="mt-1 text-xs">点击右上角“+”添加内容</p>
        </div>
        <div v-else class="min-h-0 flex-1 space-y-2 overflow-y-auto pr-1">
          <button
            v-for="item in setupForces"
            :key="item.id"
            class="group flex w-full items-start gap-3 rounded-lg border border-gray-200 p-3 text-left hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"
            @click="openFixedOtherItemModal(item)"
          >
            <Target class="mt-0.5 size-4 shrink-0 text-gray-400" />
            <span class="min-w-0 flex-1">
              <span
                class="block truncate text-sm font-medium text-gray-900 dark:text-white"
              >
                {{ item.name }}
              </span>
              <span
                class="mt-1 block max-h-12 overflow-hidden break-words text-xs leading-5 text-gray-500 dark:text-gray-400"
              >
                {{ item.notes || "未填写详细信息" }}
              </span>
            </span>
            <span
              class="shrink-0 rounded p-1 text-gray-400 opacity-0 hover:bg-gray-100 hover:text-red-500 group-hover:opacity-100 dark:hover:bg-gray-700"
              @click.stop="removeSetupForce(item.id)"
            >
              <Trash2 class="size-3.5" />
            </span>
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="isOtherSettingsOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/30 px-4"
      @click.self="isOtherSettingsOpen = false"
    >
      <div
        class="flex max-h-[72vh] w-full max-w-lg flex-col rounded-xl bg-white p-4 shadow-xl dark:bg-gray-900"
      >
        <div class="mb-4 flex items-center justify-between">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              其他设定
            </h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              共
              {{ setupOtherSettings.length }}
              类，可管理货币、装备、规则等自定义内容。
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button
              class="rounded-lg border border-gray-200 p-2 text-gray-500 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-400 dark:hover:bg-gray-800"
              title="创建设定类型"
              @click="openOtherSettingModal()"
            >
              <Plus class="size-4" />
            </button>
            <button
              class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
              @click="isOtherSettingsOpen = false"
            >
              <X class="size-4" />
            </button>
          </div>
        </div>
        <div
          v-if="setupOtherSettings.length === 0"
          class="flex min-h-56 flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 text-center text-gray-400 dark:border-gray-700"
        >
          <Puzzle class="size-8" />
          <p class="mt-2 text-sm">还没有其他设定</p>
          <p class="mt-1 text-xs">点击右上角“+”创建设定类型</p>
        </div>
        <div v-else class="min-h-0 flex-1 space-y-3 overflow-y-auto pr-1">
          <div
            v-for="setting in setupOtherSettings"
            :key="setting.id"
            class="rounded-lg border border-gray-200 p-3 dark:border-gray-700"
          >
            <button
              class="group flex w-full items-start gap-3 text-left"
              @click="selectOtherSetting(setting.id)"
            >
              <Puzzle class="mt-0.5 size-4 shrink-0 text-gray-400" />
              <span class="min-w-0 flex-1">
                <span class="flex items-center gap-2">
                  <span
                    class="truncate text-sm font-semibold text-gray-900 dark:text-white"
                  >
                    {{ setting.title }}
                  </span>
                  <span
                    class="shrink-0 rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-gray-500 dark:bg-gray-800 dark:text-gray-400"
                  >
                    {{ setting.items.length }} 项
                  </span>
                </span>
                <span
                  class="mt-1 block max-h-10 overflow-hidden break-words text-xs leading-5 text-gray-500 dark:text-gray-400"
                >
                  {{ setting.description || "未填写作用说明" }}
                </span>
              </span>
              <ChevronDown
                class="size-4 shrink-0 text-gray-400 transition-transform"
                :class="activeOtherSettingId === setting.id ? 'rotate-180' : ''"
              />
            </button>
            <div
              v-if="activeOtherSettingId === setting.id"
              class="mt-3 space-y-2 border-t border-gray-100 pt-3 dark:border-gray-800"
            >
              <div class="flex items-center justify-between gap-2">
                <button
                  class="text-xs font-medium text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
                  @click="openOtherSettingModal(setting)"
                >
                  编辑类型
                </button>
                <div class="flex items-center gap-2">
                  <button
                    class="rounded-lg border border-gray-200 px-2.5 py-1.5 text-xs text-gray-600 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
                    @click="openOtherItemModal(setting.id)"
                  >
                    添加内容
                  </button>
                  <button
                    class="rounded-lg px-2.5 py-1.5 text-xs text-red-500 hover:bg-red-50 dark:hover:bg-red-950/30"
                    @click="removeSetupOtherSetting(setting.id)"
                  >
                    删除类型
                  </button>
                </div>
              </div>
              <div
                v-if="setting.items.length === 0"
                class="rounded-lg bg-gray-50 px-3 py-4 text-center text-xs text-gray-400 dark:bg-gray-800/60"
              >
                还没有具体内容，可添加货币、装备、规则、禁忌等。
              </div>
              <button
                v-for="item in setting.items"
                :key="item.id"
                class="group flex w-full items-start gap-2 rounded-lg bg-gray-50 p-2.5 text-left hover:bg-gray-100 dark:bg-gray-800/60 dark:hover:bg-gray-800"
                @click="openOtherItemModal(setting.id, item)"
              >
                <span class="min-w-0 flex-1">
                  <span
                    class="block truncate text-sm font-medium text-gray-900 dark:text-white"
                  >
                    {{ item.name }}
                  </span>
                  <span
                    class="mt-0.5 block max-h-10 overflow-hidden break-words text-xs leading-5 text-gray-500 dark:text-gray-400"
                  >
                    {{
                      [
                        item.appearanceTime
                          ? `出场时间：${item.appearanceTime}`
                          : "",
                        item.notes || "未填写详细信息",
                      ]
                        .filter(Boolean)
                        .join("；")
                    }}
                  </span>
                </span>
                <span
                  class="shrink-0 rounded p-1 text-gray-400 opacity-0 hover:bg-gray-200 hover:text-red-500 group-hover:opacity-100 dark:hover:bg-gray-700"
                  @click.stop="removeSetupOtherItem(setting.id, item.id)"
                >
                  <Trash2 class="size-3.5" />
                </span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div
      v-if="isCharacterModalOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/30 px-4"
      @click.self="closeCharacterModal"
    >
      <div
        class="w-full max-w-md rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900"
      >
        <div class="mb-4 flex items-center justify-between">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ editingCharacterId ? "编辑人物" : "创建人物" }}
          </h3>
          <button
            class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
            @click="closeCharacterModal"
          >
            <X class="size-4" />
          </button>
        </div>
        <div class="space-y-4">
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              名称 <span class="text-red-500">*</span>
            </label>
            <input
              v-model="characterForm.name"
              class="mt-2 w-full rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
              placeholder="例如：林墨"
              @keydown.enter.prevent="saveSetupCharacter"
            />
          </div>
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              出场时间 <span class="text-red-500">*</span>
            </label>
            <div class="relative mt-2">
              <button
                class="flex w-full items-center justify-between rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
                @click="toggleAppearanceTimeMenu('character')"
              >
                <span class="truncate">{{
                  appearanceTimeValue("character")
                }}</span>
                <ChevronDown class="size-4 shrink-0 text-gray-400" />
              </button>
              <div
                v-if="appearanceTimeMenuOpen === 'character'"
                class="absolute right-0 z-10 mt-1 min-w-full rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
              >
                <button
                  v-for="option in appearanceTimeOptions"
                  :key="option"
                  class="w-full whitespace-nowrap px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                  :class="
                    appearanceTimeValue('character') === option
                      ? 'bg-gray-100 dark:bg-gray-700'
                      : ''
                  "
                  @click="selectAppearanceTime('character', option)"
                >
                  {{ option }}
                </button>
              </div>
            </div>
          </div>
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              详细信息 <span class="text-red-500">*</span>
            </label>
            <textarea
              v-model="characterForm.notes"
              required
              rows="4"
              class="mt-2 w-full resize-none rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm leading-6 text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
              placeholder="身份、性格、能力、目标、秘密、剧情作用等。"
            />
          </div>
        </div>
        <div class="mt-5 flex justify-end gap-2">
          <button
            class="rounded-lg border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
            @click="closeCharacterModal"
          >
            取消
          </button>
          <button
            class="rounded-lg bg-gray-900 px-5 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
            @click="saveSetupCharacter"
          >
            保存
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="isMapModalOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/30 px-4"
      @click.self="closeMapModal"
    >
      <div
        class="w-full max-w-md rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900"
      >
        <div class="mb-4 flex items-center justify-between">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ editingMapId ? "编辑地点" : "创建地点" }}
          </h3>
          <button
            class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
            @click="closeMapModal"
          >
            <X class="size-4" />
          </button>
        </div>
        <div class="space-y-4">
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              名称 <span class="text-red-500">*</span>
            </label>
            <input
              v-model="mapForm.name"
              class="mt-2 w-full rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
              placeholder="例如：青岚城、七班教室、东区黑市"
              @keydown.enter.prevent="saveSetupMap"
            />
          </div>
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              出场时间 <span class="text-red-500">*</span>
            </label>
            <div class="relative mt-2">
              <button
                class="flex w-full items-center justify-between rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
                @click="toggleAppearanceTimeMenu('map')"
              >
                <span class="truncate">{{ appearanceTimeValue("map") }}</span>
                <ChevronDown class="size-4 shrink-0 text-gray-400" />
              </button>
              <div
                v-if="appearanceTimeMenuOpen === 'map'"
                class="absolute right-0 z-10 mt-1 min-w-full rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
              >
                <button
                  v-for="option in appearanceTimeOptions"
                  :key="option"
                  class="w-full whitespace-nowrap px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                  :class="
                    appearanceTimeValue('map') === option
                      ? 'bg-gray-100 dark:bg-gray-700'
                      : ''
                  "
                  @click="selectAppearanceTime('map', option)"
                >
                  {{ option }}
                </button>
              </div>
            </div>
          </div>
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              详细信息 <span class="text-red-500">*</span>
            </label>
            <textarea
              v-model="mapForm.notes"
              required
              rows="4"
              class="mt-2 w-full resize-none rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm leading-6 text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
              placeholder="地理位置、势力归属、规则、资源、危险或剧情作用。"
            />
          </div>
        </div>
        <div class="mt-5 flex justify-end gap-2">
          <button
            class="rounded-lg border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
            @click="closeMapModal"
          >
            取消
          </button>
          <button
            class="rounded-lg bg-gray-900 px-5 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
            @click="saveSetupMap"
          >
            保存
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="isOtherSettingModalOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/30 px-4"
      @click.self="closeOtherSettingModal"
    >
      <div
        class="w-full max-w-md rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900"
      >
        <div class="mb-4 flex items-center justify-between">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ editingOtherSettingId ? "编辑设定类型" : "创建设定类型" }}
          </h3>
          <button
            class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
            @click="closeOtherSettingModal"
          >
            <X class="size-4" />
          </button>
        </div>
        <div class="space-y-4">
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              名称 <span class="text-red-500">*</span>
            </label>
            <input
              v-model="otherSettingForm.title"
              class="mt-2 w-full rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
              placeholder="例如：货币、装备、规则、禁忌"
              @keydown.enter.prevent="saveSetupOtherSetting"
            />
          </div>
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              作用说明
            </label>
            <textarea
              v-model="otherSettingForm.description"
              rows="4"
              class="mt-2 w-full resize-none rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm leading-6 text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
              placeholder="说明这个设定在故事中的作用、规则或与主线的关系。"
            />
          </div>
        </div>
        <div class="mt-5 flex justify-end gap-2">
          <button
            class="rounded-lg border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
            @click="closeOtherSettingModal"
          >
            取消
          </button>
          <button
            class="rounded-lg bg-gray-900 px-5 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
            @click="saveSetupOtherSetting"
          >
            保存
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="isOtherItemModalOpen"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/30 px-4"
      @click.self="closeOtherItemModal"
    >
      <div
        class="flex max-h-[72vh] w-full max-w-md flex-col rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900"
      >
        <div class="mb-4 flex items-center justify-between">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ setupItemModalTitle }}
            </h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ setupItemModalSubtitle }}
            </p>
          </div>
          <button
            class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
            @click="closeOtherItemModal"
          >
            <X class="size-4" />
          </button>
        </div>
        <div class="min-h-0 flex-1 space-y-4 overflow-y-auto pr-1">
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              名称 <span class="text-red-500">*</span>
            </label>
            <input
              :value="setupItemFormName()"
              class="mt-2 w-full rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
              placeholder="例如：灵石、契约规则、禁用道具"
              @input="
                updateSetupItemFormName(
                  ($event.target as HTMLInputElement).value
                )
              "
              @keydown.enter.prevent="saveSetupOtherItem"
            />
          </div>
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              出场时间 <span class="text-red-500">*</span>
            </label>
            <div class="relative mt-2">
              <button
                class="flex w-full items-center justify-between rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
                @click="toggleAppearanceTimeMenu('setup-item')"
              >
                <span class="truncate">{{
                  appearanceTimeValue("setup-item")
                }}</span>
                <ChevronDown class="size-4 shrink-0 text-gray-400" />
              </button>
              <div
                v-if="appearanceTimeMenuOpen === 'setup-item'"
                class="absolute right-0 z-10 mt-1 min-w-full rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
              >
                <button
                  v-for="option in appearanceTimeOptions"
                  :key="option"
                  class="w-full whitespace-nowrap px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                  :class="
                    appearanceTimeValue('setup-item') === option
                      ? 'bg-gray-100 dark:bg-gray-700'
                      : ''
                  "
                  @click="selectAppearanceTime('setup-item', option)"
                >
                  {{ option }}
                </button>
              </div>
            </div>
          </div>
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              详细信息 <span class="text-red-500">*</span>
            </label>
            <textarea
              :value="setupItemFormNotes()"
              required
              rows="4"
              class="mt-2 w-full resize-none rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm leading-6 text-gray-700 outline-none focus:border-gray-400 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300"
              :placeholder="setupItemNotesPlaceholder"
              @input="
                updateSetupItemFormNotes(
                  ($event.target as HTMLTextAreaElement).value
                )
              "
            />
          </div>
        </div>
        <div class="mt-5 flex shrink-0 justify-end gap-2">
          <button
            class="rounded-lg border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
            @click="closeOtherItemModal"
          >
            取消
          </button>
          <button
            class="rounded-lg bg-gray-900 px-5 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
            @click="saveSetupOtherItem"
          >
            保存
          </button>
        </div>
      </div>
    </div>

    <!-- Editor Mode -->
    <template v-if="store.viewMode === 'editor'">
      <div
        v-if="store.editorChapter"
        ref="editorContainer"
        class="flex flex-1 overflow-hidden bg-white dark:bg-gray-950"
      >
        <aside
          class="flex w-14 shrink-0 flex-col items-center gap-2 border-r border-gray-200 py-3 dark:border-gray-800"
        >
          <div
            class="relative"
            @mouseenter="openDraftPicker"
            @mouseleave="closeDraftPickerSoon"
          >
            <button
              class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
              title="选择草稿"
              @click="
                showDraftPicker ? closeDraftPickerSoon() : openDraftPicker()
              "
            >
              <FileText class="size-5" />
            </button>
            <div
              v-if="showDraftPicker"
              class="absolute left-10 top-0 z-30 h-10 w-4"
            />
            <div
              v-if="showDraftPicker"
              class="absolute left-14 top-0 z-30 max-h-72 w-56 overflow-y-auto rounded-lg border border-gray-200 bg-white p-2 shadow-lg dark:border-gray-700 dark:bg-gray-900"
              @mouseenter="openDraftPicker"
            >
              <div class="mb-2 px-2 text-xs font-semibold text-gray-500">
                选择草稿
              </div>
              <div
                v-for="draft in store.currentEditorDrafts"
                :key="draft.id"
                class="group flex w-full items-start gap-1 rounded-md px-2 py-2 text-left text-xs"
                :class="
                  store.editorDraftId === draft.id
                    ? 'bg-gray-100 text-gray-900 dark:bg-gray-800 dark:text-white'
                    : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800'
                "
              >
                <button
                  class="min-w-0 flex-1 text-left"
                  @click="
                    store.selectEditorDraft(draft.id);
                    showDraftPicker = false;
                  "
                >
                  <span class="flex items-center gap-1 truncate">
                    <span class="min-w-0 flex-1 truncate">{{
                      draft.draftName ||
                      (draft.status === 2 ? "当前正文" : "可编辑草稿")
                    }}</span>
                    <CheckCircle2
                      v-if="draft.status === 2"
                      class="size-3.5 shrink-0 text-green-500"
                    />
                  </span>
                  <span class="mt-0.5 block truncate text-[11px] text-gray-400"
                    >{{ draft.wordCount }} 字 ·
                    {{ draft.updatedAt.toLocaleString() }}</span
                  >
                </button>
                <button
                  v-if="draft.status !== 2"
                  class="mt-0.5 shrink-0 rounded p-1 text-gray-400 transition hover:bg-red-50 hover:text-red-500 dark:text-gray-500 dark:hover:bg-red-950/30 dark:hover:text-red-400"
                  title="删除草稿"
                  @click.stop="requestDeleteEditorDraft(draft.id)"
                >
                  <Trash2 class="size-3.5" />
                </button>
              </div>
              <div
                v-if="store.currentEditorDrafts.length === 0"
                class="px-2 py-3 text-xs text-gray-400"
              >
                暂无可编辑草稿，请先在对话中点击“加入草稿”。
              </div>
            </div>
          </div>
          <button
            :class="editorToolButtonClass"
            :title="hasEditorContent ? '查找 Ctrl+F' : ''"
            :disabled="!hasEditorContent"
            @click="toggleFind"
          >
            <Search class="size-5" />
          </button>
          <button
            :class="editorToolButtonClass"
            :title="hasEditorContent ? '替换 Ctrl+H' : ''"
            :disabled="!hasEditorContent"
            @click="toggleReplace"
          >
            <Replace class="size-5" />
          </button>
          <button
            :class="editorToolButtonClass"
            :title="hasEditorContent ? '一键排版' : ''"
            :disabled="!hasEditorContent"
            @click="formatEditorContent"
          >
            <AlignLeft class="size-5" />
          </button>
          <button
            :class="editorToolButtonClass"
            :title="hasEditorContent ? '高频词统计' : ''"
            :disabled="!hasEditorContent"
            @click="showWordStats = !showWordStats"
          >
            <BarChart3 class="size-5" />
          </button>
          <div
            class="relative"
            @mouseenter="hasEditorContent && openAITools()"
            @mouseleave="hasEditorContent && closeAIToolsSoon()"
          >
            <button
              :class="editorToolButtonClass"
              :title="hasEditorContent ? 'AI工具' : ''"
              :disabled="!hasEditorContent || !store.currentEditorDraft"
              @click="showAITools = !showAITools"
            >
              <Loader2
                v-if="isHumanizing || isProofreading"
                class="size-5 animate-spin"
              />
              <Bot v-else class="size-5" />
            </button>
            <div
              v-if="showAITools && hasEditorContent"
              class="absolute left-12 top-0 z-30 w-32 rounded-lg border border-gray-200 bg-white p-1.5 shadow-xl dark:border-gray-700 dark:bg-gray-900"
              @mouseenter="openAITools"
              @mouseleave="closeAIToolsSoon"
            >
              <button
                class="flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-gray-300 dark:hover:bg-gray-800"
                title="改写当前正文以降低AI味，结果不会直接覆盖左侧文本"
                :disabled="isHumanizing || isProofreading"
                @click="runHumanize"
              >
                <Eraser class="size-4 text-gray-400" />
                <span>AI消痕</span>
              </button>
              <button
                class="flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-xs text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-gray-300 dark:hover:bg-gray-800"
                title="只检查错字、语法、标点和明显逻辑，适合手动修改后自查，不改文风和剧情"
                :disabled="isHumanizing || isProofreading"
                @click="runProofread"
              >
                <UserSearch class="size-4 text-gray-400" />
                <span>AI校审</span>
              </button>
            </div>
          </div>
        </aside>

        <section class="relative flex min-w-0 flex-1 flex-col">
          <div
            class="flex h-14 items-center justify-between border-b border-gray-200 px-4 dark:border-gray-800"
          >
            <div class="min-w-0">
              <div class="flex min-w-0 items-center gap-1">
                <input
                  v-if="isEditingDraftName"
                  ref="draftTitleInput"
                  v-model="editorDraftName"
                  class="min-w-0 max-w-80 bg-transparent text-sm font-semibold text-gray-900 outline-none dark:text-white"
                  @blur="saveDraftName"
                  @keydown.enter.prevent="saveDraftName"
                />
                <h1
                  v-else
                  class="truncate text-sm font-semibold text-gray-900 dark:text-white"
                >
                  {{ editorDraftName || editorTitle || "章节编辑" }}
                </h1>
                <button
                  v-if="store.currentEditorDraft"
                  class="shrink-0 rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                  title="编辑草稿名"
                  @click="startEditingDraftName"
                >
                  <Pencil class="size-3.5" />
                </button>
              </div>
              <p class="mt-0.5 text-xs text-gray-500">
                {{ wordCount }} 字 · {{ editorSaveLabel }}
                <template v-if="store.currentEditorDraft?.usedAt">
                  · 该草稿已应用正文
                </template>
              </p>
            </div>
            <div class="flex items-center gap-3">
              <button
                class="inline-flex items-center gap-2 rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
                :disabled="!store.currentEditorDraft"
                @click="store.applyEditorDraft()"
              >
                <CheckCircle2 class="size-4" />应用正文
              </button>
            </div>
          </div>

          <div
            v-if="showFindPanel"
            class="absolute right-4 top-16 z-20 w-80 rounded-lg border border-gray-200 bg-white p-3 text-xs shadow-xl dark:border-gray-700 dark:bg-gray-900"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="font-semibold text-gray-700 dark:text-gray-200">{{
                showReplacePanel ? "替换" : "查找"
              }}</span>
              <button
                class="rounded p-1 text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
                @click="
                  showFindPanel = false;
                  showReplacePanel = false;
                "
              >
                ×
              </button>
            </div>
            <div class="space-y-2">
              <div class="flex items-center gap-2">
                <input
                  ref="findInput"
                  v-model="findText"
                  class="min-w-0 flex-1 rounded border border-gray-200 bg-white px-2 py-1 outline-none dark:border-gray-700 dark:bg-gray-950"
                  placeholder="查找"
                  @keydown.enter="findNext"
                />
                <span class="shrink-0 text-gray-400"
                  >{{ activeFindIndex }}/{{ findMatches }}</span
                >
              </div>
              <div v-if="showReplacePanel" class="flex items-center gap-2">
                <input
                  v-model="replaceText"
                  class="min-w-0 flex-1 rounded border border-gray-200 bg-white px-2 py-1 outline-none dark:border-gray-700 dark:bg-gray-950"
                  placeholder="替换为"
                />
              </div>
            </div>
            <div class="mt-2 flex justify-end gap-1">
              <button
                class="rounded px-2 py-1 hover:bg-gray-100 dark:hover:bg-gray-800"
                @click="findPrevious"
              >
                上一个
              </button>
              <button
                class="rounded px-2 py-1 hover:bg-gray-100 dark:hover:bg-gray-800"
                @click="findNext"
              >
                下一个
              </button>
              <template v-if="showReplacePanel">
                <button
                  class="rounded px-2 py-1 hover:bg-gray-100 dark:hover:bg-gray-800"
                  @click="replaceCurrent"
                >
                  替换
                </button>
                <button
                  class="rounded px-2 py-1 hover:bg-gray-100 dark:hover:bg-gray-800"
                  @click="replaceAll"
                >
                  全部替换
                </button>
              </template>
            </div>
          </div>

          <div
            v-if="showWordStats"
            class="absolute right-4 top-16 z-20 w-80 rounded-lg border border-gray-200 bg-white p-3 shadow-xl dark:border-gray-700 dark:bg-gray-900"
          >
            <div class="mb-2 flex items-center justify-between">
              <span
                class="text-xs font-semibold text-gray-700 dark:text-gray-200"
                >高频词统计</span
              >
              <button
                class="rounded p-1 text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800"
                @click="showWordStats = false"
              >
                ×
              </button>
            </div>
            <p class="mb-2 text-[11px] text-gray-400">
              基于中文分词统计重复词；点击词语后用查找栏逐个定位。
            </p>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="item in frequentWords"
                :key="item[0]"
                class="rounded px-2 py-1 text-xs"
                :class="
                  highlightedWord === item[0]
                    ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/40 dark:text-yellow-200'
                    : 'bg-gray-50 text-gray-600 hover:bg-gray-100 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700'
                "
                @click="focusWord(item[0])"
              >
                {{ item[0] }} × {{ item[1] }}
              </button>
              <span
                v-if="frequentWords.length === 0"
                class="text-xs text-gray-400"
                >暂无高频词</span
              >
            </div>
          </div>

          <div
            v-if="store.currentEditorDraft"
            class="flex min-h-0 flex-1 overflow-hidden"
          >
            <div class="relative min-h-0 flex-1">
              <textarea
                ref="editorTextarea"
                v-model="editorContent"
                class="chapter-editor-textarea h-full w-full resize-none overflow-auto bg-white px-4 py-4 font-mono text-sm leading-7 text-gray-800 outline-none disabled:cursor-not-allowed disabled:opacity-80 dark:bg-gray-950 dark:text-gray-200"
                spellcheck="false"
                :disabled="isProofreading"
                @input="scheduleEditorSave"
                @scroll="handleEditorScroll"
                @keydown="handleEditorKeydown"
              />
              <div
                v-if="proofreadTextSegments.length > 0 && !isProofreading"
                class="pointer-events-none absolute inset-0 z-10 overflow-hidden"
              >
                <div
                  class="whitespace-pre-wrap px-4 py-4 font-mono text-sm leading-7 text-transparent"
                  :style="{
                    transform: `translateY(-${editorScrollTop}px)`,
                  }"
                >
                  <template
                    v-for="(segment, index) in proofreadTextSegments"
                    :key="`${index}-${segment.suggestionIndex ?? 'text'}`"
                  >
                    <span v-if="segment.suggestionIndex === null">{{
                      segment.text
                    }}</span>
                    <span
                      v-else
                      class="group pointer-events-auto relative rounded bg-red-50 text-red-600 underline decoration-red-300 underline-offset-2 dark:bg-red-950/40 dark:text-red-300 dark:decoration-red-700"
                    >
                      {{ segment.text }}
                      <span
                        class="absolute left-0 top-full z-40 mt-1 hidden w-72 rounded-lg border border-gray-200 bg-white p-3 text-xs leading-5 text-gray-600 shadow-xl group-hover:block dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300"
                      >
                        <span
                          class="block font-semibold text-gray-900 dark:text-gray-100"
                          >建议文本</span
                        >
                        <span class="mt-1 block">{{
                          proofreadSuggestions[segment.suggestionIndex]
                            ?.suggestedText
                        }}</span>
                        <span
                          class="mt-2 block font-semibold text-gray-900 dark:text-gray-100"
                          >修改原因</span
                        >
                        <span class="mt-1 block">{{
                          proofreadSuggestions[segment.suggestionIndex]?.reason
                        }}</span>
                        <span class="mt-3 flex justify-end gap-2">
                          <button
                            class="rounded border border-gray-200 px-2 py-1 text-gray-500 hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"
                            @mousedown.prevent
                            @click.stop="
                              ignoreProofreadSuggestion(segment.suggestionIndex)
                            "
                          >
                            忽略
                          </button>
                          <button
                            class="rounded bg-gray-900 px-2 py-1 text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
                            @mousedown.prevent
                            @click.stop="
                              applyProofreadSuggestion(segment.suggestionIndex)
                            "
                          >
                            应用
                          </button>
                        </span>
                      </span>
                    </span>
                  </template>
                </div>
              </div>
              <div
                v-if="isProofreading"
                class="absolute inset-0 z-30 overflow-hidden bg-gray-950/20 shadow-inner backdrop-blur-[2px] dark:bg-gray-950/60"
              >
                <div
                  class="proofread-scan-line absolute left-0 right-0 h-px bg-gray-700/60 shadow-[0_0_18px_rgba(75,85,99,0.55)] dark:bg-gray-100/70"
                />
                <div
                  class="absolute inset-0 flex flex-col items-center justify-center px-6 text-center"
                >
                  <div
                    class="rounded-xl border border-gray-200 bg-white px-6 py-5 shadow-2xl dark:border-gray-700 dark:bg-gray-900"
                  >
                    <Loader2
                      class="mx-auto size-7 animate-spin text-gray-700 dark:text-gray-200"
                    />
                    <p
                      class="mt-4 text-base font-semibold text-gray-900 dark:text-white"
                    >
                      AI 校审中
                    </p>
                    <p
                      class="mt-2 max-w-sm text-sm leading-6 text-gray-600 dark:text-gray-300"
                    >
                      只检查错字、语法、标点和明显逻辑。校审期间无法修改文本，完成后会在原文上标出建议位置。
                    </p>
                    <button
                      class="mt-4 rounded-lg border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-200 dark:hover:bg-gray-800"
                      @click="cancelProofread"
                    >
                      取消
                    </button>
                  </div>
                </div>
              </div>
            </div>
            <div
              v-if="isHumanizing || humanizedContent"
              class="group relative min-h-0 flex-1 border-l border-gray-200 dark:border-gray-800"
            >
              <div v-if="humanizedContent" class="absolute right-4 top-4 z-10">
                <button
                  class="rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                  @click="showHumanizeMenu = !showHumanizeMenu"
                >
                  <MoreHorizontal class="size-4" />
                </button>
                <div
                  v-if="showHumanizeMenu"
                  class="absolute right-0 top-full mt-1 w-32 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
                >
                  <button
                    v-if="humanizeReport"
                    class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    @click="
                      showHumanizeReport = true;
                      showHumanizeMenu = false;
                    "
                  >
                    消痕报告
                  </button>
                  <button
                    class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    @click="joinHumanizedAsDraft"
                  >
                    新加草稿
                  </button>
                  <button
                    class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    @click="
                      applyHumanizedContent();
                      showHumanizeMenu = false;
                    "
                  >
                    应用草稿
                  </button>
                </div>
              </div>
              <div
                v-if="isHumanizing"
                class="flex h-full flex-col items-center justify-center bg-gray-50 px-6 text-center dark:bg-gray-900"
              >
                <Loader2 class="size-6 animate-spin text-gray-500" />
                <p
                  class="mt-4 text-sm font-medium text-gray-800 dark:text-gray-100"
                >
                  AI 消痕处理中
                </p>
                <p
                  class="mt-2 max-w-xs text-xs leading-5 text-gray-500 dark:text-gray-400"
                >
                  请不要离开当前编辑区，等待任务完成后再继续编辑或切换页面。
                </p>
                <button
                  class="mt-3 rounded px-3 py-1.5 text-xs text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
                  @click="cancelHumanize"
                >
                  取消
                </button>
              </div>
              <div
                v-else
                class="h-full overflow-auto whitespace-pre-wrap bg-gray-50 px-4 py-4 font-mono text-sm leading-7 text-gray-800 dark:bg-gray-900 dark:text-gray-200"
              >
                {{ humanizedContent }}
              </div>
            </div>
          </div>
          <div
            v-else
            class="flex flex-1 flex-col items-center justify-center pb-20 text-center text-gray-400"
          >
            <img
              :src="emptyEditImage"
              alt=""
              class="h-40 w-60 object-contain opacity-85 dark:opacity-60 dark:contrast-90"
            />
            <p class="mt-3 text-sm">还没有可编辑草稿</p>
            <p class="mt-1 text-xs">
              <button
                type="button"
                class="font-medium text-gray-700 underline underline-offset-4 hover:text-gray-900 dark:text-gray-200 dark:hover:text-white"
                @click="store.switchToChatMode()"
              >
                回到对话
              </button>
              ，点击 AI 正文卡片上的“加入草稿”。
            </p>
          </div>
        </section>
        <div
          v-if="showHumanizeReport"
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/30 px-4"
          @click.self="showHumanizeReport = false"
        >
          <div
            class="flex max-h-[80vh] w-full max-w-3xl flex-col rounded-xl bg-white shadow-xl dark:bg-gray-900"
          >
            <div
              class="flex items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-gray-800"
            >
              <div>
                <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
                  AI 消痕报告
                </h3>
                <p class="mt-1 text-xs text-gray-500">
                  服务器不会保留该报告，如需保留，请自行
                  <button
                    class="font-medium text-gray-900 underline underline-offset-2 hover:text-gray-600 dark:text-gray-100 dark:hover:text-gray-300"
                    @click="downloadHumanizeReport"
                  >
                    下载
                  </button>
                </p>
              </div>
              <button
                class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                @click="showHumanizeReport = false"
              >
                <X class="size-4" />
              </button>
            </div>
            <div
              class="chat-markdown humanize-report min-h-0 flex-1 overflow-auto px-5 py-4 text-sm"
              v-html="renderMarkdown(humanizeReport)"
            />
          </div>
        </div>
        <BackToTopButton :target="editorContainer" />
      </div>
      <div v-else class="flex flex-1 items-center justify-center">
        <div class="text-center">
          <img
            :src="emptyEditImage"
            alt=""
            class="mx-auto h-44 w-64 object-contain opacity-85 dark:opacity-60 dark:contrast-90"
          />
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
            在卷列表中选择章节，点击「编辑」开始处理正文
          </p>
        </div>
      </div>
    </template>

    <!-- Draft Preview In Chat -->
    <div
      v-if="
        store.viewMode === 'chat' &&
        !store.isNovelSetupChoiceOpen &&
        !store.isNovelSetupOpen &&
        previewDraft
      "
      ref="previewContainer"
      class="flex flex-1 overflow-y-auto px-6 py-6"
    >
      <div class="mx-auto w-full max-w-3xl">
        <div class="mb-4 flex items-center justify-between gap-3">
          <button
            class="rounded-lg border border-gray-200 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
            @click="closeDraftPreview"
          >
            返回对话
          </button>
          <div class="flex items-center gap-2">
            <button
              class="rounded-lg border border-gray-200 p-2 text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
              title="上一篇"
              :disabled="!previousDraftMessage"
              @click="openAdjacentDraft(previousDraftMessage)"
            >
              <ArrowLeft class="size-4" />
            </button>
            <button
              class="rounded-lg border border-gray-200 p-2 text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
              title="下一篇"
              :disabled="!nextDraftMessage"
              @click="openAdjacentDraft(nextDraftMessage)"
            >
              <ArrowRight class="size-4" />
            </button>
            <button
              class="rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
              :disabled="
                !!previewMessageId &&
                store.savingDraftMessageId === previewMessageId
              "
              @click="joinPreviewDraft"
            >
              {{
                previewMessageId &&
                store.savingDraftMessageId === previewMessageId
                  ? "保存中..."
                  : "加入草稿"
              }}
            </button>
          </div>
        </div>
        <article class="rounded-lg bg-white p-6 shadow-sm dark:bg-gray-800">
          <div class="mb-5 border-b border-gray-200 pb-4 dark:border-gray-700">
            <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
              {{ previewDraft.title }}
            </h1>
            <div class="mt-2 flex items-center justify-between gap-3 text-xs">
              <p class="text-amber-600 dark:text-amber-300">临时正文预览</p>
              <span class="text-gray-500 dark:text-gray-400"
                >{{ previewWordCount }} 字</span
              >
            </div>
          </div>
          <div
            class="whitespace-pre-wrap text-sm leading-8 text-gray-700 dark:text-gray-300"
          >
            {{ previewDraft.content }}
          </div>
        </article>
      </div>
      <BackToTopButton :target="previewContainer" />
    </div>

    <!-- Chat Empty State -->
    <div
      v-else-if="
        store.viewMode === 'chat' &&
        !store.isNovelSetupChoiceOpen &&
        !store.isNovelSetupOpen &&
        store.isMessagesLoading &&
        store.activeMessages.length === 0
      "
      class="flex flex-1 items-center justify-center"
    >
      <div class="flex flex-col items-center text-gray-400 dark:text-gray-500">
        <p class="ai-thinking-placeholder text-sm" aria-label="正在加载对话">
          正在加载对话
        </p>
      </div>
    </div>

    <!-- Chat Messages -->
    <div
      v-if="
        store.viewMode === 'chat' &&
        !store.isNovelSetupChoiceOpen &&
        !store.isNovelSetupOpen &&
        !previewDraft &&
        store.activeMessages.length > 0
      "
      ref="messagesContainer"
      class="flex-1 overflow-y-auto px-3 pb-44 pt-4 sm:px-6 sm:pt-6"
      @scroll="handleMessagesScroll"
      @wheel.passive="handleMessagesWheel"
    >
      <div
        v-if="showQuestionNav"
        class="group fixed right-5 top-1/2 z-30 -translate-y-1/2"
        @wheel.prevent="
          messagesContainer?.scrollBy({ top: ($event as WheelEvent).deltaY })
        "
      >
        <div class="flex items-center justify-end">
          <div
            class="hidden max-h-40 w-48 overflow-y-auto rounded-lg border border-gray-200 bg-white p-1.5 shadow-lg group-hover:block dark:border-gray-700 dark:bg-gray-800"
            @wheel.stop
          >
            <button
              v-for="item in questionNavItems"
              :key="item.id"
              class="block w-full truncate rounded-md px-2 py-1.5 text-left text-xs transition-colors"
              :class="
                activeQuestionMessageId === item.id
                  ? 'bg-gray-100 text-gray-900 dark:bg-gray-700 dark:text-white'
                  : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700'
              "
              @click="scrollToMessage(item.id)"
            >
              {{ item.label }}
            </button>
          </div>
          <div class="h-44 w-3" />
          <div
            class="flex h-24 w-7 flex-col items-center justify-center gap-1 transition-transform group-hover:rotate-180"
            @wheel.prevent="
              messagesContainer?.scrollBy({
                top: ($event as WheelEvent).deltaY,
              })
            "
          >
            <span class="h-0.5 w-5 rounded bg-gray-400 dark:bg-gray-500" />
            <span class="h-0.5 w-4 rounded bg-gray-400 dark:bg-gray-500" />
            <span class="h-0.5 w-5 rounded bg-gray-400 dark:bg-gray-500" />
            <span class="h-0.5 w-3 rounded bg-gray-400 dark:bg-gray-500" />
          </div>
        </div>
      </div>
      <div class="mx-auto max-w-3xl space-y-6">
        <template v-for="message in store.activeMessages" :key="message.id">
          <!-- AI 消息：无头像 -->
          <div
            v-if="message.role === 'assistant'"
            class="flex flex-col items-start"
            :data-message-id="message.id"
          >
            <Transition name="chapter-progress-collapse">
              <div
                v-if="shouldShowChapterGeneration(message)"
                class="chapter-generation-progress"
                aria-label="高一致性生成进度"
              >
                <div class="space-y-2">
                  <div
                    v-for="(step, stepIndex) in chapterGenerationSteps(message)"
                    :key="`${message.id}-chapter-progress-${stepIndex}`"
                  >
                    <div
                      class="chapter-generation-step"
                      :class="
                        isActiveChapterGenerationStep(message, stepIndex)
                          ? 'text-gray-700 dark:text-gray-100'
                          : 'text-gray-400 dark:text-gray-500'
                      "
                    >
                      <span
                        class="chapter-generation-dot"
                        :class="
                          isActiveChapterGenerationStep(message, stepIndex)
                            ? 'chapter-generation-dot-active'
                            : ''
                        "
                      />
                      <span
                        :class="
                          isActiveChapterGenerationStep(message, stepIndex)
                            ? 'ai-thinking-placeholder'
                            : ''
                        "
                      >
                        {{ step }}
                      </span>
                      <span
                        v-if="isActiveChapterGenerationStep(message, stepIndex)"
                        class="shrink-0 text-xs tabular-nums text-gray-400"
                      >
                        {{ chapterGenerationElapsed(message) }}
                      </span>
                      <span
                        v-else-if="
                          chapterGenerationStepElapsed(message, step, stepIndex)
                        "
                        class="shrink-0 text-xs tabular-nums text-gray-400"
                      >
                        {{
                          chapterGenerationStepElapsed(message, step, stepIndex)
                        }}
                      </span>
                    </div>
                    <div
                      v-if="
                        chapterGenerationStepOutput(message, step, stepIndex)
                      "
                      class="mt-1 ml-4 max-w-2xl overflow-hidden rounded-md bg-gray-100/70 text-xs leading-5 text-gray-500 dark:bg-gray-800/70 dark:text-gray-300"
                    >
                      <button
                        v-if="
                          isChapterGenerationStepOutputCollapsed(
                            message,
                            step,
                            stepIndex
                          )
                        "
                        type="button"
                        class="flex w-full items-center justify-between gap-2 px-3 py-1.5 text-left text-[11px] font-medium text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
                        @click="
                          toggleChapterGenerationStepOutput(
                            message,
                            step,
                            stepIndex
                          )
                        "
                      >
                        <span>步骤输出已收起</span>
                        <ChevronDown class="size-3.5 shrink-0" />
                      </button>
                      <div
                        class="px-3 py-2 transition-all"
                        :class="
                          isChapterGenerationStepOutputCollapsed(
                            message,
                            step,
                            stepIndex
                          )
                            ? 'max-h-10 overflow-hidden opacity-70'
                            : 'max-h-56 overflow-y-auto'
                        "
                      >
                        <template
                          v-if="
                            chapterGenerationStepOutput(
                              message,
                              step,
                              stepIndex
                            )?.type === 'issues'
                          "
                        >
                          <div
                            v-for="item in chapterGenerationStepOutput(
                              message,
                              step,
                              stepIndex
                            )?.items || []"
                            :key="item"
                            class="py-0.5"
                          >
                            {{ item }}
                          </div>
                        </template>
                        <div v-else class="whitespace-pre-wrap">
                          {{
                            chapterGenerationStepOutput(
                              message,
                              step,
                              stepIndex
                            )?.text
                          }}
                        </div>
                      </div>
                      <button
                        v-if="
                          !isChapterGenerationStepOutputCollapsed(
                            message,
                            step,
                            stepIndex
                          )
                        "
                        type="button"
                        class="flex w-full items-center justify-end gap-1 px-3 pb-2 text-[11px] text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
                        @click="
                          toggleChapterGenerationStepOutput(
                            message,
                            step,
                            stepIndex
                          )
                        "
                      >
                        <span>收起</span>
                        <ChevronDown class="size-3.5 rotate-180" />
                      </button>
                    </div>
                  </div>
                </div>
                <div
                  v-if="chapterGenerationFailedHint(message)"
                  class="mt-2 ml-4 max-w-2xl rounded-md bg-red-50 px-3 py-2 text-xs leading-5 text-red-600 dark:bg-red-950/30 dark:text-red-300"
                >
                  {{ chapterGenerationFailedHint(message) }}
                </div>
              </div>
            </Transition>
            <div
              v-if="shouldShowAssistantBubble(message)"
              class="max-w-full rounded-2xl bg-white px-4 py-3 text-sm text-gray-700 shadow-sm dark:bg-gray-800 dark:text-gray-100"
              :class="message.planOptions ? 'w-full' : 'inline-block'"
            >
              <div
                v-if="shouldShowAssistantContent(message)"
                class="chat-markdown"
                v-html="renderMarkdown(message.content)"
              />
              <div
                v-if="message.chapterDraft"
                class="mt-4 overflow-hidden rounded-2xl border border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-900"
              >
                <div
                  class="border-b border-gray-200 px-4 py-3 dark:border-gray-700"
                >
                  <div class="flex items-center justify-between gap-3">
                    <div>
                      <div
                        class="text-sm font-medium text-gray-900 dark:text-white"
                      >
                        {{ message.chapterDraft.title }}
                      </div>
                      <div
                        class="mt-1 text-xs text-gray-500 dark:text-gray-300"
                      >
                        实际
                        {{
                          message.chapterDraft.content.replace(/\s/g, "").length
                        }}
                        字
                      </div>
                    </div>
                  </div>
                  <p
                    v-if="message.chapterDraft.revisionNotes"
                    class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-300"
                  >
                    {{ message.chapterDraft.revisionNotes }}
                  </p>
                  <div class="mt-3 flex items-center justify-between">
                    <span
                      class="rounded bg-amber-100 px-2 py-1 text-xs font-medium text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
                      >临时正文</span
                    >
                    <span
                      v-if="!isDraftReady(message)"
                      class="text-xs text-gray-500 dark:text-gray-300"
                    >
                      正文生成中...
                    </span>
                    <div v-else class="flex items-center gap-2">
                      <button
                        class="rounded-md border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-100 dark:border-gray-700 dark:text-gray-100 dark:hover:bg-gray-800"
                        @click="openDraftPreview(message)"
                      >
                        预览阅读
                      </button>
                      <button
                        class="rounded-md bg-gray-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
                        :disabled="
                          !message.chapterDraft.draftId ||
                          store.savingDraftMessageId === message.id
                        "
                        @click="store.joinChapterDraft(message.id)"
                      >
                        {{
                          store.savingDraftMessageId === message.id
                            ? "保存中..."
                            : "加入草稿"
                        }}
                      </button>
                    </div>
                  </div>
                </div>
                <div
                  ref="streamingDraftContainer"
                  class="max-h-96 overflow-y-auto whitespace-pre-wrap px-4 py-3 text-sm leading-7 text-gray-700 dark:text-gray-100"
                  @wheel.passive="handleStreamingDraftWheel"
                >
                  {{ message.chapterDraft.content }}
                </div>
              </div>
              <div
                v-if="message.planOptions"
                class="overflow-hidden rounded-2xl border border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-900"
                :class="
                  shouldShowAssistantContent(message) || message.chapterDraft
                    ? 'mt-4'
                    : ''
                "
              >
                <div
                  v-if="shouldShowPlanApplyHeader(message)"
                  class="flex items-center justify-between gap-3 border-b border-gray-200 bg-white/70 px-4 py-3 dark:border-gray-700 dark:bg-gray-950/30"
                >
                  <span
                    class="text-sm font-medium text-gray-900 dark:text-white"
                  >
                    {{ planApplyHeaderTitle(message) }}
                  </span>
                  <Loader2
                    v-if="message.temporary"
                    class="size-4 shrink-0 animate-spin text-gray-400"
                  />
                  <button
                    v-else
                    class="inline-flex items-center gap-1.5 rounded-md bg-gray-900 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-gray-800 disabled:cursor-not-allowed disabled:bg-gray-300 disabled:text-gray-500 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200 dark:disabled:bg-gray-700 dark:disabled:text-gray-500"
                    :disabled="isPlanApplyButtonDisabled(message)"
                    @click="handleApplyPlan(message)"
                  >
                    <Loader2
                      v-if="store.applyingPlanMessageId === message.id"
                      class="size-3.5 animate-spin"
                    />
                    {{ planApplyButtonText(message) }}
                  </button>
                </div>
                <div class="space-y-3 p-3">
                  <template
                    v-for="(option, optionIndex) in message.planOptions"
                    :key="option.id"
                  >
                    <div
                      v-if="
                        option.custom &&
                        activeCustomOption === `${message.id}:${option.id}`
                      "
                      class="flex items-center gap-2 rounded-lg border border-gray-300 bg-white px-4 py-3 dark:border-gray-600 dark:bg-gray-950/60"
                    >
                      <div
                        class="flex size-6 shrink-0 items-center justify-center rounded border border-gray-300 dark:border-gray-600"
                      >
                        <div
                          class="size-3 rounded-sm bg-gray-300 dark:bg-gray-600"
                        />
                      </div>
                      <input
                        v-model="customInputText"
                        class="flex-1 bg-transparent text-sm text-gray-700 outline-none placeholder:text-gray-400 dark:text-gray-100 dark:placeholder:text-gray-400"
                        placeholder="输入你的想法，按 Enter 发送..."
                        @keydown.enter="handleCustomSend()"
                      />
                    </div>
                    <div
                      v-else
                      class="overflow-hidden rounded-lg border border-gray-200 bg-white transition-colors hover:border-gray-300 hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-950/60 dark:hover:border-gray-600 dark:hover:bg-gray-800"
                      :class="option.custom ? 'cursor-pointer' : ''"
                      @click="option.custom ? handleUsePlan(option) : undefined"
                    >
                      <div
                        class="flex w-full items-start justify-between gap-3 px-4 py-3 text-left"
                        :class="
                          option.optionType === 'volume' ||
                          option.optionType === 'chapter'
                            ? 'cursor-default'
                            : 'cursor-pointer'
                        "
                        @click.stop="
                          option.custom
                            ? handleUsePlan(option)
                            : togglePlanDetails(message.id, option)
                        "
                      >
                        <div class="min-w-0 flex-1">
                          <div class="flex items-center gap-2">
                            <div
                              class="flex size-6 items-center justify-center rounded border border-gray-300 dark:border-gray-600"
                            >
                              <div
                                class="size-3 rounded-sm bg-gray-300 dark:bg-gray-600"
                              />
                            </div>
                            <span
                              class="font-medium text-gray-900 dark:text-white"
                              >{{ planDisplayTitle(option, optionIndex) }}</span
                            >
                          </div>
                          <div
                            v-if="
                              option.description &&
                              option.optionType === 'chapter'
                            "
                            class="chat-markdown ml-8 mt-1 text-xs leading-5 text-gray-500 dark:text-gray-300"
                            v-html="renderMarkdown(option.description)"
                          />
                          <p
                            v-else-if="option.description"
                            class="ml-8 mt-1 text-xs leading-5 text-gray-500 dark:text-gray-300"
                          >
                            {{ option.description }}
                          </p>
                        </div>
                        <button
                          v-if="isSelectablePlan(option)"
                          class="shrink-0 rounded-md bg-gray-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
                          @click.stop="handleUsePlan(option)"
                        >
                          使用
                        </button>
                        <ChevronDown
                          v-if="planDetails(option).length > 0"
                          class="mt-0.5 size-5 shrink-0 text-gray-400 transition-transform"
                          :class="
                            expandedPlanDetailKey ===
                            planDetailKey(message.id, option.id)
                              ? 'rotate-180'
                              : ''
                          "
                        />
                      </div>
                      <div
                        v-if="
                          expandedPlanDetailKey ===
                            planDetailKey(message.id, option.id) &&
                          planDetails(option).length > 0
                        "
                        class="border-t border-gray-200 bg-white/70 px-4 py-3 dark:border-gray-700 dark:bg-gray-950/40"
                      >
                        <div class="space-y-2">
                          <div
                            v-for="detail in normalPlanDetails(option)"
                            :key="detail.key"
                            class="flex gap-3 rounded-md bg-gray-50 px-3 py-2 dark:bg-gray-900"
                          >
                            <div
                              class="w-20 shrink-0 pt-0.5 text-[11px] font-semibold text-gray-500 dark:text-gray-300"
                            >
                              {{ detail.label }}
                            </div>
                            <div
                              class="min-w-0 flex-1 whitespace-pre-wrap text-xs leading-5 text-gray-600 dark:text-gray-100"
                            >
                              <div
                                v-if="isCompactPlanDetail(detail)"
                                class="flex flex-wrap gap-1.5"
                              >
                                <span
                                  v-for="item in planDetailItems(detail.value)"
                                  :key="`${detail.key}-${item}`"
                                  class="inline-flex max-w-full rounded-md bg-white px-2 py-1 text-xs leading-4 text-gray-600 dark:bg-gray-950/60 dark:text-gray-100"
                                >
                                  {{ item }}
                                </span>
                              </div>
                              <div
                                v-else-if="Array.isArray(detail.value)"
                                class="space-y-0.5"
                              >
                                <div
                                  v-for="(item, itemIndex) in planDetailItems(
                                    detail.value
                                  )"
                                  :key="`${detail.key}-${itemIndex}`"
                                  class="flex gap-2"
                                >
                                  <span class="text-gray-400"
                                    >{{ itemIndex + 1 }}.</span
                                  >
                                  <span class="min-w-0 flex-1">
                                    <span
                                      class="chat-markdown plan-detail-markdown"
                                      v-html="renderMarkdown(item)"
                                    />
                                  </span>
                                </div>
                              </div>
                              <div
                                v-else-if="isMarkdownPlanDetail(detail)"
                                class="chat-markdown"
                                v-html="renderMarkdown(String(detail.value))"
                              />
                              <template v-else>{{ detail.value }}</template>
                            </div>
                          </div>
                          <div
                            v-if="planCharacterSettings(option).length > 0"
                            class="rounded-md bg-gray-50 px-3 py-2 dark:bg-gray-900"
                          >
                            <div
                              class="mb-2 text-[11px] font-semibold text-gray-500 dark:text-gray-300"
                            >
                              重点人物
                            </div>
                            <div class="space-y-2">
                              <div
                                v-for="(
                                  character, characterIndex
                                ) in planCharacterSettings(option)"
                                :key="`${option.id}-character-${characterIndex}`"
                                class="flex gap-2 rounded-md border border-gray-200 bg-white px-2.5 py-2 dark:border-gray-700 dark:bg-gray-950/60"
                              >
                                <span
                                  class="flex size-5 shrink-0 items-center justify-center rounded bg-gray-100 text-[10px] font-semibold text-gray-500 dark:bg-gray-800 dark:text-gray-300"
                                >
                                  {{ characterIndex + 1 }}
                                </span>
                                <span
                                  class="min-w-0 flex-1 whitespace-pre-wrap text-xs leading-5 text-gray-600 dark:text-gray-100"
                                >
                                  {{ character }}
                                </span>
                              </div>
                            </div>
                          </div>
                          <div
                            v-if="planTemporarySetupSections(option).length > 0"
                            class="rounded-md bg-gray-50 px-3 py-2 dark:bg-gray-900"
                          >
                            <div
                              class="mb-2 flex items-center gap-1.5 text-[11px] font-semibold text-gray-500 dark:text-gray-300"
                            >
                              临时设定
                              <Info
                                class="size-3 cursor-help text-gray-400"
                                title="临时设定只服务当前卷，正文可使用这些内容，但不能在正文阶段继续新增重要设定。"
                              />
                            </div>
                            <div class="space-y-2">
                              <div
                                v-for="section in planTemporarySetupSections(
                                  option
                                )"
                                :key="`${option.id}-temporary-${section.key}`"
                                class="rounded-md border border-gray-200 bg-white px-2.5 py-2 dark:border-gray-700 dark:bg-gray-950/60"
                              >
                                <div
                                  class="mb-1 text-[11px] font-semibold text-gray-500 dark:text-gray-300"
                                >
                                  {{ section.label }}
                                </div>
                                <div class="space-y-1">
                                  <div
                                    v-for="(item, itemIndex) in section.items"
                                    :key="`${section.key}-${itemIndex}`"
                                    class="flex gap-2 text-xs leading-5 text-gray-600 dark:text-gray-100"
                                  >
                                    <span class="text-gray-400"
                                      >{{ itemIndex + 1 }}.</span
                                    >
                                    <span class="min-w-0 flex-1">
                                      <span
                                        class="chat-markdown plan-detail-markdown"
                                        v-html="renderMarkdown(item)"
                                      />
                                    </span>
                                  </div>
                                </div>
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  </template>
                  <div
                    v-if="shouldShowPlanOptionsPlaceholder(message)"
                    class="overflow-hidden rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 dark:border-gray-700 dark:bg-gray-900"
                  >
                    <div class="flex items-start gap-3">
                      <Loader2
                        class="mt-0.5 size-4 shrink-0 animate-spin text-gray-400"
                      />
                      <div class="min-w-0 flex-1">
                        <div class="space-y-2">
                          <div
                            class="h-2.5 w-5/6 rounded bg-gray-200 dark:bg-gray-700"
                          />
                          <div
                            class="h-2.5 w-2/3 rounded bg-gray-200 dark:bg-gray-700"
                          />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <div
              v-if="shouldShowThinkingStatus(message)"
              class="mt-2 flex w-fit items-center gap-2 pl-1 text-xs"
              aria-label="AI 正在思考"
            >
              <span class="ai-thinking-placeholder">正在思考</span>
              <span class="tabular-nums text-gray-400">{{
                thinkingElapsed(message)
              }}</span>
            </div>
            <div
              v-if="shouldShowMessageTime(message)"
              class="mt-1 text-xs text-gray-400"
            >
              {{ formatTime(message.timestamp) }}
            </div>
          </div>
          <!-- 用户消息：无头像 -->
          <div v-else class="flex justify-end" :data-message-id="message.id">
            <div class="flex flex-col items-end">
              <div
                class="inline-block max-w-full rounded-2xl bg-gray-200 px-4 py-3 text-sm text-gray-800 dark:bg-gray-700 dark:text-gray-50"
              >
                <div
                  class="chat-markdown"
                  v-html="renderMarkdown(message.content)"
                />
              </div>
              <div class="mt-1 text-xs text-gray-400">
                {{ formatTime(message.timestamp) }}
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- Chat Input -->
    <div
      v-if="
        store.viewMode === 'chat' &&
        !store.isNovelSetupChoiceOpen &&
        !store.isNovelSetupOpen &&
        !previewDraft &&
        store.activeMessages.length > 0
      "
      class="pointer-events-none absolute inset-x-0 bottom-4 px-6"
    >
      <div class="pointer-events-auto mx-auto max-w-3xl">
        <div
          v-if="shouldShowScrollToLatestReply"
          class="mb-2 flex justify-center"
        >
          <button
            class="flex size-9 items-center justify-center rounded-full border border-gray-200 bg-white text-gray-600 shadow-lg shadow-black/10 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:hover:bg-gray-700"
            title="回到最新回复"
            @click="scrollToLatestAIReply"
          >
            <ChevronDown class="size-5" />
          </button>
        </div>
        <div v-if="quickPrompts.length > 0" class="mb-2 flex flex-wrap gap-2">
          <button
            v-if="store.selectedChapterId"
            class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs transition-colors"
            :class="
              chapterGraphMode
                ? 'border-gray-900 bg-gray-900 text-white hover:bg-gray-800 dark:border-gray-100 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200'
                : 'border-gray-200 bg-white text-gray-600 hover:border-gray-300 hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:hover:border-gray-600 dark:hover:bg-gray-700'
            "
            :title="
              chapterGraphMode
                ? '当前为高一致性模式，会生成后校验'
                : '当前为快速模式，直接生成正文'
            "
            @click="toggleChapterGraphMode"
          >
            <ShieldCheck class="size-3.5" />
            高一致性
          </button>
          <button
            v-for="prompt in quickPrompts"
            :key="prompt"
            class="rounded-full border border-gray-200 bg-white px-3 py-1.5 text-xs text-gray-600 transition-colors hover:border-gray-300 hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100 dark:hover:border-gray-600 dark:hover:bg-gray-700"
            @click="applyQuickPrompt(prompt)"
          >
            {{ prompt }}
          </button>
        </div>
        <div
          class="flex items-center gap-2 rounded-2xl border border-gray-200 bg-white p-2 dark:border-gray-700 dark:bg-gray-800"
        >
          <textarea
            ref="inputRef"
            v-model="inputText"
            :placeholder="chatInputPlaceholder"
            rows="1"
            class="max-h-[180px] min-h-10 flex-1 resize-none overflow-y-auto bg-transparent px-2 py-2 text-sm text-gray-700 placeholder-gray-400 outline-none dark:text-gray-100"
            @input="resizeInput"
            @keydown="handleKeydown"
          />
          <div class="flex shrink-0 items-center gap-2">
            <button
              class="flex items-center justify-center rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 disabled:cursor-not-allowed disabled:text-gray-300 dark:text-gray-300 dark:hover:bg-gray-700 dark:disabled:text-gray-600"
              :title="isVoiceRecognizing ? '取消语音识别' : '语音识别'"
              @click="startVoiceRecognition"
            >
              <X v-if="isVoiceRecognizing" class="size-4 animate-pulse" />
              <Mic v-else class="size-4" />
            </button>
            <button
              class="flex items-center justify-center rounded-lg bg-gray-900 p-2 text-white transition-colors hover:bg-gray-800 disabled:cursor-not-allowed disabled:bg-gray-300 disabled:text-gray-500 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200 dark:disabled:bg-gray-700 dark:disabled:text-gray-500"
              :disabled="store.activeStream ? false : !canSubmitInput"
              :title="
                isVoiceRecognizing
                  ? '确认识别文本'
                  : store.activeStream
                  ? '停止回复'
                  : '发送'
              "
              @click="
                isVoiceRecognizing
                  ? confirmVoiceRecognition()
                  : store.activeStream
                  ? store.stopActiveStream()
                  : handleSend()
              "
            >
              <Square v-if="store.activeStream" class="size-4 fill-current" />
              <Check v-else-if="isVoiceRecognizing" class="size-4" />
              <ArrowUp v-else class="size-4" />
            </button>
          </div>
          <span v-if="isVoiceRecognizing && voiceTranscript" class="sr-only">{{
            voiceTranscript
          }}</span>
        </div>
      </div>
    </div>
    <div
      v-if="planApplyConfirm"
      class="fixed inset-0 z-[70] flex items-center justify-center bg-black/30 px-4"
      @click.self="closePlanApplyConfirm"
    >
      <div
        class="w-full max-w-sm rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900"
      >
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">
          覆盖规划
        </h3>
        <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-gray-300">
          {{ planApplyConfirm.message }}
        </p>
        <div class="mt-5 flex justify-end gap-2">
          <button
            class="rounded-lg border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
            @click="closePlanApplyConfirm"
          >
            取消
          </button>
          <button
            class="rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:bg-gray-300 disabled:text-gray-500 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200 dark:disabled:bg-gray-700 dark:disabled:text-gray-400"
            :disabled="planApplyConfirmRemaining > 0"
            @click="confirmApplyPlan"
          >
            {{
              planApplyConfirmRemaining > 0
                ? `确认覆盖（${planApplyConfirmRemaining}s）`
                : "确认覆盖"
            }}
          </button>
        </div>
      </div>
    </div>
    <div
      v-if="confirmDialog"
      class="fixed inset-0 z-[70] flex items-center justify-center bg-black/30 px-4"
      @click.self="closeConfirmDialog"
    >
      <div
        class="w-full max-w-sm rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900"
      >
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">
          确认删除
        </h3>
        <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-gray-300">
          {{ confirmDialog.message }}
        </p>
        <div class="mt-5 flex justify-end gap-2">
          <button
            class="rounded-lg border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
            @click="closeConfirmDialog"
          >
            取消
          </button>
          <button
            class="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700"
            @click="runConfirmDialogAction"
          >
            删除
          </button>
        </div>
      </div>
    </div>
    <!-- 未保存离开提示 -->
    <div
      v-if="store.isUnsavedSetupLeaveOpen"
      class="fixed inset-0 z-[80] flex items-center justify-center bg-black/30 px-4"
      @click.self="store.closeUnsavedSetupLeave()"
    >
      <div
        class="w-full max-w-sm rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900"
      >
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">
          {{ store.novelSetupGenerating ? "模板正在生成" : "未保存的更改" }}
        </h3>
        <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-gray-300">
          {{
            store.novelSetupGenerating
              ? "当前正在生成具体模板，请等待生成完成，或在生成弹窗中先取消生成。"
              : "当前新建小说数据尚未保存，离开后将丢失所有未保存的内容。"
          }}
        </p>
        <div v-if="store.novelSetupGenerating" class="mt-5 flex justify-end">
          <button
            class="rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
            @click="store.closeUnsavedSetupLeave()"
          >
            知道了
          </button>
        </div>
        <div v-else class="mt-5 flex justify-end gap-2">
          <button
            class="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700"
            @click="store.confirmLeaveWithoutSaving()"
          >
            离开
          </button>
          <button
            class="rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
            @click="handleSaveAndLeave"
          >
            保存
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-markdown {
  overflow-wrap: anywhere;
  line-height: 1.7;
  color: rgb(31 41 55);
}

.humanize-report {
  color: rgb(31 41 55);
}

.typing-cursor {
  width: 0.12rem;
  height: 1.1rem;
  border-radius: 999px;
  background: currentColor;
  animation: typing-caret 0.85s steps(1, end) infinite;
}

.ai-thinking-placeholder {
  position: relative;
  display: inline-block;
  overflow: hidden;
  color: rgb(107 114 128);
  font-weight: 500;
  letter-spacing: 0;
}

.ai-thinking-placeholder::after {
  content: "";
  position: absolute;
  inset: 0;
  transform: translateX(-120%);
  background: linear-gradient(
    90deg,
    transparent,
    rgba(255, 255, 255, 0.92),
    transparent
  );
  animation: thinking-sweep 1.45s ease-in-out infinite;
  mix-blend-mode: screen;
}

.chapter-generation-progress {
  margin-bottom: 0.5rem;
  max-width: 36rem;
  padding: 0.25rem 0.125rem;
  font-size: 0.8125rem;
  line-height: 1.5;
}

.chapter-generation-step {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  transition: color 0.2s ease;
}

.chapter-generation-dot {
  width: 0.375rem;
  height: 0.375rem;
  flex-shrink: 0;
  border-radius: 999px;
  background: currentColor;
  opacity: 0.45;
}

.chapter-generation-dot-active {
  opacity: 1;
  animation: chapter-progress-pulse 1.15s ease-in-out infinite;
}

.chapter-progress-collapse-enter-active,
.chapter-progress-collapse-leave-active {
  max-height: 18rem;
  overflow: hidden;
  transition: opacity 0.24s ease, transform 0.24s ease, max-height 0.28s ease;
}

.chapter-progress-collapse-enter-from,
.chapter-progress-collapse-leave-to {
  max-height: 0;
  opacity: 0;
  transform: translateY(-0.25rem);
}

.chapter-progress-collapse-enter-to,
.chapter-progress-collapse-leave-from {
  max-height: 18rem;
  opacity: 1;
  transform: translateY(0);
}

.proofread-scan-line {
  animation: proofread-scan 1.8s ease-in-out infinite alternate;
}

.setup-thinking-slide-enter-active,
.setup-thinking-slide-leave-active {
  transition: opacity 0.22s ease, transform 0.22s ease;
}

.setup-thinking-slide-enter-from {
  opacity: 0;
  transform: translateY(0.5rem);
}

.setup-thinking-slide-leave-to {
  opacity: 0;
  transform: translateY(-0.5rem);
}

@keyframes typing-caret {
  0%,
  45% {
    opacity: 1;
  }
  46%,
  100% {
    opacity: 0;
  }
}

@keyframes proofread-scan {
  from {
    top: 0;
    opacity: 0.45;
  }
  to {
    top: calc(100% - 1px);
    opacity: 1;
  }
}

@keyframes thinking-sweep {
  0% {
    transform: translateX(-120%);
  }
  65%,
  100% {
    transform: translateX(120%);
  }
}

@keyframes chapter-progress-pulse {
  0%,
  100% {
    transform: scale(1);
    opacity: 0.65;
  }
  50% {
    transform: scale(1.45);
    opacity: 1;
  }
}

.chat-markdown :deep(p) {
  margin: 0.4rem 0;
}

.chat-markdown :deep(p:first-child) {
  margin-top: 0;
}

.chat-markdown :deep(p:last-child) {
  margin-bottom: 0;
}

.chat-markdown.plan-detail-markdown {
  display: inline;
  line-height: 1.45;
}

.chat-markdown.plan-detail-markdown :deep(p) {
  display: inline;
  margin: 0;
}

.chat-markdown.plan-detail-markdown :deep(ul),
.chat-markdown.plan-detail-markdown :deep(ol) {
  display: inline;
  margin: 0.15rem 0;
  padding-left: 0;
  list-style: none;
}

.chat-markdown.plan-detail-markdown :deep(li) {
  display: inline;
  margin: 0.1rem 0;
}

.chat-markdown.plan-detail-markdown :deep(li + li)::before {
  content: "、";
}

.chat-markdown :deep(ul),
.chat-markdown :deep(ol) {
  margin: 0.5rem 0;
  padding-left: 1.35rem;
}

.chat-markdown :deep(ul) {
  list-style: disc;
}

.chat-markdown :deep(ol) {
  list-style: decimal;
}

.chat-markdown :deep(li) {
  margin: 0.25rem 0;
  padding-left: 0.1rem;
}

.chat-markdown :deep(li > p) {
  margin: 0.15rem 0;
}

.chat-markdown :deep(strong) {
  font-weight: 650;
  color: inherit;
}

.chat-markdown :deep(a) {
  color: rgb(37 99 235);
  text-decoration: underline;
  text-underline-offset: 2px;
}

.chat-markdown :deep(code) {
  border-radius: 0.25rem;
  background: rgb(243 244 246);
  padding: 0.1rem 0.25rem;
  font-size: 0.85em;
}

.chat-markdown :deep(pre) {
  margin: 0.6rem 0;
  overflow-x: auto;
  border-radius: 0.5rem;
  background: rgb(17 24 39);
  padding: 0.75rem;
  color: rgb(243 244 246);
}

.chat-markdown :deep(pre code) {
  background: transparent;
  padding: 0;
  color: inherit;
}

.chat-markdown :deep(blockquote) {
  margin: 0.6rem 0;
  border-left: 3px solid rgb(209 213 219);
  padding-left: 0.75rem;
  color: rgb(75 85 99);
}

.chat-markdown :deep(h1),
.chat-markdown :deep(h2),
.chat-markdown :deep(h3) {
  margin: 0.7rem 0 0.35rem;
  font-weight: 650;
  line-height: 1.35;
  color: inherit;
}

.chat-markdown :deep(h1) {
  font-size: 1.1rem;
}

.chat-markdown :deep(h2),
.chat-markdown :deep(h3) {
  font-size: 1rem;
}

:global(.dark .chat-markdown) {
  color: rgb(229 231 235);
}

:global(.dark .chat-markdown p),
:global(.dark .chat-markdown li),
:global(.dark .chat-markdown td),
:global(.dark .chat-markdown span) {
  color: rgb(229 231 235);
}

:global(.dark .chat-markdown a) {
  color: rgb(147 197 253);
}

:global(.dark .humanize-report),
:global(.dark .humanize-report *) {
  color: rgb(229 231 235);
}

:global(.dark .humanize-report h1),
:global(.dark .humanize-report h2),
:global(.dark .humanize-report h3),
:global(.dark .humanize-report strong),
:global(.dark .humanize-report th) {
  color: rgb(249 250 251);
}

:global(.dark .chat-markdown code) {
  background: rgb(55 65 81);
  color: rgb(243 244 246);
}

:global(.dark .chat-markdown pre) {
  background: rgb(3 7 18);
  color: rgb(243 244 246);
}

:global(.dark .chat-markdown pre code) {
  color: inherit;
}

:global(.dark .chat-markdown blockquote) {
  border-left-color: rgb(75 85 99);
  color: rgb(229 231 235);
}

:global(.dark .chat-markdown h1),
:global(.dark .chat-markdown h2),
:global(.dark .chat-markdown h3),
:global(.dark .chat-markdown strong) {
  color: rgb(249 250 251);
}

.chat-markdown :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin: 0.5em 0 1em;
  font-size: 0.875rem;
}

.chat-markdown :deep(th),
.chat-markdown :deep(td) {
  border: 1px solid rgb(209 213 219);
  padding: 0.5rem 0.75rem;
  text-align: left;
}

.chat-markdown :deep(th) {
  background: rgb(243 244 246);
  font-weight: 600;
}

.chat-markdown :deep(tr:nth-child(even) td) {
  background: rgb(249 250 251);
}

:global(.dark .chat-markdown th) {
  background: rgb(55 65 81);
  border-color: rgb(75 85 99);
  color: rgb(249 250 251);
}

:global(.dark .chat-markdown td) {
  border-color: rgb(75 85 99);
  color: rgb(229 231 235);
}

:global(.dark .chat-markdown tr:nth-child(even) td) {
  background: rgb(17 24 39);
}

:global(.dark .chat-markdown tr:nth-child(odd) td) {
  background: rgb(31 41 55);
}
</style>
