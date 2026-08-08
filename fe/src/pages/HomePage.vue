<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import {
  BookOpen,
  CheckCircle2,
  CircleCheck,
  FileText,
  Globe,
  Layers,
  LayoutDashboard,
  Lightbulb,
  LogOut,
  PenLine,
  PlayCircle,
  Sparkles,
  Users,
} from "lucide-vue-next";
import { useAppStore } from "@/stores/app";
import AuthModal from "@/components/auth/AuthModal.vue";
import homeImage from "@/assets/img/home.webp";
import aiNovelLogo from "@/assets/img/ai-novel.webp";
import descriptionImage from "@/assets/img/description.webp";

const router = useRouter();
const store = useAppStore();

const stats = [
  { icon: Sparkles, value: "4.2 亿+", label: "累计生成字数" },
  { icon: BookOpen, value: "180,000+", label: "创建小说" },
  { icon: Users, value: "360,000+", label: "角色设定" },
  { icon: CircleCheck, value: "98%", label: "用户满意度" },
];

const workflow = [
  { icon: Lightbulb, title: "设定", desc: "填写题材与方向" },
  { icon: Globe, title: "世界", desc: "生成世界观设定" },
  { icon: Users, title: "角色", desc: "完善人物关系" },
  { icon: Layers, title: "分卷", desc: "规划全书结构" },
  { icon: FileText, title: "章节", desc: "拆解章节大纲" },
  { icon: PenLine, title: "正文", desc: "生成并编辑草稿" },
  { icon: CheckCircle2, title: "导出", desc: "下载或分享成稿" },
];

const accountName = computed(
  () => store.authUsername || store.accountSettings.username || "未设置用户名"
);

const accountEmail = computed(
  () => store.accountSettings.email || "未绑定邮箱"
);

const avatarText = computed(() => {
  const name = accountName.value || accountEmail.value;
  return name.trim().charAt(0).toUpperCase() || "?";
});

function startCreating() {
  if (!store.isLoggedIn) {
    store.openAuthModal("login");
    return;
  }
  router.push("/workspace");
}

function openLogin() {
  store.openAuthModal("login");
}

function openRegister() {
  store.openAuthModal("register");
}

async function confirmLogout() {
  await store.confirmLogout();
}
</script>

