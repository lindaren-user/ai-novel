<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { X, User, Lock, Eye, EyeOff, Mail } from "lucide-vue-next";
import { useAppStore } from "@/stores/app";
import { sendCodeApi, getTurnstileConfigApi, type TurnstileConfig } from "@/services/api";

declare global {
  interface Window {
    turnstile?: {
      render: (element: HTMLElement, options: Record<string, unknown>) => string;
      reset: (widgetId?: string) => void;
      remove: (widgetId: string) => void;
    };
  }
}

let turnstileScriptPromise: Promise<void> | null = null;

const router = useRouter();
const store = useAppStore();

const props = defineProps<{
  open: boolean;
  mode: "login" | "register";
}>();

const emit = defineEmits<{
  close: [];
  "update:mode": [mode: "login" | "register"];
}>();

// 表单字段
const username = ref("");
const email = ref("");
const password = ref("");
const confirmPassword = ref("");
const code = ref("");
const showPassword = ref(false);
const loading = ref(false);

// 登录子模式：password | code
const loginSubMode = ref<"password" | "code">("password");

const turnstileConfig = ref<TurnstileConfig | null>(null);
const turnstileToken = ref("");
const turnstileLoading = ref(false);
const turnstileEl = ref<HTMLElement | null>(null);
const turnstileWidgetId = ref<string | null>(null);

// 验证码倒计时
const codeCountdown = ref(0);
let countdownTimer: ReturnType<typeof setTimeout> | null = null;

const submitLabel = computed(() => {
  if (loading.value) return "处理中...";
  return props.mode === "login" ? "登录" : "注册";
});

const passwordMismatch = computed(() => {
  return (
    props.mode === "register" &&
    confirmPassword.value.length > 0 &&
    password.value !== confirmPassword.value
  );
});

const showTurnstile = computed(() => {
  return props.mode === "login" || props.mode === "register";
});

watch(
  () => props.open,
  (val) => {
    if (val) {
      username.value = "";
      email.value = "";
      password.value = "";
      confirmPassword.value = "";
      code.value = "";
      resetTurnstileState();
      loading.value = false;
      loginSubMode.value = "password";
      clearCountdown();
      void ensureTurnstile();
    } else {
      resetTurnstileState();
    }
  }
);

watch(() => props.mode, () => {
  resetTurnstileState();
  void ensureTurnstile();
});

function clearCountdown() {
  codeCountdown.value = 0;
  if (countdownTimer) {
    clearTimeout(countdownTimer);
    countdownTimer = null;
  }
}

function tickCountdown() {
  codeCountdown.value--;
  if (codeCountdown.value > 0) {
    countdownTimer = setTimeout(tickCountdown, 1000);
  }
}

function startCountdown() {
  clearCountdown();
  codeCountdown.value = 60;
  countdownTimer = setTimeout(tickCountdown, 1000);
}

function switchMode() {
  emit("update:mode", props.mode === "login" ? "register" : "login");
  resetTurnstileState();
}

function switchLoginSubMode(sub: "password" | "code") {
  loginSubMode.value = sub;
}

async function handleSendCode() {
  if (!email.value.trim()) {
    store.notifyError("请先输入邮箱");
    return;
  }
  if (!email.value.includes("@")) {
    store.notifyError("请输入正确的邮箱地址");
    return;
  }
  if (codeCountdown.value > 0) return;

  try {
    await sendCodeApi(email.value.trim());
    startCountdown();
    store.notifyInfo("验证码已发送");
  } catch (err) {
    store.notifyError(err instanceof Error ? err.message : "验证码发送失败");
  }
}

// 校验用户名：不能为空，仅限字母数字，最多64位
function validateUsername(val: string): string | null {
  const trimmed = val.trim();
  if (!trimmed) return "请输入用户名";
  if (!/^[a-zA-Z0-9]+$/.test(trimmed)) return "用户名只能包含字母和数字";
  if (trimmed.length > 64) return "用户名不能超过64位";
  return null;
}

function validatePasswordStrength(val: string): string | null {
  if (val.length < 8) return "密码长度至少 8 位";
  if (!/[A-Za-z]/.test(val)) return "密码必须包含字母";
  if (!/\d/.test(val)) return "密码必须包含数字";
  return null;
}

