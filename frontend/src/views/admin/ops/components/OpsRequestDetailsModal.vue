<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import OpsRequestDetailPanel from './OpsRequestDetailPanel.vue'
import { useAppStore } from '@/stores'
import { opsAPI, type OpsRequestDetailsParams, type OpsRequestDetail } from '@/api/admin/ops'
import { parseTimeRangeMinutes, formatDateTime } from '../utils/opsFormatters'
import { pushEscape } from '@/composables/useEscapeStack'

export interface OpsRequestDetailsPreset {
  title: string
  kind?: OpsRequestDetailsParams['kind']
  sort?: OpsRequestDetailsParams['sort']
  min_duration_ms?: number
  max_duration_ms?: number
}

interface Props {
  modelValue: boolean
  timeRange: string
  preset: OpsRequestDetailsPreset
  platform?: string
  groupId?: number | null
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'openErrorDetail', errorId: number): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const items = ref<OpsRequestDetail[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const selectedRow = ref<OpsRequestDetail | null>(null)

const close = () => emit('update:modelValue', false)

// 当 detail 面板有选中行时，压入高优先级 ESC：先关详情，再关 modal
let popDetailEscape: (() => void) | null = null

watch(
  () => [props.modelValue, selectedRow.value] as const,
  ([open, row]) => {
    popDetailEscape?.()
    popDetailEscape = null
    if (open && row != null) {
      popDetailEscape = pushEscape(() => {
        selectedRow.value = null
      })
    }
  }
)

onUnmounted(() => {
  popDetailEscape?.()
  popDetailEscape = null
})

function buildTimeParams(): Pick<OpsRequestDetailsParams, 'start_time' | 'end_time'> {
  const minutes = parseTimeRangeMinutes(props.timeRange)
  const endTime = new Date()
  const startTime = new Date(endTime.getTime() - minutes * 60 * 1000)
  return {
    start_time: startTime.toISOString(),
    end_time: endTime.toISOString()
  }
}

function syncSelectedRow() {
  if (selectedRow.value && items.value.some((r) => r.request_id === selectedRow.value!.request_id)) return
  selectedRow.value = items.value.length > 0 ? items.value[0] : null
}

const fetchData = async () => {
  if (!props.modelValue) return
  loading.value = true
  try {
    const params: OpsRequestDetailsParams = {
      ...buildTimeParams(),
      page: page.value,
      page_size: pageSize.value,
      kind: props.preset.kind ?? 'all',
      sort: props.preset.sort ?? 'created_at_desc'
    }

    const platform = (props.platform || '').trim()
    if (platform) params.platform = platform
    if (typeof props.groupId === 'number' && props.groupId > 0) params.group_id = props.groupId

    if (typeof props.preset.min_duration_ms === 'number') params.min_duration_ms = props.preset.min_duration_ms
    if (typeof props.preset.max_duration_ms === 'number') params.max_duration_ms = props.preset.max_duration_ms

    const res = await opsAPI.listRequestDetails(params)
    items.value = res.items || []
    total.value = res.total || 0
    syncSelectedRow()
  } catch (e: any) {
    console.error('[OpsRequestDetailsModal] Failed to fetch request details', e)
    appStore.showError(e?.message || t('admin.ops.requestDetails.failedToLoad'))
    items.value = []
    total.value = 0
    selectedRow.value = null
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      page.value = 1
      pageSize.value = 10
      selectedRow.value = null
      fetchData()
    } else {
      selectedRow.value = null
    }
  }
)

watch(
  () => [
    props.timeRange,
    props.platform,
    props.groupId,
    props.preset.kind,
    props.preset.sort,
    props.preset.min_duration_ms,
    props.preset.max_duration_ms
  ],
  () => {
    if (!props.modelValue) return
    page.value = 1
    fetchData()
  }
)

function handlePageChange(next: number) {
  page.value = next
  fetchData()
}

function handlePageSizeChange(next: number) {
  pageSize.value = next
  page.value = 1
  fetchData()
}

function selectRow(row: OpsRequestDetail) {
  selectedRow.value = row
}

const kindBadgeClass = (kind: string) => {
  if (kind === 'error') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
}
</script>

