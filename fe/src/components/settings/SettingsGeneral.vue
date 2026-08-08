<script setup lang="ts">
import { computed, ref, watch, onUnmounted } from "vue";
import {
  Check,
  ChevronDown,
  Eye,
  EyeOff,
  Info,
  Minus,
  Plus,
  Trash2,
  Wrench,
} from "lucide-vue-next";
import { useAppStore } from "@/stores/app";

const store = useAppStore();

const downloadFormatOpen = ref(false);
const customDraftOpen = ref(false);
const shareSecurityRef = ref<HTMLElement | null>(null);
const showShareSecurityKey = ref(false);

const downloadFormatOptions = [".txt", ".docx", ".pdf"];
const downloadLayoutOptions = [
  { value: "volume", label: "按卷" },
  { value: "chapter", label: "按章" },
] as const;
const providerOptions = [
  {
    value: "deepseek",
    label: "DeepSeek",
    hint: "DeepSeek API",
    apiUrl: "https://api.deepseek.com/",
  },
  {
    value: "gpt",
    label: "GPT",
    hint: "OpenAI API",
    apiUrl: "https://api.openai.com/v1",
  },
  {
    value: "gemini",
    label: "Gemini",
    hint: "Google 原生",
    apiUrl: "https://generativelanguage.googleapis.com",
  },
  {
    value: "claude",
    label: "Claude",
    hint: "Anthropic 原生",
    apiUrl: "https://api.anthropic.com/v1",
  },
  {
    value: "kimi",
    label: "Kimi",
    hint: "Moonshot 兼容",
    apiUrl: "https://api.moonshot.cn/v1",
  },
  {
    value: "doubao",
    label: "豆包",
    hint: "火山方舟原生",
    apiUrl: "https://ark.cn-beijing.volces.com/api/v3",
  },
  {
    value: "qianwen",
    label: "通义千问",
    hint: "Qwen 组件",
    apiUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1",
  },
  {
    value: "custom_openai_completions",
    label: "Completions",
    hint: "Completions API",
    apiUrl: "",
  },
  // Responses API 适配器还在校准工具调用续写协议，稳定前先隐藏入口。
  // {
  //   value: "custom_openai_responses",
  //   label: "Responses",
  //   hint: "Responses API",
  //   apiUrl: "",
  // },
];

const customModelName = ref("");
const testStatus = ref<"idle" | "testing" | "success" | "error">("idle");
const testMessage = ref("");
const handledCustomModelTick = ref(0);
const hasPassedConnectionTest = ref(false);
const deleteModelDialogOpen = ref(false);

const customModels = computed(() =>
  store.models.filter((item) => !!item.userId)
);
const selectedMode = computed(() =>
  store.isCustomModelSelected || customDraftOpen.value ? "custom" : "official"
);

const selectedProviderValue = computed(() =>
  providerValueByApiUrl(store.generalSettings.customApiUrl)
);

