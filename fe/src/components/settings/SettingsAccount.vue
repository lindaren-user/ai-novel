<script setup lang="ts">
import { computed, ref } from 'vue'
import { AlertTriangle, ChevronDown, Download, LogOut, X } from 'lucide-vue-next'
import { useAppStore } from '@/stores/app'
import { changePasswordApi } from '@/services/api'
import { LANGUAGE_OPTIONS } from '@/constants'

const store = useAppStore()

const langOpen = ref(false)

// 修改密码弹窗
const passwordDialogOpen = ref(false)
const passwordOld = ref('')
const passwordNew = ref('')
const passwordConfirm = ref('')
const passwordLoading = ref(false)
const deleteDialogOpen = ref(false)
const deletePassword = ref('')
const deleteLoading = ref(false)

const passwordMismatch = computed(() =>
  passwordConfirm.value.length > 0 && passwordNew.value !== passwordConfirm.value
)

const passwordSubmitDisabled = computed(() =>
  passwordLoading.value || passwordMismatch.value
)

function openPasswordDialog() {
  passwordOld.value = ''
  passwordNew.value = ''
  passwordConfirm.value = ''
  passwordDialogOpen.value = true
}

async function handleChangePassword() {
  if (passwordNew.value.length < 6) {
    store.notifyError('新密码至少6位')
    return
  }
  if (passwordNew.value !== passwordConfirm.value) {
    return
  }
  passwordLoading.value = true
  try {
    await changePasswordApi(passwordOld.value, passwordNew.value)
    store.notifyInfo('密码修改成功')
    passwordDialogOpen.value = false
  } catch (err) {
    store.notifyError(err instanceof Error ? err.message : '密码修改失败')
  } finally {
    passwordLoading.value = false
  }
}

function openDeleteDialog() {
  deletePassword.value = ''
  deleteDialogOpen.value = true
}

async function handleDeleteAccount() {
  if (!deletePassword.value) {
    store.notifyError('请输入当前密码')
    return
  }
  deleteLoading.value = true
  try {
    await store.deleteAccount(deletePassword.value)
    deleteDialogOpen.value = false
  } catch {
    // 错误提示由 store.deleteAccount 统一处理。
  } finally {
    deleteLoading.value = false
  }
}
</script>