<template>
  <BaseDialog
    :show="modelValue"
    :title="props.preset.title || t('admin.ops.requestDetails.title')"
    width="full"
    content-class="!h-[88vh]"
    body-class="!p-0 !overflow-hidden flex flex-col"
    @close="close"
  >
    <div class="flex min-h-0 flex-1 flex-col">
      <!-- Toolbar -->
      <div class="flex-shrink-0 border-b border-gray-200 px-6 py-3 dark:border-dark-700">
        <div class="flex items-center justify-between">
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.errorDetails.total') }} {{ total }}
          </div>
          <button type="button" class="btn btn-secondary btn-sm" @click="fetchData">
            {{ t('common.refresh') }}
          </button>
        </div>
      </div>

      <!-- Body: 左右分栏 -->
      <div class="flex min-h-0 flex-1 p-6 pt-4">
        <div class="grid min-h-0 w-full gap-4 grid-cols-[minmax(0,1.45fr)_minmax(380px,1fr)]">

          <!-- 左列：列表 + 分页 -->
          <div class="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">

            <!-- Loading -->
            <div v-if="loading" class="flex flex-1 items-center justify-center py-16">
              <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
            </div>

            <!-- Empty -->
            <div v-else-if="items.length === 0" class="flex flex-1 items-center justify-center">
              <div class="px-8 py-12 text-center">
                <div class="text-sm font-medium text-gray-600 dark:text-gray-300">{{ t('admin.ops.requestDetails.empty') }}</div>
                <div class="mt-1 text-xs text-gray-400">{{ t('admin.ops.requestDetails.emptyHint') }}</div>
              </div>
            </div>

            <!-- Table -->
            <template v-else>
              <div class="min-h-0 flex-1 overflow-auto">
                <table class="w-full border-separate border-spacing-0">
                  <thead class="sticky top-0 z-10 bg-gray-50 dark:bg-dark-800">
                    <tr>
                      <th class="border-b border-gray-200 px-4 py-2.5 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-dark-400">
                        {{ t('admin.ops.requestDetails.table.time') }}
                      </th>
                      <th class="border-b border-gray-200 px-4 py-2.5 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-dark-400">
                        {{ t('admin.ops.requestDetails.table.kind') }}
                      </th>
                      <th class="border-b border-gray-200 px-4 py-2.5 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-dark-400">
                        {{ t('admin.ops.requestDetails.table.platform') }}
                      </th>
                      <th class="border-b border-gray-200 px-4 py-2.5 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-dark-400">
                        {{ t('admin.ops.requestDetails.table.model') }}
                      </th>
                      <th class="border-b border-gray-200 px-4 py-2.5 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-dark-400">
                        {{ t('admin.ops.requestDetails.table.duration') }}
                      </th>
                      <th class="border-b border-gray-200 px-4 py-2.5 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-dark-400">
                        {{ t('admin.ops.requestDetails.table.status') }}
                      </th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                    <tr
                      v-for="row in items"
                      :key="row.request_id"
                      :class="[
                        'group cursor-pointer transition-colors hover:bg-gray-50/80 dark:hover:bg-dark-800/50',
                        selectedRow?.request_id === row.request_id ? 'bg-primary-50/70 dark:bg-primary-900/10' : ''
                      ]"
                      @click="selectRow(row)"
                    >
                      <td class="whitespace-nowrap px-4 py-2 font-mono text-xs font-medium text-gray-900 dark:text-gray-200">
                        {{ formatDateTime(row.created_at).split(' ')[1] }}
                      </td>
                      <td class="whitespace-nowrap px-4 py-2">
                        <span class="rounded-full px-2 py-0.5 text-[10px] font-bold" :class="kindBadgeClass(row.kind)">
                          {{ row.kind === 'error' ? t('admin.ops.requestDetails.kind.error') : t('admin.ops.requestDetails.kind.success') }}
                        </span>
                      </td>
                      <td class="whitespace-nowrap px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-200">
                        <span class="inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-bold uppercase text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                          {{ row.platform || '-' }}
                        </span>
                      </td>
                      <td class="px-4 py-2">
                        <div class="max-w-[120px] truncate font-mono text-[11px] text-gray-700 dark:text-gray-300" :title="row.model || ''">
                          {{ row.model || '-' }}
                        </div>
                      </td>
                      <td class="whitespace-nowrap px-4 py-2 text-xs text-gray-600 dark:text-gray-300">
                        {{ typeof row.duration_ms === 'number' ? `${row.duration_ms} ms` : '-' }}
                      </td>
                      <td class="whitespace-nowrap px-4 py-2 text-xs text-gray-600 dark:text-gray-300">
                        {{ row.status_code ?? '-' }}
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>

              <Pagination
                v-if="total > 0"
                :total="total"
                :page="page"
                :page-size="pageSize"
                :page-size-options="[10, 20, 50]"
                @update:page="handlePageChange"
                @update:pageSize="handlePageSizeChange"
              />
            </template>
          </div>

          <!-- 右列：详情面板 -->
          <aside class="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-gray-50/70 dark:border-dark-700 dark:bg-dark-950/40">
            <div class="flex-shrink-0 border-b border-gray-200 px-4 py-3 dark:border-dark-700">
              <h3 class="text-sm font-bold text-gray-900 dark:text-white">{{ t('admin.ops.requestDetails.detailPaneTitle') }}</h3>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.requestDetails.detailPaneHint') }}</p>
            </div>
            <OpsRequestDetailPanel
              class="min-h-0 flex-1 overflow-auto"
              :row="selectedRow"
              :empty-text="t('admin.ops.requestDetails.detailPaneEmpty')"
            />
          </aside>
        </div>
      </div>
    </div>
  </BaseDialog>
</template>
