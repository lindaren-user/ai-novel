<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from "vue";
import {
  Plus,
  Home,
  MoreHorizontal,
  Loader2,
  FileText,
  Eye,
  Archive,
  Download,
  Share2,
  Trash2,
  Settings,
  HelpCircle,
  ChevronUp,
  X,
} from "lucide-vue-next";
import { useRouter } from "vue-router";
import { useAppStore } from "@/stores/app";
import { createFeedbackWithImagesApi, uploadFileApi } from "@/services/api";
import { overviewSummary } from "@/utils/overview";
import aiNovelLogo from "@/assets/img/ai-novel.webp";
import emptyNovelImage from "@/assets/img/novel.webp";

const store = useAppStore();
const router = useRouter();

const contextMenuId = ref<number | null>(null);
const moreOpen = ref(false);
const MENU_COLLAPSED_KEY = "ai-novel-ide.menu-collapsed";
const menuCollapsed = ref(
  (() => {
    try {
      return localStorage.getItem(MENU_COLLAPSED_KEY) === "1";
    } catch {
      return false;
    }
  })()
);

function toggleMenuCollapsed() {
  menuCollapsed.value = !menuCollapsed.value;
  try {
    localStorage.setItem(MENU_COLLAPSED_KEY, menuCollapsed.value ? "1" : "0");
  } catch {
    /* ignore */
  }
}
const moreTab = ref<"downloads" | "trash">("downloads");
const feedbackOpen = ref(false);
const feedbackText = ref("");
const feedbackImages = ref<File[]>([]);
const feedbackSending = ref(false);
const hasDownloadNotice = computed(
  () =>
    store.downloadJobs.some(
      (job) => job.status === "pending" || job.status === "running"
    ) || !!store.downloadError
);

function formatTimeAgo(date: Date): string {
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const minutes = Math.floor(diff / 60000);
  const hours = Math.floor(diff / 3600000);
  const days = Math.floor(diff / 86400000);

  if (minutes < 1) return "刚刚";
  if (minutes < 60) return `${minutes} 分钟前`;
  if (hours < 24) return `${hours} 小时前`;
  if (days === 1) return "昨天";
  return `${days} 天前`;
}

function toggleContextMenu(id: number, e: Event) {
  e.stopPropagation();
  contextMenuId.value = contextMenuId.value === id ? null : id;
}

function handleView(novelId: number) {
  contextMenuId.value = null;
  store.openOverview(novelId);
}

function handleArchive(novelId: number) {
  contextMenuId.value = null;
  store.openArchiveConfirm(novelId);
}

function openMore(tab: "downloads" | "trash" = "downloads") {
  moreTab.value = tab;
  moreOpen.value = true;
  if (tab === "downloads") void store.loadDownloadJobs();
  if (tab === "trash") void store.loadArchivedNovels();
}

function switchMoreTab(tab: "downloads" | "trash") {
  moreTab.value = tab;
  if (tab === "downloads") void store.loadDownloadJobs();
  if (tab === "trash") void store.loadArchivedNovels();
}

function closeMore() {
  moreOpen.value = false;
}

function openFeedback() {
  feedbackOpen.value = true;
}

function closeFeedback() {
  feedbackOpen.value = false;
}

function onFeedbackFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  addFeedbackImages(input.files);
  input.value = "";
}

function onFeedbackDrop(event: DragEvent) {
  addFeedbackImages(event.dataTransfer?.files || null);
}

function addFeedbackImages(files: FileList | null) {
  if (!files) return;
  const next = [...feedbackImages.value];
  for (const file of Array.from(files)) {
    if (!["image/png", "image/jpeg"].includes(file.type)) {
      store.notifyError("仅支持上传 png、jpg 图片");
      continue;
    }
    next.push(file);
  }
  feedbackImages.value = next;
}

function removeFeedbackImage(index: number) {
  feedbackImages.value = feedbackImages.value.filter((_, i) => i !== index);
}

async function sendFeedback() {
  const content = feedbackText.value.trim();
  if (!content) {
    store.notifyError("反馈内容不能为空");
    return;
  }
  feedbackSending.value = true;
  try {
    const imageUrls: string[] = [];
    for (const image of feedbackImages.value) {
      const uploaded = await uploadFileApi(image);
      imageUrls.push(uploaded.url);
    }
    await createFeedbackWithImagesApi(content, imageUrls);
    store.notifyInfo("反馈已提交");
    feedbackText.value = "";
    feedbackImages.value = [];
    closeFeedback();
  } catch (err) {
    store.notifyError(err instanceof Error ? err.message : "反馈提交失败");
  } finally {
    feedbackSending.value = false;
  }
}

