<template>
  <div class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600">
    <div class="flex items-start justify-between gap-4">
      <div class="min-w-0">
        <p class="text-sm font-medium text-gray-900 dark:text-gray-100">
          {{ t('admin.accounts.openai.codexStateKitTitle') }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.codexStateKitDesc') }}
        </p>
      </div>
      <button
        type="button"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
        data-testid="edit-codex-state-kit-enabled"
        @click="toggleEnabled"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            enabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>

    <div v-if="enabled" class="space-y-3">
      <div class="flex flex-wrap items-center gap-3">
        <label class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.codexStateKitBoundLen') }}
        </label>
        <select
          v-model="boundLen"
          class="input w-36 py-1 text-sm"
          data-testid="edit-codex-state-kit-bound-len"
          @change="saveBoundLen"
        >
          <option :value="0">{{ t('admin.accounts.openai.codexStateKitBoundAuto') }}</option>
          <option :value="292">292</option>
          <option :value="332">332</option>
        </select>
        <button
          type="button"
          class="rounded-md border border-gray-300 px-3 py-1 text-xs dark:border-dark-500"
          :disabled="busy"
          data-testid="edit-codex-state-kit-refresh"
          @click="refresh"
        >
          {{ t('admin.accounts.openai.codexStateKitRefresh') }}
        </button>
      </div>

      <p v-if="error" class="text-xs text-red-600">{{ error }}</p>
      <p v-if="status" class="text-xs text-gray-600 dark:text-gray-300">
        {{ t('admin.accounts.openai.codexStateKitStatus') }}:
        {{ status.status }}
        <span v-if="status.len"> · {{ status.len }} bytes</span>
        <span v-if="status.ageSecs != null"> · {{ status.ageSecs }}s</span>
        <span v-if="status.source"> · {{ status.source }}</span>
      </p>
      <div class="rounded-md bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
        <p class="font-medium text-gray-800 dark:text-gray-100">
          {{ t('admin.accounts.openai.codexStateKitUploadTitle') }}
        </p>
        <p class="mt-1">{{ t('admin.accounts.openai.codexStateKitUploadDesc') }}</p>
        <p class="mt-1 font-mono break-all">
          {{ t('admin.accounts.openai.codexStateKitUploadPath') }}:
          POST /v1/codex-state-kit/tokens
        </p>
      </div>
      <ul v-if="status?.models?.length" class="space-y-1 text-xs text-gray-600 dark:text-gray-300">
        <li v-for="model in status.models" :key="model.model">
          {{ model.model }} — {{ model.status }}
          <span v-if="model.len"> ({{ model.len }})</span>
        </li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  getCodexStateKit,
  refreshCodexStateKit,
  updateCodexStateKit,
  type CodexStateKitStatus
} from '@/api/admin/codexStateKit'

const props = defineProps<{
  accountId: number
  enabled: boolean
}>()

const emit = defineEmits<{
  'update:enabled': [value: boolean]
}>()

const { t } = useI18n()
const status = ref<CodexStateKitStatus | null>(null)
const busy = ref(false)
const error = ref('')
const boundLen = ref(0)

async function load() {
  error.value = ''
  try {
    status.value = await getCodexStateKit(props.accountId)
    boundLen.value = status.value.boundTokenLen === 292 || status.value.boundTokenLen === 332
      ? status.value.boundTokenLen
      : 0
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function toggleEnabled() {
  const next = !props.enabled
  emit('update:enabled', next)
  busy.value = true
  error.value = ''
  try {
    status.value = await updateCodexStateKit(props.accountId, { enabled: next })
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    busy.value = false
  }
}

async function saveBoundLen() {
  busy.value = true
  error.value = ''
  try {
    if (!boundLen.value) {
      status.value = await updateCodexStateKit(props.accountId, { clear_bound_token_len: true })
    } else {
      status.value = await updateCodexStateKit(props.accountId, { bound_token_len: boundLen.value })
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    busy.value = false
  }
}

async function refresh() {
  busy.value = true
  error.value = ''
  try {
    status.value = await refreshCodexStateKit(props.accountId)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    busy.value = false
  }
}

onMounted(() => {
  if (props.accountId) void load()
})

watch(
  () => props.accountId,
  (id) => {
    if (id) void load()
  }
)
</script>
