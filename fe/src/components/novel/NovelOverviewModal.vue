<script setup lang="ts">
import { ChevronRight, MapPinned, Network, Puzzle, Target, Users, X } from "lucide-vue-next";
import { computed, ref, watch } from "vue";
import { useAppStore } from "@/stores/app";
import CharacterRelationGraph from "@/components/common/CharacterRelationGraph.vue";
import { overviewDetails, overviewSummary } from "@/utils/overview";

const store = useAppStore();
const selectedSetupItem = ref<SetupListItem | null>(null);
const selectedSetupSection = ref<SetupListSection | null>(null);
const overviewCharacterGraphOpen = ref(false);
const selectedOverviewGraphNodeId = ref<string | null>(null);
const selectedOverviewGraphEdgeId = ref<string | null>(null);
const overviewGraphNodePositions = ref<
  Record<string, { x: number; y: number }>
>({});

const novel = computed(() => {
  if (!store.overviewNovelId) return null;
  return (
    store.novelOverviews[store.overviewNovelId] ||
    [...store.novels, ...store.archivedNovels].find(
      (n) => n.id === store.overviewNovelId
    ) ||
    null
  );
});

const totalWords = computed(() => {
  return novel.value?.wordCount || 0;
});

const createdAtLabel = computed(() => {
  const current = store.overviewNovelId
    ? [...store.novels, ...store.archivedNovels].find(
        (n) => n.id === store.overviewNovelId
      )
    : null;
  return current?.createdAt
    ? current.createdAt.toLocaleString("zh-CN")
    : "未知";
});

const updatedAtLabel = computed(() => {
  return novel.value?.updatedAt
    ? novel.value.updatedAt.toLocaleString("zh-CN")
    : "未知";
});

const planSummary = computed(() => {
  const plan = novel.value?.planData as Record<string, unknown> | undefined;
  return typeof plan?.summary === "string" ? plan.summary.trim() : "";
});

const setupDetails = computed(() => {
  const plan = novel.value?.planData as Record<string, unknown> | undefined;
  if (!plan) return [];
  const details: Array<{ key: string; label: string; value: string | string[] }> = [];
  pushDetail(details, "perspective", "叙事视角", plan.perspective);
  pushDetail(
    details,
    "length",
    "篇幅",
    [plan.length, plan.length_range].filter(Boolean).join(" ")
  );
  const tagGroups = normalizeTagGroups(plan.tag_groups);
  for (const [group, tags] of Object.entries(tagGroups)) {
    if (tags.length > 0)
      details.push({
        key: `tag-${group}`,
        label: group,
        value: tags,
      });
  }
  return details;
});

const setupListDetails = computed(() => {
  const plan = novel.value?.planData as Record<string, unknown> | undefined;
  if (!plan) return [];
  return [
    namedSection("characters", "人物设定", plan.characters),
    namedSection("maps", "地点设定", plan.maps),
    namedSection("forces", "势力设定", plan.forces),
    otherSettingsSection(plan.other_settings),
  ].filter(
    (section): section is SetupListSection =>
      !!section && section.items.length > 0
  );
});

const overviewGraphNodes = computed(() => {
  const plan = novel.value?.planData as Record<string, unknown> | undefined;
  const charArray = setupArray(plan?.characters);
  return charArray.map((item, index) => {
    const id = `char_${String(index + 1).padStart(3, "0")}`;
    const name = setupString(item.name) || `人物${index + 1}`;
    const custom = overviewGraphNodePositions.value[id];
    const appearanceTime = setupString(item.appearance_time);
    const notes = setupString(item.notes);
    if (custom) return { id, name, appearanceTime, notes, x: custom.x, y: custom.y };
    return { id, name, appearanceTime, notes };
  });
});

const selectedOverviewGraphNode = computed(() =>
  overviewGraphNodes.value.find((node) => node.id === selectedOverviewGraphNodeId.value)
);

const selectedOverviewGraphEdge = computed(() =>
  overviewGraphEdges.value.find((edge) => edge.id === selectedOverviewGraphEdgeId.value)
);

const overviewGraphEdges = computed(() => {
  const plan = novel.value?.planData as Record<string, unknown> | undefined;
  const nodes = new Map(overviewGraphNodes.value.map((n) => [n.id, n]));
  return setupArray(plan?.relationships).flatMap((item, index) => {
    const source = nodes.get(setupString(item.character_a || item.characterA));
    const target = nodes.get(setupString(item.character_b || item.characterB));
    if (!source || !target) return [];
    return {
      id: `ov-edge-${index}`,
      source: source.id,
      target: target.id,
      sourceName: source.name,
      targetName: target.name,
      description: setupString(item.description),
    };
  });
});