function handleDownloadNovel(novelId: number) {
  contextMenuId.value = null;
  void store.startDownload("novel", novelId);
}

async function handleShareNovel(novelId: number) {
  contextMenuId.value = null;
  const novel = store.novels.find((n) => n.id === novelId);
  if (novel) {
    const overview = await store.loadNovelOverview(novelId);
    store.openShare(
      "novel",
      novel.id,
      novel.title,
      overviewSummary(overview?.planData || novel.planData) || "小说梗概尚未生成"
    );
  }
}

function isSetupNovel(status: number) {
  return status === 1;
}

function closeContextMenu() {
  contextMenuId.value = null;
}

function onDocumentClick(e: MouseEvent) {
  if (!contextMenuId.value && !moreOpen.value) return;
  const target = e.target as HTMLElement;
  if (
    !target.closest(".context-menu-trigger") &&
    !target.closest(".context-menu-dropdown")
  ) {
    closeContextMenu();
  }
  if (!target.closest(".more-trigger") && !target.closest(".more-panel")) {
    closeMore();
  }
}

onMounted(() => document.addEventListener("click", onDocumentClick));
onUnmounted(() => document.removeEventListener("click", onDocumentClick));
</script>

<template>
  <div
    class="flex h-screen w-[240px] shrink-0 flex-col border-r border-gray-200 bg-white transition-all dark:border-gray-800 dark:bg-gray-950"
  >
    <!-- Header -->
    <div
      class="flex h-16 items-center justify-between border-b border-gray-200 px-4 dark:border-gray-800"
    >
      <h2 class="min-w-0 font-medium text-gray-900 dark:text-white">
        <button
          class="block max-w-32"
          @click="router.push('/')"
        >
          <img :src="aiNovelLogo" alt="AI Novel" class="h-11 w-auto object-contain dark:invert" />
        </button>
      </h2>
      <div class="flex items-center gap-1">
        <button
          class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
          title="工作台首页"
          @click="store.openWorkspaceHome()"
        >
          <Home class="size-5" />
        </button>
        <button
          class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
          :disabled="store.isNovelCreating"
          :class="store.isNovelCreating ? 'cursor-not-allowed opacity-50' : ''"
          @click="store.beginNovelSetupForm()"
        >
          <Plus class="size-5" />
        </button>
      </div>
    </div>

    <!-- Novel List -->
    <div class="flex-1 overflow-y-auto">
      <!-- Empty state: no novels -->
      <div
        v-if="store.isNovelsLoading"
        class="flex flex-col items-center justify-center py-16 text-gray-400 dark:text-gray-500"
      >
        <p class="text-sm">正在加载小说...</p>
      </div>

      <div
        v-else-if="store.novels.length === 0"
        class="flex flex-col items-center justify-center py-16 text-gray-400 dark:text-gray-500"
      >
        <img
          :src="emptyNovelImage"
          alt=""
          class="h-24 w-32 object-contain opacity-80 dark:opacity-60 dark:contrast-90"
        />
        <p class="mt-2 text-sm">暂无小说</p>
        <p class="mt-1 text-xs">点击右上角 + 创建第一本小说</p>
      </div>

      <div class="space-y-1 p-2">
        <div
          v-for="novel in store.novels"
          :key="novel.id"
          class="group relative"
        >
          <div
            class="flex w-full min-w-0 cursor-pointer items-start gap-3 rounded-lg px-3 py-3 text-left transition-colors"
            :class="
              store.selectedNovelId === novel.id
                ? 'bg-gray-100 dark:bg-gray-800'
                : 'hover:bg-gray-50 dark:hover:bg-gray-800/50'
            "
            @click="store.selectNovel(novel.id)"
          >
            <div
              class="flex size-6 shrink-0 items-center justify-center rounded"
              :class="
                isSetupNovel(novel.status)
                  ? 'text-amber-500 dark:text-amber-300'
                  : 'text-gray-400 dark:text-gray-500'
              "
            >
              <FileText class="size-5" />
            </div>
            <div class="min-w-0 flex-1 overflow-hidden">
              <div
                class="truncate text-sm font-medium text-gray-900 dark:text-white"
                :title="novel.title"
              >
                {{ novel.title }}
              </div>
              <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {{
                  isSetupNovel(novel.status)
                    ? `设定暂存 · ${formatTimeAgo(novel.updatedAt)}`
                    : formatTimeAgo(novel.updatedAt)
                }}
              </div>
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <Loader2
                v-if="store.isNovelOrChildStreaming(novel.id)"
                class="size-4 animate-spin text-gray-400"
                title="AI 正在回复"
              />
              <div class="relative w-6 shrink-0">
                <button
                  v-if="!isSetupNovel(novel.status)"
                  class="context-menu-trigger rounded p-1 text-gray-400 opacity-0 transition-opacity group-hover:opacity-100 hover:bg-gray-200 dark:text-gray-500 dark:hover:bg-gray-700"
                  @click="toggleContextMenu(novel.id, $event)"
                >
                  <MoreHorizontal class="size-4" />
                </button>
                <!-- Context Menu -->
                <div
                  v-if="contextMenuId === novel.id"
                  class="context-menu-dropdown absolute right-0 top-full z-20 mt-1 w-28 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
                  @click.stop
                >
                  <button
                    class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    @click="handleView(novel.id)"
                  >
                    <Eye class="size-4" />
                    查看
                  </button>
                  <button
                    class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    @click="handleArchive(novel.id)"
                  >
                    <Archive class="size-4" />
                    归档
                  </button>
                  <button
                    class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    @click="handleDownloadNovel(novel.id)"
                  >
                    <Download class="size-4" />
                    下载
                  </button>
                  <button
                    class="flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                    @click="handleShareNovel(novel.id)"
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

    <div class="border-t border-gray-200 px-3 py-2 dark:border-gray-800">
      <div v-show="!menuCollapsed" class="space-y-1">
        <button
          class="more-trigger relative flex w-full items-center gap-3 rounded-lg px-2 py-2.5 text-sm text-gray-600 transition-colors hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
          @click="openMore(hasDownloadNotice ? 'downloads' : 'trash')"
        >
          <MoreHorizontal class="size-5 shrink-0" />
          <span class="font-medium">更多</span>
          <span
            v-if="hasDownloadNotice"
            class="absolute right-3 top-2 size-2 rounded-full bg-red-500"
          />
        </button>
        <button
          v-if="store.isLoggedIn"
          class="flex w-full items-center gap-3 rounded-lg px-2 py-2.5 text-sm text-gray-600 transition-colors hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
          @click="store.openSettings('general')"
        >
          <Settings class="size-5 shrink-0" />
          <span class="font-medium">设置</span>
        </button>
        <button
          class="flex w-full items-center gap-3 rounded-lg px-2 py-2.5 text-sm text-gray-600 transition-colors hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
          @click="openFeedback"
        >
          <HelpCircle class="size-5 shrink-0" />
          <span class="font-medium">帮助与反馈</span>
        </button>
      </div>

      <div
        v-if="store.isLoggedIn"
        class="flex cursor-pointer items-center gap-3 rounded-lg px-2 py-2 transition-colors hover:bg-gray-100 dark:hover:bg-gray-800"
        @click="toggleMenuCollapsed"
      >
        <div
          class="flex size-9 shrink-0 items-center justify-center rounded-full bg-gray-900 text-sm font-medium text-white dark:bg-gray-100 dark:text-gray-900"
        >
          {{ store.authUsername.charAt(0) || "U" }}
        </div>
        <div class="min-w-0 flex-1">
          <div
            class="truncate text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            {{ store.authUsername || store.accountSettings.username }}
          </div>
          <div class="truncate text-xs text-gray-500 dark:text-gray-400">
            {{ store.accountSettings.email || "未绑定邮箱" }}
          </div>
        </div>
        <span
          class="shrink-0 rounded bg-gray-900 px-1.5 py-0.5 text-[10px] text-white dark:bg-gray-100 dark:text-gray-900"
          title="3次并发次数"
          >Free</span
        >
        <ChevronUp
          class="size-4 shrink-0 text-gray-400 transition-transform"
          :class="menuCollapsed ? 'rotate-180' : ''"
        />
      </div>

      <button
        v-else
        class="mt-3 w-full rounded-lg border border-dashed border-gray-300 bg-gray-50 px-3 py-3 text-center transition-colors hover:bg-gray-100 dark:border-gray-700 dark:bg-gray-800/50 dark:hover:bg-gray-800"
        @click="store.openAuthModal('login')"
      >
        <p class="text-xs text-gray-500 dark:text-gray-400">未登录</p>
        <p
          class="mt-0.5 text-xs font-medium text-gray-900 underline underline-offset-4 dark:text-white"
        >
          点击登录
        </p>
      </button>
    </div>

    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="moreOpen"
          class="fixed inset-0 z-[70] flex items-center justify-center"
        >
          <div class="absolute inset-0 bg-black/50" @click="closeMore" />
          <div
            class="more-panel relative z-10 flex h-[600px] w-[720px] overflow-hidden rounded-xl bg-white shadow-2xl dark:bg-gray-900"
          >
            <aside
              class="flex w-[180px] flex-col border-r border-gray-200 bg-gray-50 p-4 dark:border-gray-800 dark:bg-gray-950"
            >
              <div class="mb-4">
                <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
                  更多
                </h3>
              </div>
              <div class="flex-1 space-y-1">
                <button
                  class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors"
                  :class="
                    moreTab === 'downloads'
                      ? 'bg-white text-gray-900 shadow-sm dark:bg-gray-800 dark:text-white'
                      : 'text-gray-600 hover:bg-white dark:text-gray-400 dark:hover:bg-gray-800'
                  "
                  @click="switchMoreTab('downloads')"
                >
                  <Download class="size-5" />
                  下载任务
                  <span class="ml-auto flex items-center gap-2">
                    <span class="text-xs text-gray-400">{{
                      store.downloadJobs.length
                    }}</span>
                    <span
                      v-if="hasDownloadNotice"
                      class="size-2 rounded-full bg-red-500"
                    />
                  </span>
                </button>
                <button
                  class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors"
                  :class="
                    moreTab === 'trash'
                      ? 'bg-white text-gray-900 shadow-sm dark:bg-gray-800 dark:text-white'
                      : 'text-gray-600 hover:bg-white dark:text-gray-400 dark:hover:bg-gray-800'
                  "
                  @click="switchMoreTab('trash')"
                >
                  <Trash2 class="size-5" />
                  回收站
                  <span class="ml-auto text-xs text-gray-400">{{
                    store.archivedNovels.length
                  }}</span>
                </button>
              </div>
            </aside>

            <section class="flex min-w-0 flex-1 flex-col">
              <div
                class="flex h-[65px] items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-800"
              >
                <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
                  {{ moreTab === "downloads" ? "下载任务" : "回收站" }}
                </h3>
                <button
                  class="rounded-lg p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300"
                  @click="closeMore"
                >
                  <X class="size-5" />
                </button>
              </div>

              <div
                v-if="moreTab === 'downloads'"
                class="flex-1 overflow-y-auto p-6"
              >
                <div v-if="store.downloadJobs.length > 0" class="space-y-3">
                  <div
                    v-for="job in store.downloadJobs"
                    :key="job.id"
                    class="rounded-lg border border-gray-200 p-4 dark:border-gray-800"
                  >
                    <div class="flex items-center justify-between gap-3">
                      <div class="min-w-0">
                        <p
                          class="truncate text-sm font-medium text-gray-900 dark:text-white"
                        >
                          {{ job.filename || "下载文件" }}
                        </p>
                        <p
                          class="mt-1 text-xs"
                          :class="
                            job.status === 'error'
                              ? 'text-red-500'
                              : 'text-gray-500 dark:text-gray-400'
                          "
                        >
                          {{ job.message || "正在准备下载" }}
                        </p>
                      </div>
                      <span
                        class="shrink-0 text-xs text-gray-500 dark:text-gray-400"
                        >{{ job.progress || 0 }}%</span
                      >
                    </div>
                    <div
                      class="mt-3 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-gray-800"
                    >
                      <div
                        class="h-full rounded-full bg-gray-900 transition-all dark:bg-gray-100"
                        :style="{ width: `${job.progress || 0}%` }"
                      />
                    </div>
                  </div>
                </div>
                <div
                  v-else
                  class="flex h-full flex-col items-center justify-center text-gray-400 dark:text-gray-500"
                >
                  <Download class="size-10" />
                  <p class="mt-2 text-sm">暂无下载任务</p>
                </div>
              </div>

              <div v-else class="flex-1 overflow-y-auto p-6">
                <div
                  v-if="store.archivedNovels.length === 0"
                  class="flex h-full flex-col items-center justify-center text-gray-400 dark:text-gray-500"
                >
                  <Archive class="size-10" />
                  <p class="mt-2 text-sm">回收站为空</p>
                </div>
                <div v-else class="space-y-1">
                  <div
                    v-for="novel in store.archivedNovels"
                    :key="novel.id"
                    class="flex items-center gap-3 rounded-lg px-3 py-3 hover:bg-gray-50 dark:hover:bg-gray-800/50"
                  >
                    <FileText class="size-5 shrink-0 text-gray-400" />
                    <div class="min-w-0 flex-1">
                      <p
                        class="truncate text-sm font-medium text-gray-900 dark:text-white"
                      >
                        {{ novel.title }}
                      </p>
                      <p class="text-xs text-gray-500 dark:text-gray-400">
                        {{ formatTimeAgo(novel.updatedAt) }}
                      </p>
                    </div>
                    <button
                      class="rounded-lg px-3 py-1.5 text-xs text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                      @click="store.restoreNovel(novel.id)"
                    >
                      恢复
                    </button>
                  </div>
                </div>
              </div>
            </section>
          </div>
        </div>
      </Transition>
    </Teleport>

    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="feedbackOpen"
          class="fixed inset-0 z-[80] flex items-center justify-center bg-black/30 px-4"
          @click.self="closeFeedback"
        >
          <div
            class="w-full max-w-xl rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900"
          >
            <div class="mb-4 flex items-center justify-between">
              <div>
                <h3
                  class="text-base font-semibold text-gray-900 dark:text-white"
                >
                  帮助与反馈
                </h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  您也可以添加
                  <button
                    class="text-blue-500 underline underline-offset-2 hover:text-blue-600 dark:text-blue-400 dark:hover:text-blue-300"
                  >
                    交流群
                  </button>
                </p>
              </div>
              <button
                class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200"
                @click="closeFeedback"
              >
                <X class="size-4" />
              </button>
            </div>

            <div
              class="flex h-52 flex-col rounded-lg border border-gray-200 bg-white p-2 focus-within:border-gray-400 dark:border-gray-700 dark:bg-gray-950"
              @dragover.prevent
              @drop.prevent="onFeedbackDrop"
            >
              <div
                v-if="feedbackImages.length"
                class="mb-2 flex max-h-16 min-h-10 flex-wrap gap-2 overflow-y-auto"
              >
                <div
                  v-for="(file, index) in feedbackImages"
                  :key="`${file.name}-${file.size}-${file.lastModified}`"
                  class="group inline-flex max-w-56 items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-2.5 py-1.5 text-xs text-gray-600 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-300"
                  :title="file.name"
                >
                  <FileText class="size-3.5 shrink-0 text-gray-400" />
                  <span class="truncate">{{ file.name }}</span>
                  <button
                    class="rounded p-0.5 text-gray-400 hover:bg-gray-200 hover:text-gray-700 dark:hover:bg-gray-800 dark:hover:text-gray-100"
                    @click="removeFeedbackImage(index)"
                  >
                    <X class="size-3" />
                  </button>
                </div>
              </div>
              <textarea
                v-model="feedbackText"
                class="min-h-0 flex-1 resize-none bg-transparent px-1 py-1 text-sm leading-6 text-gray-700 outline-none placeholder:text-gray-400 dark:text-gray-300"
                placeholder="填写您在使用过程中遇到的问题或建议"
              />
              <div class="mt-2 flex items-center">
                <label
                  class="inline-flex size-8 cursor-pointer items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 hover:text-gray-800 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-100"
                  title="上传 png / jpg 图片"
                >
                  <Plus class="size-4" />
                  <input
                    class="hidden"
                    type="file"
                    multiple
                    accept="image/png,image/jpeg,.png,.jpg,.jpeg"
                    @change="onFeedbackFileChange"
                  />
                </label>
              </div>
            </div>

            <div class="mt-5 flex justify-end gap-2">
              <button
                class="rounded-lg border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
                @click="closeFeedback"
              >
                取消
              </button>
              <button
                class="inline-flex items-center gap-2 rounded-lg bg-gray-900 px-5 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-200"
                :disabled="feedbackSending"
                @click="sendFeedback"
              >
                {{ feedbackSending ? "发送中..." : "发送" }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