async function handleSubmit() {
  if (loading.value) return;

  if (props.mode === "register") {
    const userErr = validateUsername(username.value);
    if (userErr) {
      store.notifyError(userErr);
      return;
    }
    if (!email.value.trim() || !email.value.includes("@")) {
      store.notifyError("请输入正确的邮箱地址");
      return;
    }
    if (!code.value.trim()) {
      store.notifyError("请输入验证码");
      return;
    }
    const passwordErr = validatePasswordStrength(password.value);
    if (passwordErr) {
      store.notifyError(passwordErr);
      return;
    }
    if (!confirmPassword.value) {
      store.notifyError("请再次输入密码");
      return;
    }
    if (passwordMismatch.value) {
      store.notifyError("两次密码不一致");
      return;
    }
    if (turnstileConfig.value?.enabled && !turnstileToken.value) {
      store.notifyError("请完成人机验证");
      return;
    }

    try {
      loading.value = true;
      await store.register(
        username.value.trim(),
        email.value.trim(),
        password.value,
        code.value.trim(),
        turnstileToken.value
      );
      emit("close");
      router.push("/workspace");
    } catch (err) {
      store.notifyError(err instanceof Error ? err.message : "注册失败");
      resetTurnstileWidget();
    } finally {
      loading.value = false;
    }
  } else {
    if (loginSubMode.value === "password") {
      const account = username.value.trim();
      if (!account) {
        store.notifyError("请输入邮箱或用户名");
        return;
      }
      if (!password.value) {
        store.notifyError("请输入密码");
        return;
      }
      if (turnstileConfig.value?.enabled && !turnstileToken.value) {
        store.notifyError("请完成人机验证");
        return;
      }

      try {
        loading.value = true;
        await store.login(
          account,
          password.value,
          turnstileToken.value
        );
        emit("close");
        router.push("/workspace");
      } catch (err) {
        store.notifyError(err instanceof Error ? err.message : "登录失败");
        resetTurnstileWidget();
      } finally {
        loading.value = false;
      }
    } else {
      if (!email.value.trim() || !email.value.includes("@")) {
        store.notifyError("请输入正确的邮箱地址");
        return;
      }
      if (!code.value.trim()) {
        store.notifyError("请输入验证码");
        return;
      }
      if (turnstileConfig.value?.enabled && !turnstileToken.value) {
        store.notifyError("请完成人机验证");
        return;
      }

      try {
        loading.value = true;
        await store.loginByCode(email.value.trim(), code.value.trim(), turnstileToken.value);
        emit("close");
        router.push("/workspace");
      } catch (err) {
        store.notifyError(err instanceof Error ? err.message : "登录失败");
        resetTurnstileWidget();
      } finally {
        loading.value = false;
      }
    }
  }
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === "Enter") handleSubmit();
}

async function ensureTurnstile() {
  if (!showTurnstile.value || turnstileWidgetId.value) return;
  turnstileLoading.value = true;
  try {
    if (!turnstileConfig.value) {
      turnstileConfig.value = await getTurnstileConfigApi();
    }
    if (!turnstileConfig.value.enabled || !turnstileConfig.value.siteKey) return;
    await loadTurnstileScript();
    await nextTick();
    renderTurnstile();
  } catch (err) {
    store.notifyError(err instanceof Error ? err.message : "加载人机验证失败");
  } finally {
    turnstileLoading.value = false;
  }
}

function renderTurnstile() {
  if (!turnstileEl.value || !window.turnstile || !turnstileConfig.value?.siteKey || turnstileWidgetId.value) return;
  turnstileWidgetId.value = window.turnstile.render(turnstileEl.value, {
    sitekey: turnstileConfig.value.siteKey,
    theme: "auto",
    size: "flexible",
    callback: (token: string) => {
      turnstileToken.value = token;
    },
    "expired-callback": () => {
      turnstileToken.value = "";
    },
    "error-callback": () => {
      turnstileToken.value = "";
    },
  });
}