const modalHiddenPlanFields = new Set([
  "characters",
  "maps",
  "forces",
  "other_settings",
  "tag_groups",
  "length",
  "length_range",
  "perspective",
]);

const planDetails = computed(() => {
  if (!novel.value) return [];
  return overviewDetails(novel.value.planData).filter(
    (detail) =>
      detail.key !== "character_settings" &&
      !modalHiddenPlanFields.has(detail.key)
  );
});

const keyCharacters = computed(() => {
  const value = novel.value?.planData?.character_settings;
  if (!Array.isArray(value)) return [];
  return value
    .filter((item): item is string => typeof item === "string" && !!item.trim())
    .map((item) => item.trim());
});

function pushDetail(
  target: Array<{ key: string; label: string; value: string | string[] }>,
  key: string,
  label: string,
  value: unknown
) {
  if (typeof value !== "string" || !value.trim()) return;
  target.push({ key, label, value: value.trim() });
}

type SetupListItem = {
  title: string;
  appearanceTime: string;
  description: string;
  children: SetupListItem[];
};

type SetupListSection = {
  key: string;
  label: string;
  items: SetupListItem[];
};

function setupListItem(
  title: string,
  description: string,
  appearanceTime = "",
  children: SetupListItem[] = []
): SetupListItem {
  return { title, appearanceTime, description, children };
}

function namedSection(
  key: string,
  label: string,
  value: unknown
): SetupListSection | null {
  if (!Array.isArray(value)) return null;
  const items = value
    .map((item) => {
      if (!item || typeof item !== "object") return null;
      const raw = item as Record<string, unknown>;
      const title = typeof raw.name === "string" ? raw.name.trim() : "";
      const description = typeof raw.notes === "string" ? raw.notes.trim() : "";
      const appearanceTime = setupString(raw.appearance_time);
      if (!title && !description) return null;
      return setupListItem(
        title || description,
        title ? description : "",
        appearanceTime
      );
    })
    .filter((item): item is SetupListItem => !!item);
  return items.length > 0 ? { key, label, items } : null;
}

function otherSettingsSection(value: unknown): SetupListSection | null {
  if (!Array.isArray(value)) return null;
  const items = value
    .map((item) => {
      if (!item || typeof item !== "object") return null;
      const raw = item as Record<string, unknown>;
      const title = typeof raw.title === "string" ? raw.title.trim() : "";
      const description =
        typeof raw.description === "string" ? raw.description.trim() : "";
      const children = namedSection("children", "", raw.items)?.items || [];
      if (!title && !description && children.length === 0) return null;
      return setupListItem(
        title || description || "未命名设定",
        title ? description : "",
        "",
        children
      );
    })
    .filter((item): item is SetupListItem => !!item);
  return items.length > 0
    ? { key: "other_settings", label: "其他设定", items }
    : null;
}

function setupItemKey(
  section: SetupListSection,
  item: SetupListItem,
  index: number
) {
  return `${section.key}-${index}-${item.title}`;
}

function setupChildKey(
  item: SetupListItem,
  child: SetupListItem,
  index: number
) {
  return `${item.title}-${index}-${child.title}`;
}

function normalizeTagGroups(value: unknown) {
  const result: Record<string, string[]> = {};
  if (!value || typeof value !== "object" || Array.isArray(value))
    return result;
  const raw = value as Record<string, unknown>;
  for (const group of ["题材", "类型", "基调", "文风", "雷点"]) {
    const rawTags = raw[group];
    if (!Array.isArray(rawTags)) continue;
    const tags = rawTags
      .filter((tag): tag is string => typeof tag === "string" && !!tag.trim())
      .map((tag) => tag.trim());
    if (tags.length > 0) result[group] = tags;
  }
  for (const [group, rawTags] of Object.entries(raw)) {
    if (result[group] || !Array.isArray(rawTags)) continue;
    const tags = rawTags
      .filter((tag): tag is string => typeof tag === "string" && !!tag.trim())
      .map((tag) => tag.trim());
    if (tags.length > 0) result[group] = tags;
  }
  return result;
}

function detailListItems(value: string | string[]) {
  return Array.isArray(value) ? value : [value];
}

function isLongDetailItem(value: string) {
  return value.length > 32 || /[\r\n]/.test(value);
}

function detailCardClass(value: string, compactTextClass = "") {
  const base = "max-w-full whitespace-pre-wrap rounded-md bg-gray-50 dark:bg-gray-800/70";
  if (isLongDetailItem(value)) {
    return `${base} flex w-full px-3 py-3 leading-6 ${compactTextClass}`.trim();
  }
  return `${base} inline-flex px-2.5 py-1.5 ${compactTextClass}`.trim();
}

