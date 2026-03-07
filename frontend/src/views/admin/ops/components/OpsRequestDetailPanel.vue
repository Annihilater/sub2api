<template>
  <div class="h-full">
    <!-- 空状态 -->
    <div
      v-if="!row"
      class="flex min-h-[240px] items-center justify-center px-6 py-10 text-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ emptyText || t('admin.ops.requestDetails.detailPaneEmpty') }}
    </div>

    <!-- 详情内容 -->
    <div v-else class="space-y-5 p-6">
      <!-- Request ID -->
      <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900 col-span-2">
        <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.requestId') }}</div>
        <div class="mt-1 flex items-center gap-2">
          <span class="flex-1 truncate font-mono text-sm font-medium text-gray-900 dark:text-white" :title="row.request_id || '—'">
            {{ row.request_id || '—' }}
          </span>
          <button
            v-if="row.request_id"
            class="flex-shrink-0 rounded-md bg-gray-100 px-2 py-1 text-[10px] font-bold text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600"
            @click="handleCopy(row.request_id)"
          >
            {{ t('admin.ops.requestDetails.copy') }}
          </button>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <!-- Time -->
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.time') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ formatDateTime(row.created_at) }}
          </div>
        </div>

        <!-- Kind -->
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.kind') }}</div>
          <div class="mt-1">
            <span class="rounded-full px-2 py-1 text-[10px] font-bold" :class="kindBadgeClass(row.kind)">
              {{ row.kind === 'error' ? t('admin.ops.requestDetails.kind.error') : t('admin.ops.requestDetails.kind.success') }}
            </span>
          </div>
        </div>

        <!-- Platform -->
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.platform') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ (row.platform || '—').toUpperCase() }}
          </div>
        </div>

        <!-- Model -->
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.model') }}</div>
          <div class="mt-1 truncate text-sm font-medium text-gray-900 dark:text-white" :title="row.model || '—'">
            {{ row.model || '—' }}
          </div>
        </div>

        <!-- Duration -->
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.duration') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ typeof row.duration_ms === 'number' ? `${row.duration_ms} ms` : '—' }}
          </div>
        </div>

        <!-- Status -->
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.status') }}</div>
          <div class="mt-1">
            <span
              v-if="row.status_code != null"
              :class="['inline-flex items-center rounded-lg px-2 py-1 text-xs font-black ring-1 ring-inset shadow-sm', statusClass]"
            >
              {{ row.status_code }}
            </span>
            <span v-else class="text-sm font-medium text-gray-400">—</span>
          </div>
        </div>

        <!-- Stream -->
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.detail.stream') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ row.stream != null ? (row.stream ? t('common.yes') : t('common.no')) : '—' }}
          </div>
        </div>

        <!-- Group -->
        <div v-if="row.group_id != null" class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.detail.group') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ row.group_id }}
          </div>
        </div>

        <!-- User -->
        <div v-if="row.user_id != null" class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.detail.user') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
            {{ row.user_id }}
          </div>
        </div>
      </div>

      <!-- Error section (only when kind === 'error') -->
      <div v-if="row.kind === 'error'" class="rounded-xl bg-red-50/60 p-5 dark:bg-red-950/20">
        <h3 class="text-sm font-black uppercase tracking-wider text-red-800 dark:text-red-300">{{ t('admin.ops.requestDetails.detail.errorInfo') }}</h3>
        <div class="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div v-if="row.phase" class="rounded-lg bg-white p-3 dark:bg-dark-800">
            <div class="text-[11px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.detail.phase') }}</div>
            <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ row.phase }}</div>
          </div>
          <div v-if="row.severity" class="rounded-lg bg-white p-3 dark:bg-dark-800">
            <div class="text-[11px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.detail.severity') }}</div>
            <div class="mt-1"><span :class="['rounded px-1.5 py-0.5 text-[10px] font-bold', getSeverityClass(row.severity as any)]">{{ row.severity }}</span></div>
          </div>
          <div v-if="row.message" class="sm:col-span-2 rounded-lg bg-white p-3 dark:bg-dark-800">
            <div class="text-[11px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.detail.message') }}</div>
            <div class="mt-1 break-words text-sm font-medium text-gray-900 dark:text-white">{{ row.message }}</div>
          </div>
        </div>
        <div v-if="row.error_id" class="mt-4">
          <button
            class="rounded-lg bg-red-100 px-4 py-2 text-xs font-bold text-red-700 hover:bg-red-200 dark:bg-red-900/30 dark:text-red-300 dark:hover:bg-red-900/50"
            @click="emit('openErrorDetail', row.error_id!)"
          >
            {{ t('admin.ops.requestDetails.viewError') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores'
import type { OpsRequestDetail } from '@/api/admin/ops'
import { getSeverityClass, formatDateTime } from '../utils/opsFormatters'

interface Props {
  row: OpsRequestDetail | null
  emptyText?: string
}

interface Emits {
  (e: 'openErrorDetail', errorId: number): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

async function handleCopy(text: string) {
  const ok = await copyToClipboard(text, t('admin.ops.requestDetails.requestIdCopied'))
  if (!ok) appStore.showWarning(t('admin.ops.requestDetails.copyFailed'))
}

function kindBadgeClass(kind: string) {
  if (kind === 'error') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
}

const statusClass = computed(() => {
  const code = props.row?.status_code ?? 0
  if (code >= 500) return 'bg-red-50 text-red-700 ring-red-600/20 dark:bg-red-900/30 dark:text-red-400 dark:ring-red-500/30'
  if (code === 429) return 'bg-purple-50 text-purple-700 ring-purple-600/20 dark:bg-purple-900/30 dark:text-purple-400 dark:ring-purple-500/30'
  if (code >= 400) return 'bg-amber-50 text-amber-700 ring-amber-600/20 dark:bg-amber-900/30 dark:text-amber-400 dark:ring-amber-500/30'
  if (code >= 200) return 'bg-green-50 text-green-700 ring-green-600/20 dark:bg-green-900/30 dark:text-green-400 dark:ring-green-500/30'
  return 'bg-gray-50 text-gray-700 ring-gray-600/20 dark:bg-gray-900/30 dark:text-gray-400 dark:ring-gray-500/30'
})
</script>