function loadTurnstileScript() {
  if (window.turnstile) return Promise.resolve();
  if (turnstileScriptPromise) return turnstileScriptPromise;
  turnstileScriptPromise = new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
    script.async = true;
    script.defer = true;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("加载 Turnstile 失败"));
    document.head.appendChild(script);
  });
  return turnstileScriptPromise;
}

function resetTurnstileWidget() {
  turnstileToken.value = "";
  if (turnstileWidgetId.value && window.turnstile) {
    window.turnstile.reset(turnstileWidgetId.value);
  }
}

function resetTurnstileState() {
  turnstileToken.value = "";
  if (turnstileWidgetId.value && window.turnstile) {
    window.turnstile.remove(turnstileWidgetId.value);
  }
  turnstileWidgetId.value = null;
}

onBeforeUnmount(() => {
  resetTurnstileState();
  clearCountdown();
});
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="open"
        class="fixed inset-0 z-50 flex items-center justify-center"
      >
        <div
          class="absolute inset-0 z-0 cursor-pointer bg-black/50"
          @click="emit('close')"
        />
        <div
          class="relative z-10 w-[420px] overflow-hidden rounded-xl bg-white shadow-2xl dark:bg-gray-900"
        >
          <!-- Header -->
          <div
            class="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-800"
          >
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ mode === "login" ? "登录" : "注册" }}
            </h3>
            <button
              class="cursor-pointer rounded-lg p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300"
              @click="emit('close')"
            >
              <X class="size-5" />
            </button>
          </div>

          <!-- Body -->
          <div class="p-6">
            <!-- 登录子模式切换 -->
            <div
              v-if="mode === 'login'"
              class="mb-4 flex rounded-lg bg-gray-100 p-1 dark:bg-gray-800"
            >
              <button
                class="flex-1 cursor-pointer rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                :class="
                  loginSubMode === 'password'
                    ? 'bg-white text-gray-900 shadow-sm dark:bg-gray-700 dark:text-white'
                    : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
                "
                @click="switchLoginSubMode('password')"
              >
                密码登录
              </button>
              <button
                class="flex-1 cursor-pointer rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                :class="
                  loginSubMode === 'code'
                    ? 'bg-white text-gray-900 shadow-sm dark:bg-gray-700 dark:text-white'
                    : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'
                "
                @click="switchLoginSubMode('code')"
              >
                验证码登录
              </button>
            </div>

            <div class="space-y-4">
              <!-- 用户名 (注册 + 密码登录) -->
              <div
                v-if="
                  mode === 'register' ||
                  (mode === 'login' && loginSubMode === 'password')
                "
              >
                <label
                  class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  {{ mode === "login" ? "邮箱 / 用户名" : "用户名" }}
                </label>
                <div class="relative">
                  <User
                    class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                  />
                  <input
                    v-model="username"
                    type="text"
                    class="w-full rounded-lg border border-gray-200 bg-white py-2.5 pl-10 pr-3 text-sm text-gray-700 outline-none transition-colors focus:border-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:focus:border-gray-500"
                    :placeholder="
                      mode === 'login'
                        ? '请输入邮箱或用户名'
                        : '请输入用户名（仅限字母和数字）'
                    "
                    @keydown="handleKeydown"
                  />
                </div>
              </div>

              <!-- 邮箱 (注册 + 验证码登录) -->
              <div
                v-if="
                  mode === 'register' ||
                  (mode === 'login' && loginSubMode === 'code')
                "
              >
                <label
                  class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  邮箱
                </label>
                <div class="relative">
                  <Mail
                    class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                  />
                  <input
                    v-model="email"
                    type="email"
                    class="w-full rounded-lg border border-gray-200 bg-white py-2.5 pl-10 pr-3 text-sm text-gray-700 outline-none transition-colors focus:border-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:focus:border-gray-500"
                    placeholder="请输入邮箱"
                    @keydown="handleKeydown"
                  />
                </div>
              </div>

              <!-- 密码 (注册 + 密码登录) -->
              <div
                v-if="
                  mode === 'register' ||
                  (mode === 'login' && loginSubMode === 'password')
                "
              >
                <label
                  class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  密码
                </label>
                <div class="relative">
                  <Lock
                    class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                  />
                  <input
                    v-model="password"
                    :type="showPassword ? 'text' : 'password'"
                    class="w-full rounded-lg border border-gray-200 bg-white py-2.5 pl-10 pr-10 text-sm text-gray-700 outline-none transition-colors focus:border-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:focus:border-gray-500"
                    placeholder="请输入密码"
                    @keydown="handleKeydown"
                  />
                  <button
                    type="button"
                    class="absolute right-3 top-1/2 -translate-y-1/2 cursor-pointer text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                    @click="showPassword = !showPassword"
                  >
                    <EyeOff v-if="showPassword" class="h-4 w-4" />
                    <Eye v-else class="h-4 w-4" />
                  </button>
                </div>
              </div>

              <!-- 确认密码 (注册) -->
              <div v-if="mode === 'register'">
                <label
                  class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  确认密码
                </label>
                <div class="relative">
                  <Lock
                    class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                  />
                  <input
                    v-model="confirmPassword"
                    :type="showPassword ? 'text' : 'password'"
                    class="w-full rounded-lg border bg-white py-2.5 pl-10 pr-3 text-sm text-gray-700 outline-none transition-colors dark:bg-gray-800 dark:text-gray-300"
                    :class="
                      passwordMismatch
                        ? 'border-red-400 focus:border-red-500 dark:border-red-500 dark:focus:border-red-400'
                        : 'border-gray-200 focus:border-gray-400 dark:border-gray-600 dark:focus:border-gray-500'
                    "
                    placeholder="请再次输入密码"
                    @keydown="handleKeydown"
                  />
                </div>
                <p
                  v-if="passwordMismatch"
                  class="mt-1 text-xs text-red-500 dark:text-red-400"
                >
                  两次输入的密码不一致
                </p>
              </div>

              <!-- 验证码 (注册 + 验证码登录) -->
              <div
                v-if="
                  mode === 'register' ||
                  (mode === 'login' && loginSubMode === 'code')
                "
              >
                <label
                  class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300"
                >
                  验证码
                </label>
                <div class="flex gap-2">
                  <input
                    v-model="code"
                    type="text"
                    maxlength="6"
                    class="flex-1 rounded-lg border border-gray-200 bg-white py-2.5 px-3 text-sm text-gray-700 outline-none transition-colors focus:border-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:focus:border-gray-500"
                    placeholder="请输入6位验证码"
                    @keydown="handleKeydown"
                  />
                  <button
                    type="button"
                    class="shrink-0 cursor-pointer rounded-lg px-3 py-2.5 text-sm font-medium transition-colors"
                    :class="
                      codeCountdown > 0
                        ? 'bg-gray-200 text-gray-400 cursor-not-allowed dark:bg-gray-700 dark:text-gray-500'
                        : 'bg-gray-900 text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200'
                    "
                    :disabled="codeCountdown > 0"
                    @click="handleSendCode"
                  >
                    {{ codeCountdown > 0 ? `${codeCountdown}s` : "发送验证码" }}
                  </button>
                </div>
              </div>

              <div
                v-if="showTurnstile && turnstileConfig?.enabled"
              >
                <div
                  ref="turnstileEl"
                  class="min-h-[65px] w-full"
                />
                <p
                  v-if="turnstileLoading"
                  class="mt-1 text-xs text-gray-500 dark:text-gray-400"
                >
                  正在加载验证...
                </p>
              </div>
            </div>

            <!-- Submit -->
            <button
              class="cursor-pointer mt-6 w-full rounded-lg bg-gray-900 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-gray-800 active:scale-[0.99] dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
              :disabled="loading"
              @click="handleSubmit"
            >
              {{ submitLabel }}
            </button>

            <!-- Switch mode -->
            <p
              class="mt-4 text-center text-sm text-gray-500 dark:text-gray-400"
            >
              {{ mode === "login" ? "还没有账号？" : "已有账号？" }}
              <button
                class="cursor-pointer font-medium text-gray-900 underline underline-offset-4 hover:text-gray-700 dark:text-white dark:hover:text-gray-300"
                @click="switchMode"
              >
                {{ mode === "login" ? "立即注册" : "去登录" }}
              </button>
            </p>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
input[type="password"]::-ms-reveal,
input[type="password"]::-ms-clear {
  display: none;
}
</style>