function setupString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function setupArray(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value)
    ? value.filter((item): item is Record<string, unknown> => !!item && typeof item === "object")
    : [];
}

function sectionSubtitle(section: SetupListSection): string {
  if (section.key === "characters") {
    return `${section.items.length} 人 · 管理角色信息与关系`;
  }
  if (section.key === "maps") {
    return `${section.items.length} 处 · 管理场景与地理信息`;
  }
  if (section.key === "forces") {
    return `${section.items.length} 个 · 管理阵营、组织与势力关系`;
  }
  return `${section.items.length} 类 · 货币、装备、规则等自定义设定`;
}

watch(
  () => store.overviewNovelId,
  (id) => {
    if (id) return;
    selectedSetupItem.value = null;
    selectedSetupSection.value = null;
    overviewCharacterGraphOpen.value = false;
  }
);

watch(overviewCharacterGraphOpen, (open) => {
  if (!open) return;
  overviewGraphNodePositions.value = {};
  selectedOverviewGraphNodeId.value = null;
  selectedOverviewGraphEdgeId.value = null;
});

function clearOverviewGraphSelection() {
  selectedOverviewGraphNodeId.value = null;
  selectedOverviewGraphEdgeId.value = null;
}

function selectOverviewGraphNode(id: string) {
  selectedOverviewGraphEdgeId.value = null;
  selectedOverviewGraphNodeId.value = id;
}

function selectOverviewGraphEdge(id: string) {
  selectedOverviewGraphNodeId.value = null;
  selectedOverviewGraphEdgeId.value = id;
}

function updateOverviewGraphNodePosition(id: string, position: { x: number; y: number }) {
  overviewGraphNodePositions.value = {
    ...overviewGraphNodePositions.value,
    [id]: position,
  };
}

