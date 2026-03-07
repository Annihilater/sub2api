<template>
  <div class="relative h-full">
    <!-- 空状态 -->
    <div
      v-if="!row"
      class="flex min-h-[240px] items-center justify-center px-6 py-10 text-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ emptyText || t('admin.ops.requestDetails.detailPaneEmpty') }}
    </div>

    <!-- 错误类型：直接内嵌错误详情面板 -->
    <OpsErrorDetailPanel
      v-else-if="row.kind === 'error' && row.error_id"
      :show="true"
      :error-id="row.error_id"
      error-type="request"
      class="h-full"
    />

    <!-- 成功类型 / 无 error_id：展示请求摘要 -->
    <div v-else class="space-y-4 p-6">
      <!-- Request ID -->
      <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
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
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.time') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ formatDateTime(row.created_at) }}</div>
        </div>
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.kind') }}</div>
          <div class="mt-1">
            <span class="rounded-full px-2 py-1 text-[10px] font-bold" :class="kindBadgeClass(row.kind)">
              {{ row.kind === 'error' ? t('admin.ops.requestDetails.kind.error') : t('admin.ops.requestDetails.kind.success') }}
            </span>
          </div>
        </div>
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.platform') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ (row.platform || '—').toUpperCase() }}</div>
        </div>
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.model') }}</div>
          <div class="mt-1 truncate text-sm font-medium text-gray-900 dark:text-white" :title="row.model || '—'">{{ row.model || '—' }}</div>
        </div>
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.duration') }}</div>
          <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">{{ typeof row.duration_ms === 'number' ? `${row.duration_ms} ms` : '—' }}</div>
        </div>
        <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900">
          <div class="text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.requestDetails.table.status') }}</div>
          <div class="mt-1">
            <span v-if="row.status_code != null" :class="['inline-flex items-center rounded-lg px-2 py-1 text-xs font-black ring-1 ring-inset shadow-sm', statusClass]">
              {{ row.status_code }}
            </span>
            <span v-else class="text-sm font-medium text-gray-400">—</span>
          </div>
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
import { formatDateTime } from '../utils/opsFormatters'
import OpsErrorDetailPanel from './OpsErrorDetailPanel.vue'

interface Props {
  row: OpsRequestDetail | null
  emptyText?: string
}

const props = defineProps<Props>()

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