<template>
  <div class="space-y-8">
    <div class="flex items-center gap-4">
      <div class="flex size-16 shrink-0 items-center justify-center rounded-full bg-gray-900 text-xl font-medium text-white dark:bg-gray-100 dark:text-gray-900">
        {{ (store.accountSettings.username || store.accountSettings.email || '?').charAt(0) }}
      </div>
      <div class="min-w-0 flex-1">
        <h4 class="font-medium text-gray-900 dark:text-white">{{ store.accountSettings.username || '未设置用户名' }}</h4>
        <div class="mt-0.5 flex items-center justify-between gap-3">
          <p class="min-w-0 flex-1 truncate text-sm text-gray-500 dark:text-gray-400">{{ store.accountSettings.email || '未绑定邮箱' }}</p>
          <button
            class="shrink-0 text-xs text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
            @click="openPasswordDialog"
          >
            修改密码
          </button>
        </div>
      </div>
    </div>

    <div class="flex items-start justify-between">
      <div>
        <h4 class="font-medium text-gray-900 dark:text-white">账户语言</h4>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">账户相关邮件和通知的语言</p>
      </div>
      <div class="relative">
        <button
          class="flex w-32 items-center justify-between rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
          @click="langOpen = !langOpen"
        >
          {{ store.accountSettings.language }}
          <ChevronDown class="size-4 text-gray-400" />
        </button>
        <div
          v-if="langOpen"
          class="absolute right-0 z-10 mt-1 w-32 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
        >
          <button
            v-for="option in LANGUAGE_OPTIONS"
            :key="option"
            class="w-full px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
            :class="store.accountSettings.language === option ? 'bg-gray-100 dark:bg-gray-700' : ''"
            @click="store.accountSettings.language = option; langOpen = false"
          >
            {{ option }}
          </button>
        </div>
      </div>
    </div>

    <div class="flex items-start justify-between">
      <div>
        <h4 class="font-medium text-gray-900 dark:text-white">导出数据</h4>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">导出为压缩包，每本小说一个 {{ store.generalSettings.downloadFormat }} 文件</p>
      </div>
      <button class="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-4 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700" @click="store.exportAllData()">
        <Download class="size-4" />
        导出
      </button>
    </div>

    <div class="rounded-lg border border-red-200 bg-red-50 p-4 dark:border-red-900 dark:bg-red-950/30">
      <div class="flex items-center gap-2 text-red-600 dark:text-red-400">
        <AlertTriangle class="size-5" />
        <h4 class="font-medium">注销用户</h4>
      </div>
      <p class="mt-2 text-sm text-red-600/80 dark:text-red-300/80">注销后账号会进入 7 天冷静期，期间资产暂不删除；冷静期结束后再执行清理。</p>
      <button class="mt-4 w-full rounded-lg bg-red-600 px-4 py-2 text-sm text-white transition-colors hover:bg-red-700" @click="openDeleteDialog">
        注销用户
      </button>
    </div>

    <button
      class="flex w-full items-center justify-center gap-2 rounded-lg border border-gray-200 px-4 py-3 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
      @click="store.requestLogout()"
    >
      <LogOut class="size-4" />
      退出登录
    </button>
  </div>

  <!-- 修改密码弹窗 -->
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="passwordDialogOpen"
        class="fixed inset-0 z-[80] flex items-center justify-center"
      >
        <div class="absolute inset-0 z-0 bg-black/50" @click="passwordDialogOpen = false" />
        <div class="relative z-10 w-[400px] overflow-hidden rounded-xl bg-white shadow-2xl dark:bg-gray-900">
          <div class="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-800">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">修改密码</h3>
            <button class="rounded-lg p-2 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300" @click="passwordDialogOpen = false">
              <X class="size-5" />
            </button>
          </div>
          <div class="p-6 space-y-4">
            <div>
              <label class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">旧密码</label>
              <input v-model="passwordOld" type="password" class="w-full rounded-lg border border-gray-200 bg-white py-2.5 px-3 text-sm text-gray-700 outline-none focus:border-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300" placeholder="请输入旧密码" />
            </div>
            <div>
              <label class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">新密码</label>
              <input v-model="passwordNew" type="password" class="w-full rounded-lg border border-gray-200 bg-white py-2.5 px-3 text-sm text-gray-700 outline-none focus:border-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300" placeholder="至少6位" />
            </div>
            <div>
              <label class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">确认新密码</label>
              <input
                v-model="passwordConfirm"
                type="password"
                class="w-full rounded-lg border bg-white px-3 py-2.5 text-sm text-gray-700 outline-none transition-colors dark:bg-gray-800 dark:text-gray-300"
                :class="
                  passwordMismatch
                    ? 'border-red-400 focus:border-red-500 dark:border-red-500 dark:focus:border-red-400'
                    : 'border-gray-200 focus:border-gray-400 dark:border-gray-600 dark:focus:border-gray-500'
                "
                placeholder="再次输入新密码"
              />
              <p
                v-if="passwordMismatch"
                class="mt-1 text-xs text-red-500 dark:text-red-400"
              >
                两次输入的新密码不一致
              </p>
            </div>
            <div class="flex gap-3 pt-2">
              <button class="flex-1 rounded-lg border border-gray-200 px-4 py-2.5 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800" @click="passwordDialogOpen = false">取消</button>
              <button class="flex-1 rounded-lg bg-gray-900 px-4 py-2.5 text-sm font-medium text-white hover:bg-gray-800 disabled:cursor-not-allowed disabled:bg-gray-300 disabled:text-gray-500 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200 dark:disabled:bg-gray-700 dark:disabled:text-gray-400" :disabled="passwordSubmitDisabled" @click="handleChangePassword">{{ passwordLoading ? '修改中...' : '确认修改' }}</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>

  <!-- 注销账户弹窗 -->
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="deleteDialogOpen"
        class="fixed inset-0 z-[80] flex items-center justify-center"
      >
        <div class="absolute inset-0 z-0 bg-black/50" @click="deleteDialogOpen = false" />
        <div class="relative z-10 w-[400px] overflow-hidden rounded-xl bg-white shadow-2xl dark:bg-gray-900">
          <div class="flex items-center justify-between border-b border-red-100 px-6 py-4 dark:border-red-900/60">
            <h3 class="text-lg font-semibold text-red-600 dark:text-red-300">注销用户</h3>
            <button class="rounded-lg p-2 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-300" @click="deleteDialogOpen = false">
              <X class="size-5" />
            </button>
          </div>
          <div class="space-y-4 p-6">
            <p class="text-sm leading-6 text-gray-600 dark:text-gray-300">
              注销后账号会进入 7 天冷静期，期间小说、正文、对话和模型暂不删除；冷静期结束后再执行清理。请输入当前密码确认。
            </p>
            <div>
              <label class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">当前密码</label>
              <input
                v-model="deletePassword"
                type="password"
                class="w-full rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-700 outline-none focus:border-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300"
                placeholder="请输入当前密码"
                @keydown.enter="handleDeleteAccount"
              />
            </div>
            <div class="flex gap-3 pt-2">
              <button class="flex-1 rounded-lg border border-gray-200 px-4 py-2.5 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800" @click="deleteDialogOpen = false">取消</button>
              <button class="flex-1 rounded-lg bg-red-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-60" :disabled="deleteLoading" @click="handleDeleteAccount">{{ deleteLoading ? '注销中...' : '确认注销' }}</button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>

</template>