</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="store.overviewNovelId"
        class="fixed inset-0 z-50 flex items-center justify-center"
      >
        <div
          class="absolute inset-0 bg-black/50"
          @click="store.closeOverview()"
        />
        <div
          class="relative z-10 w-[480px] overflow-hidden rounded-xl bg-white shadow-2xl dark:bg-gray-900"
        >
          <div
            class="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-800"
          >
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              小说梗概
            </h3>
            <button
              class="rounded-lg p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300"
              @click="store.closeOverview()"
            >
              <X class="size-5" />
            </button>
          </div>
          <div class="max-h-[500px] overflow-y-auto p-6">
            <h4 class="text-xl font-bold text-gray-900 dark:text-white">
              {{ novel?.title }}
            </h4>
            <p
              class="mt-1 flex justify-between text-xs text-gray-400 dark:text-gray-500"
            >
              <span>创建于 {{ createdAtLabel }} · {{ updatedAtLabel }}</span
              ><span>{{ totalWords }} 字</span>
            </p>
            <div
              v-if="
                novel &&
                (overviewDetails(novel.planData).length > 0 ||
                  planSummary ||
                  setupDetails.length > 0 ||
                  setupListDetails.length > 0)
              "
              class="mt-4 space-y-4 text-sm text-gray-700 dark:text-gray-300"
            >
              <div
                v-if="planSummary || overviewSummary(novel.planData)"
                :class="detailCardClass(planSummary || overviewSummary(novel.planData))"
              >
                {{ planSummary || overviewSummary(novel.planData) }}
              </div>
              <div class="space-y-3">
                <button
                  v-for="section in setupListDetails"
                  :key="section.key"
                  class="group flex w-full items-center gap-3 rounded-xl bg-gray-50 px-3 py-3 text-left transition-colors hover:bg-gray-100 dark:bg-gray-800/70 dark:hover:bg-gray-800"
                  @click="selectedSetupSection = section"
                >
                  <span
                    class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-white text-gray-500 dark:bg-gray-900 dark:text-gray-300"
                  >
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
              <div v-for="detail in planDetails" :key="detail.key">
                <h5
                  class="mb-1 text-sm font-bold text-gray-950 dark:text-white"
                >
                  {{ detail.label }}
                </h5>
                <div class="flex flex-wrap gap-1.5">
                  <span
                    v-for="(item, index) in detailListItems(detail.value)"
                    :key="`${detail.key}-${index}`"
                    :class="detailCardClass(item)"
                  >
                    {{ item }}
                  </span>
                </div>
              </div>
              <div v-if="keyCharacters.length > 0">
                <h5
                  class="mb-2 text-sm font-bold text-gray-950 dark:text-white"
                >
                  重点人物
                </h5>
                <div class="flex flex-wrap gap-1.5">
                  <span
                    v-for="(character, index) in keyCharacters"
                    :key="`${index}-${character}`"
                    :class="detailCardClass(character, 'text-sm text-gray-700 dark:text-gray-300')"
                  >
                    {{ character }}
                  </span>
                </div>
              </div>
              <div v-for="detail in setupDetails" :key="detail.key">
                <h5
                  class="mb-1 text-sm font-bold text-gray-950 dark:text-white"
                >
                  {{ detail.label }}
                </h5>
                <div class="flex flex-wrap gap-1.5">
                  <span
                    v-for="item in detailListItems(detail.value)"
                    :key="`${detail.key}-${item}`"
                    :class="detailCardClass(item, 'text-sm leading-5 text-gray-700 dark:text-gray-200')"
                  >
                    {{ item }}
                  </span>
                </div>
              </div>
            </div>
            <div
              v-else
              class="mt-6 rounded-lg border border-dashed border-gray-200 p-4 text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400"
            >
              小说梗概尚未生成。请在小说对话中先完成全书规划。
            </div>
          </div>
        </div>
        <div
          v-if="overviewCharacterGraphOpen"
          class="absolute inset-0 z-30 flex items-center justify-center bg-black/30 px-6"
          @click.self="overviewCharacterGraphOpen = false"
        >
          <div class="relative flex h-[86vh] max-h-[86vh] w-full max-w-3xl flex-col rounded-xl bg-white p-4 shadow-xl dark:bg-gray-900">
            <div class="mb-4 flex items-center justify-between">
              <div>
                <h4 class="text-base font-semibold text-gray-900 dark:text-white">人物关系图</h4>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  共 {{ overviewGraphNodes.length }} 人 · {{ overviewGraphEdges.length }} 条关系
                </p>
              </div>
              <button
                class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                @click="overviewCharacterGraphOpen = false"
              >
                <X class="size-4" />
              </button>
            </div>
            <div
              v-if="overviewGraphNodes.length === 0"
              class="flex min-h-56 flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 text-center text-gray-400 dark:border-gray-700"
            >
              <Network class="size-8" />
              <p class="mt-2 text-sm">暂无人物可视化数据</p>
            </div>
            <CharacterRelationGraph
              v-else
              class="min-h-0 flex-1"
              :nodes="overviewGraphNodes"
              :edges="overviewGraphEdges"
              :selected-node-id="selectedOverviewGraphNodeId"
              :selected-edge-id="selectedOverviewGraphEdgeId"
              editable
              @stage-click="clearOverviewGraphSelection"
              @node-click="selectOverviewGraphNode"
              @edge-click="selectOverviewGraphEdge"
              @node-position-change="updateOverviewGraphNodePosition"
            />
            <div
              v-if="selectedOverviewGraphNode"
              class="absolute right-6 top-20 z-10 w-72 rounded-lg border border-gray-200 bg-white/95 p-3 shadow-lg backdrop-blur dark:border-gray-700 dark:bg-gray-900/95"
              @click.stop
            >
              <div class="mb-3 flex items-start justify-between gap-2">
                <div class="min-w-0">
                  <p class="text-xs text-gray-400 dark:text-gray-500">人物详情</p>
                  <h4 class="mt-0.5 truncate text-sm font-semibold text-gray-900 dark:text-white">
                    {{ selectedOverviewGraphNode.name || "未命名人物" }}
                  </h4>
                </div>
                <button
                  class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                  @click="selectedOverviewGraphNodeId = null"
                >
                  <X class="size-3.5" />
                </button>
              </div>
              <div class="space-y-3 text-sm">
                <div>
                  <p class="text-xs font-medium text-gray-400 dark:text-gray-500">出场时间</p>
                  <p class="mt-1 text-gray-700 dark:text-gray-300">
                    {{ selectedOverviewGraphNode.appearanceTime || "未填写" }}
                  </p>
                </div>
                <div>
                  <p class="text-xs font-medium text-gray-400 dark:text-gray-500">详细信息</p>
                  <p class="mt-1 max-h-40 overflow-y-auto whitespace-pre-wrap text-xs leading-5 text-gray-600 dark:text-gray-300">
                    {{ selectedOverviewGraphNode.notes || "未填写详细信息" }}
                  </p>
                </div>
              </div>
            </div>
            <div
              v-if="selectedOverviewGraphEdge"
              class="absolute right-6 top-20 z-10 w-72 rounded-lg border border-gray-200 bg-white/95 p-3 shadow-lg backdrop-blur dark:border-gray-700 dark:bg-gray-900/95"
              @click.stop
            >
              <div class="mb-3 flex items-start justify-between gap-2">
                <div class="min-w-0">
                  <p class="text-xs text-gray-400 dark:text-gray-500">关系说明</p>
                  <h4 class="mt-0.5 truncate text-sm font-semibold text-gray-900 dark:text-white">
                    {{ selectedOverviewGraphEdge.sourceName }}
                    ↔
                    {{ selectedOverviewGraphEdge.targetName }}
                  </h4>
                </div>
                <button
                  class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                  @click="selectedOverviewGraphEdgeId = null"
                >
                  <X class="size-3.5" />
                </button>
              </div>
              <p class="max-h-44 overflow-y-auto whitespace-pre-wrap rounded-lg border border-gray-200 bg-white px-2.5 py-2 text-sm leading-6 text-gray-700 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-300">
                {{ selectedOverviewGraphEdge.description || "未填写关系说明" }}
              </p>
            </div>
          </div>
        </div>
        <div
          v-if="selectedSetupSection"
          class="absolute inset-0 z-20 flex items-center justify-center bg-black/30 px-6"
          @click.self="selectedSetupSection = null"
        >
          <div class="flex max-h-[72vh] w-full max-w-md flex-col rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900">
            <div class="mb-4 flex items-center justify-between">
              <div>
                <h4 class="text-base font-semibold text-gray-900 dark:text-white">
                  {{ selectedSetupSection.label }}
                </h4>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ sectionSubtitle(selectedSetupSection) }}
                </p>
              </div>
              <div class="flex items-center gap-2">
                <button
                  v-if="selectedSetupSection.key === 'characters'"
                  class="rounded-lg border border-gray-200 p-2 text-gray-500 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-400 dark:hover:bg-gray-800"
                  title="查看人物关系图"
                  @click="overviewCharacterGraphOpen = true"
                >
                  <Network class="size-4" />
                </button>
                <button
                  class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                  @click="selectedSetupSection = null"
                >
                  <X class="size-4" />
                </button>
              </div>
            </div>
            <div class="min-h-0 flex-1 space-y-2 overflow-y-auto pr-1">
              <button
                v-for="(item, index) in selectedSetupSection.items"
                :key="setupItemKey(selectedSetupSection, item, index)"
                class="w-full rounded-lg border border-gray-200 bg-white p-3 text-left transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-900 dark:hover:bg-gray-800"
                @click="selectedSetupItem = item"
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
          v-if="selectedSetupItem"
          class="absolute inset-0 z-30 flex items-center justify-center bg-black/30 px-6"
          @click.self="selectedSetupItem = null"
        >
          <div
            class="flex max-h-[72vh] w-full max-w-md flex-col rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900"
          >
            <div class="mb-4 flex shrink-0 items-center justify-between">
              <div>
                <h4
                  class="text-base font-semibold text-gray-900 dark:text-white"
                >
                  {{ selectedSetupItem.title }}
                </h4>
                <p
                  v-if="selectedSetupItem.appearanceTime"
                  class="mt-1 text-xs text-gray-500 dark:text-gray-400"
                >
                  出场时间：{{ selectedSetupItem.appearanceTime }}
                </p>
              </div>
              <button
                class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                @click="selectedSetupItem = null"
              >
                <X class="size-4" />
              </button>
            </div>
            <div class="min-h-0 flex-1 overflow-y-auto pr-1">
              <p
                v-if="selectedSetupItem.description"
                class="whitespace-pre-wrap rounded-lg bg-gray-50 p-3 text-sm leading-6 text-gray-700 dark:bg-gray-800 dark:text-gray-300"
              >
                {{ selectedSetupItem.description }}
              </p>
                <div
                  v-if="selectedSetupItem.children.length > 0"
                  class="mt-3 space-y-2"
                >
                  <div
                    v-for="(child, index) in selectedSetupItem.children"
                    :key="setupChildKey(selectedSetupItem, child, index)"
                    class="rounded-lg border border-gray-200 p-3 dark:border-gray-700"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span
                      class="text-sm font-medium text-gray-900 dark:text-white"
                      >{{ child.title }}</span
                    >
                    <span
                      v-if="child.appearanceTime"
                      class="shrink-0 text-xs text-gray-400"
                      >{{ child.appearanceTime }}</span
                    >
                  </div>
                  <p
                    v-if="child.description"
                    class="mt-1 whitespace-pre-wrap text-xs leading-5 text-gray-600 dark:text-gray-300"
                  >
                    {{ child.description }}
                  </p>
                  </div>
                </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