function normalizedApiUrl(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function providerValueByApiUrl(apiUrl: string) {
  const normalized = normalizedApiUrl(apiUrl);
  if (!normalized) {
    return "custom_openai_completions";
  }
  const matched = providerOptions.find(
    (provider) =>
      provider.value !== "custom_openai_completions" &&
      provider.value !== "custom_openai_responses" &&
      normalizedApiUrl(provider.apiUrl) === normalized
  );
  return matched?.value || "custom_openai_completions";
}

const apiPrefixTitle = computed(() => {
  return "填写 SDK/API 使用的接口前缀，可以包含版本路径，例如 /v1、/api/v3、/compatible-mode/v1；自定义兼容会按 OpenAI Chat Completions API 协议拼接 /chat/completions，不要填写 /chat/completions、/messages、:generateContent 这类具体接口。";
});

function resetConnectionTest() {
  hasPassedConnectionTest.value = false;
  if (testStatus.value === "success") {
    testStatus.value = "idle";
    testMessage.value = "";
  }
}

function selectModel(modelId: number) {
  const model = store.models.find((item) => item.id === modelId);
  if (!model) return;
  store.generalSettings.modelId = modelId;
  store.generalSettings.model = model.name;
  store.generalSettings.customProvider = model.provider;
  store.generalSettings.customModelId = model.modelId;
  store.generalSettings.customApiUrl = model.apiUrl;
  store.generalSettings.customApiKey = model.apiKey;
  hasPassedConnectionTest.value = true;
  customDraftOpen.value = false;
}

function selectOfficialModel() {
  const model = store.officialModel;
  if (!model) return;
  store.generalSettings.modelId = model.id;
  store.generalSettings.model = model.name;
  customDraftOpen.value = false;
}

function startCustomModel() {
  customModelName.value = "";
  store.generalSettings.customProvider = "deepseek";
  store.generalSettings.customModelId = "";
  store.generalSettings.customApiUrl = "https://api.deepseek.com/v1";
  store.generalSettings.customApiKey = "";
  resetConnectionTest();
  customDraftOpen.value = true;
}

watch(
  () => store.customModelRequestTick,
  (tick) => {
    if (tick <= handledCustomModelTick.value) return;
    handledCustomModelTick.value = tick;
    startCustomModel();
  },
  { immediate: true }
);

watch(
  () => store.shareSecurityRequestTick,
  (tick) => {
    if (tick <= 0) return;
    setTimeout(
      () =>
        shareSecurityRef.value?.scrollIntoView({
          block: "center",
          behavior: "smooth",
        }),
      0
    );
  }
);

function selectProvider(provider: (typeof providerOptions)[number]) {
  store.generalSettings.customProvider = provider.value;
  store.generalSettings.customApiUrl = provider.apiUrl;
  resetConnectionTest();
}

function setConsistencyCheckCount(value: number) {
  const count = Math.trunc(Number(value) || 0);
  store.generalSettings.consistencyCheckCount = Math.min(
    10,
    Math.max(1, count || 3)
  );
}

watch(
  () => store.generalSettings.customApiUrl,
  (apiUrl) => {
    store.generalSettings.customProvider = providerValueByApiUrl(apiUrl);
  }
);

watch(
  () => [
    store.generalSettings.customModelId,
    store.generalSettings.customApiUrl,
    store.generalSettings.customApiKey,
  ],
  () => {
    if (customDraftOpen.value) resetConnectionTest();
  }
);

let testTimer: ReturnType<typeof setTimeout> | null = null;

function clearTestTimer() {
  if (testTimer) {
    clearTimeout(testTimer);
    testTimer = null;
  }
}

function resetTestStatusSoon() {
  testTimer = setTimeout(() => {
    testStatus.value = "idle";
    testMessage.value = "";
  }, 3000);
}

async function testConnection() {
  clearTestTimer();
  if (
    !store.generalSettings.customModelId.trim() ||
    !store.generalSettings.customApiUrl.trim() ||
    !store.generalSettings.customApiKey.trim()
  ) {
    testStatus.value = "error";
    testMessage.value = "请先填写模型 ID、API 地址和 API Key";
    resetTestStatusSoon();
    return;
  }
  testStatus.value = "testing";
  testMessage.value = "";
  try {
    const result = await store.testCustomModel();
    testStatus.value = result.ok ? "success" : "error";
    testMessage.value = result.message || (result.ok ? "连接成功" : "连接失败");
    hasPassedConnectionTest.value = !!result.ok;
  } catch (err) {
    void err;
    testStatus.value = "error";
    hasPassedConnectionTest.value = false;
    testMessage.value = "连接失败";
  }
  resetTestStatusSoon();
}

async function saveCustomModel() {
  if (!hasPassedConnectionTest.value) {
    testStatus.value = "error";
    testMessage.value = "请先测试连接成功后再保存";
    return;
  }
  try {
    await store.createCustomModel(customModelName.value.trim() || undefined);
    customDraftOpen.value = false;
    testStatus.value = "success";
    testMessage.value = "已保存并启用";
    hasPassedConnectionTest.value = true;
    resetTestStatusSoon();
  } catch {
    testStatus.value = "error";
    testMessage.value = "保存失败，请检查配置";
  }
}

async function deleteSelectedCustomModel() {
  const modelId = store.generalSettings.modelId;
  if (!modelId || !store.isCustomModelSelected) return;
  await store.deleteCustomModel(modelId);
  deleteModelDialogOpen.value = false;
}

onUnmounted(() => {
  clearTestTimer();
});
</script>

<template>
  <div class="space-y-8">
    <!-- Share Security -->
    <div ref="shareSecurityRef" class="flex items-start justify-between">
      <div>
        <h4 class="font-medium text-gray-900 dark:text-white">分享安全</h4>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          分享时用于身份判断的密钥
        </p>
      </div>
      <div class="relative w-48">
        <input
          :value="store.generalSettings.shareSecurityKey"
          :type="showShareSecurityKey ? 'text' : 'password'"
          placeholder="输入密钥..."
          class="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 pr-9 text-sm text-gray-700 outline-none placeholder:text-gray-400 focus:border-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:placeholder:text-gray-500"
          @blur="
            store.generalSettings.shareSecurityKey = (
              $event.target as HTMLInputElement
            ).value
          "
        />
        <button
          type="button"
          class="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
          title="显示或隐藏密钥"
          @click="showShareSecurityKey = !showShareSecurityKey"
        >
          <EyeOff v-if="showShareSecurityKey" class="size-4" />
          <Eye v-else class="size-4" />
        </button>
      </div>
    </div>

    <!-- Download Format -->
    <div class="flex items-start justify-between">
      <div>
        <h4 class="font-medium text-gray-900 dark:text-white">下载格式</h4>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          选择下载组织方式和文件格式
        </p>
      </div>
      <div class="flex items-center gap-2">
        <div
          class="flex overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800"
        >
          <button
            v-for="option in downloadLayoutOptions"
            :key="option.value"
            class="px-3 py-2 text-sm transition-colors"
            :class="
              store.generalSettings.downloadLayout === option.value
                ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900'
                : 'text-gray-600 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700'
            "
            @click="store.generalSettings.downloadLayout = option.value"
          >
            {{ option.label }}
          </button>
        </div>
        <div class="relative">
          <button
            class="flex w-20 items-center justify-between rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
            @click="downloadFormatOpen = !downloadFormatOpen"
          >
            {{ store.generalSettings.downloadFormat }}
            <ChevronDown class="size-4 text-gray-400" />
          </button>
          <div
            v-if="downloadFormatOpen"
            class="absolute right-0 z-10 mt-1 w-20 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
          >
            <button
              v-for="fmt in downloadFormatOptions"
              :key="fmt"
              class="w-full px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
              :class="
                store.generalSettings.downloadFormat === fmt
                  ? 'bg-gray-100 dark:bg-gray-700'
                  : ''
              "
              @click="
                store.generalSettings.downloadFormat = fmt;
                downloadFormatOpen = false;
              "
            >
              {{ fmt }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Consistency Check Count -->
    <div class="flex items-start justify-between">
      <div>
        <h4 class="font-medium text-gray-900 dark:text-white">
          一致性校验次数
        </h4>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          高一致性模式下最多重写和校验的轮数
        </p>
      </div>
      <div
        class="flex h-10 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800"
      >
        <button
          type="button"
          class="flex w-10 items-center justify-center text-gray-500 transition-colors hover:bg-gray-50 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-40 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white"
          title="减少次数"
          :disabled="store.generalSettings.consistencyCheckCount <= 1"
          @click="
            setConsistencyCheckCount(
              store.generalSettings.consistencyCheckCount - 1
            )
          "
        >
          <Minus class="size-4" />
        </button>
        <input
          :value="store.generalSettings.consistencyCheckCount"
          type="number"
          min="1"
          max="10"
          step="1"
          class="w-14 border-x border-gray-200 bg-transparent px-2 text-center text-sm tabular-nums text-gray-700 outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none dark:border-gray-700 dark:text-gray-300"
          @input="
            setConsistencyCheckCount(
              Number(($event.target as HTMLInputElement).value)
            )
          "
          @blur="
            setConsistencyCheckCount(
              store.generalSettings.consistencyCheckCount
            )
          "
        />
        <button
          type="button"
          class="flex w-10 items-center justify-center text-gray-500 transition-colors hover:bg-gray-50 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-40 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white"
          title="增加次数"
          :disabled="store.generalSettings.consistencyCheckCount >= 10"
          @click="
            setConsistencyCheckCount(
              store.generalSettings.consistencyCheckCount + 1
            )
          "
        >
          <Plus class="size-4" />
        </button>
      </div>
    </div>

    <!-- Model Selection -->
    <div>
      <div class="flex items-start justify-between gap-6">
        <div>
          <h4 class="font-medium text-gray-900 dark:text-white">模型选择</h4>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            选择你的 AI 模型
          </p>
        </div>

        <div class="grid w-72 grid-cols-2 gap-2">
          <button
            class="relative rounded-lg border px-3 py-3 text-left transition-colors"
            :class="
              selectedMode === 'official'
                ? 'border-gray-900 bg-white text-gray-900 dark:border-white dark:bg-gray-800 dark:text-white'
                : 'border-gray-200 bg-gray-50 text-gray-600 hover:bg-white dark:border-gray-700 dark:bg-gray-800/60 dark:text-gray-400 dark:hover:bg-gray-800'
            "
            @click="selectOfficialModel"
          >
            <span class="block text-sm font-medium">官方模型</span>
            <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400"
              >默认托管</span
            >
            <Check
              v-if="selectedMode === 'official'"
              class="absolute right-2 top-2 size-4"
            />
          </button>

          <button
            class="relative rounded-lg border px-3 py-3 text-left transition-colors"
            :class="
              selectedMode === 'custom'
                ? 'border-gray-900 bg-white text-gray-900 dark:border-white dark:bg-gray-800 dark:text-white'
                : 'border-gray-200 bg-gray-50 text-gray-600 hover:bg-white dark:border-gray-700 dark:bg-gray-800/60 dark:text-gray-400 dark:hover:bg-gray-800'
            "
            @click="
              customModels.length > 0
                ? selectModel(customModels[0].id)
                : startCustomModel()
            "
          >
            <span class="block text-sm font-medium">自定义模型</span>
            <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400"
              >用户 API</span
            >
            <Check
              v-if="selectedMode === 'custom'"
              class="absolute right-2 top-2 size-4"
            />
          </button>
        </div>
      </div>

      <div
        v-if="selectedMode === 'custom'"
        class="mt-6 space-y-6 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800/50"
      >
        <div v-if="customModels.length > 0" class="flex flex-wrap gap-2">
          <button
            v-for="model in customModels"
            :key="model.id"
            class="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm transition-colors"
            :class="
              store.generalSettings.modelId === model.id
                ? 'border-gray-900 bg-white text-gray-900 dark:border-white dark:bg-gray-800 dark:text-white'
                : 'border-gray-200 bg-white text-gray-600 hover:border-gray-300 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-400 dark:hover:border-gray-600'
            "
            @click="selectModel(model.id)"
          >
            <Wrench class="size-4" />
            <span>{{ model.name }}</span>
          </button>
          <button
            class="rounded-lg border border-dashed border-gray-300 px-3 py-2 text-sm text-gray-600 hover:bg-white dark:border-gray-600 dark:text-gray-400 dark:hover:bg-gray-900"
            @click="startCustomModel"
          >
            新增自定义模型
          </button>
        </div>

        <div
          v-if="customDraftOpen"
          class="flex items-start justify-between gap-4"
        >
          <div class="w-24 shrink-0 pt-1">
            <h4 class="text-sm font-medium text-gray-900 dark:text-white">
              模型名称
            </h4>
          </div>
          <div class="flex-1">
            <input
              v-model="customModelName"
              type="text"
              placeholder="输入自定义名称，仅方便自己查看"
              class="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 outline-none placeholder:text-gray-400 focus:border-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:placeholder:text-gray-500"
            />
          </div>
        </div>

        <div
          v-if="customDraftOpen"
          class="flex items-start justify-between gap-4"
        >
          <div class="w-24 shrink-0 pt-1">
            <h4 class="text-sm font-medium text-gray-900 dark:text-white">
              厂商
            </h4>
          </div>
          <div class="grid flex-1 grid-cols-2 gap-2 sm:grid-cols-3">
            <button
              v-for="provider in providerOptions"
              :key="provider.value"
              class="rounded-lg border px-3 py-2 text-left transition-colors"
              :class="
                selectedProviderValue === provider.value
                  ? 'border-gray-900 bg-white text-gray-900 dark:border-white dark:bg-gray-800 dark:text-white'
                  : 'border-gray-200 bg-white text-gray-600 hover:border-gray-300 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-400 dark:hover:border-gray-600'
              "
              @click="selectProvider(provider)"
            >
              <span class="block text-sm font-medium">{{
                provider.label
              }}</span>
              <span
                class="mt-0.5 block text-xs text-gray-500 dark:text-gray-500"
                >{{ provider.hint }}</span
              >
            </button>
          </div>
        </div>

        <div class="flex items-start justify-between gap-4">
          <div class="w-24 shrink-0 pt-1">
            <h4 class="text-sm font-medium text-gray-900 dark:text-white">
              模型 ID
            </h4>
          </div>
          <div class="flex-1">
            <input
              v-model="store.generalSettings.customModelId"
              type="text"
              :readonly="!customDraftOpen"
              :disabled="!customDraftOpen"
              placeholder="输入模型 ID"
              class="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 outline-none placeholder:text-gray-400 focus:border-gray-400 disabled:cursor-not-allowed disabled:opacity-60 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:placeholder:text-gray-500"
            />
          </div>
        </div>

        <div class="flex items-start justify-between gap-4">
          <div class="w-24 shrink-0 pt-1">
            <h4
              class="flex items-center gap-1 text-sm font-medium text-gray-900 dark:text-white"
            >
              接口前缀
              <span
                :title="apiPrefixTitle"
                class="inline-flex shrink-0 cursor-help"
              >
                <Info class="size-3 text-gray-400" />
              </span>
            </h4>
          </div>
          <div class="flex-1">
            <input
              v-model="store.generalSettings.customApiUrl"
              type="text"
              :readonly="!customDraftOpen"
              :disabled="!customDraftOpen"
              placeholder="例如 https://api.openai.com/v1"
              class="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 outline-none placeholder:text-gray-400 focus:border-gray-400 disabled:cursor-not-allowed disabled:opacity-60 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:placeholder:text-gray-500"
            />
          </div>
        </div>
        <div class="flex items-start justify-between gap-4">
          <div class="w-24 shrink-0 pt-1">
            <h4 class="text-sm font-medium text-gray-900 dark:text-white">
              API Key
            </h4>
          </div>
          <div class="flex-1">
            <input
              v-model="store.generalSettings.customApiKey"
              type="password"
              :readonly="!customDraftOpen"
              :disabled="!customDraftOpen"
              placeholder="sk-xxxxxxxxxxxxxxxx"
              class="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 outline-none placeholder:text-gray-400 focus:border-gray-400 disabled:cursor-not-allowed disabled:opacity-60 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:placeholder:text-gray-500"
            />
            <p
              class="mt-1 flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400"
            >
              服务器代理模式
              <span
                title="API Key 将会加密保存到服务器"
                class="inline-flex shrink-0 cursor-help"
              >
                <Info class="size-3 text-gray-400" />
              </span>
            </p>
          </div>
        </div>
        <!-- Test Connection (only in draft mode) -->
        <div class="flex items-center gap-4">
          <div class="w-24 shrink-0" />
          <span v-if="testStatus === 'success'" class="text-sm text-green-500"
            >连接成功</span
          >
          <span v-if="testStatus === 'error'" class="text-sm text-red-500">{{
            testMessage
          }}</span>
          <div class="flex-1" />
          <button
            v-if="customDraftOpen"
            class="shrink-0 rounded-lg border border-gray-200 bg-white px-4 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
            :disabled="testStatus === 'testing'"
            @click="testConnection"
          >
            <span
              v-if="testStatus === 'testing'"
              class="flex items-center gap-2"
              >测试中...</span
            >
            <span v-else>测试连接</span>
          </button>
          <button
            v-if="customDraftOpen"
            class="shrink-0 rounded-lg bg-gray-900 px-4 py-2 text-sm text-white transition-colors hover:bg-gray-800 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
            :disabled="store.isModelCreating || !hasPassedConnectionTest"
            @click="saveCustomModel"
          >
            {{ store.isModelCreating ? "保存中..." : "保存并启用" }}
          </button>
          <button
            v-if="!customDraftOpen && store.isCustomModelSelected"
            class="shrink-0 rounded-lg border border-red-200 px-4 py-2 text-sm text-red-600 transition-colors hover:bg-red-50 dark:border-red-900/60 dark:text-red-400 dark:hover:bg-red-950/30"
            @click="deleteModelDialogOpen = true"
          >
            <span class="inline-flex items-center gap-2">
              <Trash2 class="size-4" />
              删除自定义模型
            </span>
          </button>
        </div>
      </div>
    </div>
    <div
      v-if="deleteModelDialogOpen"
      class="fixed inset-0 z-[70] flex items-center justify-center bg-black/30 px-4"
      @click.self="deleteModelDialogOpen = false"
    >
      <div
        class="w-full max-w-sm rounded-xl bg-white p-5 shadow-xl dark:bg-gray-900"
      >
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">
          确认删除
        </h3>
        <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-gray-300">
          确定删除当前自定义模型吗？删除后会切换到可用模型。
        </p>
        <div class="mt-5 flex justify-end gap-2">
          <button
            class="rounded-lg border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
            @click="deleteModelDialogOpen = false"
          >
            取消
          </button>
          <button
            class="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700"
            @click="deleteSelectedCustomModel"
          >
            删除
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