<template>
  <div class="h-dvh overflow-hidden bg-[#f7f7f7] text-black">
    <header class="fixed left-0 right-0 top-0 z-50">
      <div
        class="mx-auto flex h-16 max-w-none items-center justify-between px-5 lg:px-10"
      >
        <button class="flex items-center gap-4" @click="router.push('/')">
          <img :src="aiNovelLogo" alt="AI Novel" class="h-14 w-auto object-contain" />
        </button>

        <div
          v-if="store.isLoggedIn"
          class="flex items-center gap-3 text-sm font-medium"
        >
          <button
            class="hidden items-center gap-1.5 transition-colors hover:text-gray-500 sm:inline-flex"
            @click="startCreating"
          >
            <LayoutDashboard class="size-4" />
            工作台
          </button>
          <span class="hidden h-4 w-px bg-gray-300 sm:block" />
          <div class="hidden min-w-0 text-right sm:block">
            <div class="max-w-40 truncate text-sm font-semibold text-black">
              {{ accountName }}
            </div>
            <div class="mt-0.5 max-w-48 truncate text-xs text-gray-500">
              {{ accountEmail }}
            </div>
          </div>
          <div
            class="flex size-10 items-center justify-center rounded-full bg-black text-sm font-semibold text-white shadow-lg shadow-black/10"
          >
            {{ avatarText }}
          </div>
          <button
            class="flex size-10 items-center justify-center rounded-lg text-gray-600 transition-colors hover:bg-white/70 hover:text-black"
            title="退出登录"
            @click="store.requestLogout()"
          >
            <LogOut class="size-5" />
          </button>
        </div>
        <div v-else class="flex items-center gap-6 text-sm font-medium">
          <span class="hidden h-4 w-px bg-gray-300 lg:block" />
          <button
            class="hidden transition-colors hover:text-gray-500 sm:block"
            @click="openLogin"
          >
            登录
          </button>
          <button
            class="rounded-lg bg-black px-5 py-2.5 text-white shadow-lg shadow-black/10 transition-colors hover:bg-gray-800"
            @click="openRegister"
          >
            注册
          </button>
        </div>
      </div>
    </header>

    <main class="relative h-[calc(100dvh-4rem)] overflow-hidden pt-16">
      <section class="relative isolate h-full overflow-hidden px-5 lg:px-8">
        <img
          :src="homeImage"
          alt=""
          fetchpriority="high"
          decoding="async"
          class="pointer-events-none absolute inset-x-0 -top-40 -z-20 h-[calc(100%+10rem)] w-full object-cover object-top"
        />
        <div
          class="pointer-events-none absolute inset-0 -z-10 bg-gradient-to-r from-[#f7f7f7]/92 via-[#f7f7f7]/50 to-transparent"
        />
        <div
          class="pointer-events-none absolute inset-x-0 bottom-0 -z-10 h-48 bg-gradient-to-t from-[#f7f7f7]/95 via-[#f7f7f7]/45 to-transparent"
        />

        <div
          class="mx-auto flex h-full w-full max-w-[1320px] flex-col justify-between pb-0 pt-10"
        >
          <div
            class="flex min-h-0 flex-1 items-center pb-4"
          >
            <div class="max-w-[480px]">
              <img
                :src="descriptionImage"
                alt="从一个想法，到一本完整的小说。AI 帮你构建世界、设计角色、规划情节、生成内容，你只需专注于讲述动人的故事。"
                fetchpriority="high"
                decoding="async"
                class="w-full max-w-[520px] object-contain object-left"
              />

              <div class="mt-7 flex flex-wrap gap-4">
                <button
                  class="inline-flex items-center gap-3 rounded-lg bg-black px-6 py-3 text-base font-semibold text-white shadow-xl shadow-black/10 transition-colors hover:bg-gray-800"
                  @click="startCreating"
                >
                  <PenLine class="size-5" />
                  开始创作
                </button>
                <button
                  class="inline-flex items-center gap-3 rounded-lg border border-gray-200 bg-white/80 px-6 py-3 text-base font-semibold text-black shadow-sm shadow-black/5 backdrop-blur transition-colors hover:bg-white"
                  @click="startCreating"
                >
                  <PlayCircle class="size-5" />
                  观看演示
                </button>
              </div>
            </div>

            <aside
              class="absolute right-[12%] top-[28%] hidden max-w-[190px] text-gray-600 lg:block"
            >
              <div class="text-5xl font-serif leading-none text-gray-400">
                “
              </div>
              <p class="mt-2 text-sm leading-7">
                故事的起点，<br />
                由你决定；<br />
                世界的延展，<br />
                由 AI 完成。
              </p>
              <div class="mt-5 h-px w-6 bg-gray-400" />
            </aside>
          </div>

          <div
            class="relative top-5 mt-10 rounded-2xl border border-gray-200 bg-white/78 px-6 py-6 shadow-xl shadow-black/5 backdrop-blur md:px-8"
          >
            <div class="grid gap-6 md:grid-cols-2 xl:grid-cols-4">
              <div
                v-for="item in stats"
                :key="item.label"
                class="flex items-center gap-5 xl:border-r xl:border-gray-200 xl:last:border-r-0"
              >
                <div
                  class="flex size-11 shrink-0 items-center justify-center rounded-2xl bg-gray-50 shadow-inner shadow-black/5"
                >
                  <component :is="item.icon" class="size-5" />
                </div>
                <div>
                  <div class="text-xl font-black leading-none">
                    {{ item.value }}
                  </div>
                  <div class="mt-2 text-xs text-gray-500">{{ item.label }}</div>
                </div>
              </div>
            </div>
          </div>

          <div
            class="relative top-5 mt-12 grid items-center gap-8 lg:grid-cols-[180px_1fr]"
          >
            <div class="hidden lg:block">
              <h2 class="text-lg font-bold">
                创作流程
              </h2>
              <p class="mt-4 text-sm leading-6 text-gray-500">
                AI 全程陪伴，创作流畅自然
              </p>
            </div>
            <div class="grid grid-cols-7 gap-4">
              <template v-for="(item, index) in workflow" :key="item.title">
                <div class="relative flex min-w-0 flex-col items-center">
                  <div
                    class="flex h-[112px] w-full max-w-[96px] flex-col items-center justify-center rounded-xl border border-gray-200 bg-white/76 px-2 py-3 text-center shadow-sm shadow-black/5 backdrop-blur"
                  >
                    <component :is="item.icon" class="size-6" />
                    <h3 class="mt-3 text-xs font-bold">{{ item.title }}</h3>
                    <p
                      class="mt-1 hidden text-[11px] leading-4 text-gray-500 xl:block"
                    >
                      {{ item.desc }}
                    </p>
                  </div>
                  <div class="mt-4 text-xs text-gray-500">
                    {{ String(index + 1).padStart(2, "0") }}
                  </div>
                  <span
                    v-if="index < workflow.length - 1"
                    class="absolute right-[-14px] top-10 hidden text-lg text-gray-400 lg:block"
                    >→</span
                  >
                </div>
              </template>
            </div>
          </div>
        </div>
      </section>
    </main>

    <AuthModal
      :open="store.isAuthModalOpen"
      :mode="store.authModalMode"
      @close="store.closeAuthModal()"
      @update:mode="store.setAuthModalMode($event)"
    />
    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="store.isLogoutConfirmOpen"
          class="fixed inset-0 z-[80] flex items-center justify-center"
        >
          <div
            class="absolute inset-0 bg-black/50"
            @click="store.cancelLogout()"
          />
          <div
            class="relative z-10 w-[360px] rounded-xl bg-white p-6 shadow-2xl"
          >
            <h3 class="text-lg font-semibold text-gray-900">
              确定退出登录吗？
            </h3>
            <div class="mt-6 flex justify-end gap-2">
              <button
                class="rounded-lg border border-gray-200 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
                @click="store.cancelLogout()"
              >
                取消
              </button>
              <button
                class="rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800"
                @click="confirmLogout"
              >
                确定退出
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
