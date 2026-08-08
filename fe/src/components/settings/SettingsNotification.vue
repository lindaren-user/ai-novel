<script setup lang="ts">
import { ref } from "vue";
import { ChevronDown } from "lucide-vue-next";
import { useAppStore } from "@/stores/app";

const store = useAppStore();

const timeOptions = Array.from({ length: 24 }, (_, i) => {
  const hour = i.toString().padStart(2, "0");
  return `${hour}:00`;
});

const startOpen = ref(false);
const endOpen = ref(false);
</script>

<template>
  <div class="space-y-8">
    <div class="flex items-start justify-between">
      <div>
        <h4 class="font-medium text-gray-900 dark:text-white">消息通知</h4>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          接收消息，及时获取重要信息
        </p>
      </div>
      <button
        class="relative h-6 w-11 rounded-full transition-colors"
        :class="
          store.notificationSettings.desktopNotification
            ? 'bg-gray-900 dark:bg-white'
            : 'bg-gray-200 dark:bg-gray-700'
        "
        @click="
          store.notificationSettings.desktopNotification =
            !store.notificationSettings.desktopNotification
        "
      >
        <span
          class="absolute left-0.5 top-0.5 size-5 rounded-full shadow transition-all"
          :class="
            store.notificationSettings.desktopNotification
              ? 'translate-x-5 bg-white dark:bg-gray-900'
              : 'translate-x-0 bg-white'
          "
        />
      </button>
    </div>

    <div>
      <div class="flex items-start justify-between">
        <div>
          <h4 class="font-medium text-gray-900 dark:text-white">免打扰时段</h4>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            在设定的时间段内不接收通知
          </p>
        </div>
        <button
          class="relative h-6 w-11 rounded-full transition-colors"
          :class="
            store.notificationSettings.doNotDisturb
              ? 'bg-gray-900 dark:bg-white'
              : 'bg-gray-200 dark:bg-gray-700'
          "
          @click="
            store.notificationSettings.doNotDisturb =
              !store.notificationSettings.doNotDisturb
          "
        >
          <span
            class="absolute left-0.5 top-0.5 size-5 rounded-full shadow transition-all"
            :class="
              store.notificationSettings.doNotDisturb
                ? 'translate-x-5 bg-white dark:bg-gray-900'
                : 'translate-x-0 bg-white'
            "
          />
        </button>
      </div>

      <div
        v-if="store.notificationSettings.doNotDisturb"
        class="mt-4 grid grid-cols-2 gap-4"
      >
        <div>
          <label class="mb-1 block text-sm text-gray-500 dark:text-gray-400"
            >开始时间</label
          >
          <div class="relative">
            <button
              class="flex w-full items-center justify-between rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
              @click="startOpen = !startOpen"
            >
              {{ store.notificationSettings.doNotDisturbStart }}
              <ChevronDown class="size-4 text-gray-400" />
            </button>
            <div
              v-if="startOpen"
              class="absolute right-0 z-10 mt-1 max-h-48 w-24 overflow-y-auto rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
            >
              <button
                v-for="time in timeOptions"
                :key="time"
                class="w-full px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                :class="
                  store.notificationSettings.doNotDisturbStart === time
                    ? 'bg-gray-100 dark:bg-gray-700'
                    : ''
                "
                @click="
                  store.notificationSettings.doNotDisturbStart = time;
                  startOpen = false;
                "
              >
                {{ time }}
              </button>
            </div>
          </div>
        </div>
        <div>
          <label class="mb-1 block text-sm text-gray-500 dark:text-gray-400"
            >结束时间</label
          >
          <div class="relative">
            <button
              class="flex w-full items-center justify-between rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
              @click="endOpen = !endOpen"
            >
              {{ store.notificationSettings.doNotDisturbEnd }}
              <ChevronDown class="size-4 text-gray-400" />
            </button>
            <div
              v-if="endOpen"
              class="absolute right-0 z-10 mt-1 max-h-48 w-24 overflow-y-auto rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800"
            >
              <button
                v-for="time in timeOptions"
                :key="time"
                class="w-full px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700"
                :class="
                  store.notificationSettings.doNotDisturbEnd === time
                    ? 'bg-gray-100 dark:bg-gray-700'
                    : ''
                "
                @click="
                  store.notificationSettings.doNotDisturbEnd = time;
                  endOpen = false;
                "
              >
                {{ time }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
