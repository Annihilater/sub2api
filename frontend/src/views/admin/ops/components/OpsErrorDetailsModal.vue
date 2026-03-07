<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import OpsErrorLogTable from './OpsErrorLogTable.vue'
import OpsErrorDetailPanel from './OpsErrorDetailPanel.vue'
import { opsAPI, type OpsErrorLog } from '@/api/admin/ops'
import { pushEscape } from '@/composables/useEscapeStack'

interface Props {
  show: boolean
  timeRange: string
  platform?: string
  groupId?: number | null
  errorType: 'request' | 'upstream'
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'openErrorDetail', errorId: number): void
}>()

const { t } = useI18n()


const loading = ref(false)
const rows = ref<OpsErrorLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const selectedErrorId = ref<number | null>(null)

const q = ref('')
const statusCode = ref<number | 'other' | null>(null)
const phase = ref<string>('')
const errorOwner = ref<string>('')
const viewMode = ref<'errors' | 'excluded' | 'all'>('errors')


const modalTitle = computed(() => {
  return props.errorType === 'upstream' ? t('admin.ops.errorDetails.upstreamErrors') : t('admin.ops.errorDetails.requestErrors')
})

const usesSplitDetail = computed(() => props.errorType === 'request')

const statusCodeSelectOptions = computed(() => {
  const codes = [400, 401, 403, 404, 409, 422, 429, 500, 502, 503, 504, 529]
  return [
    { value: null, label: t('common.all') },
    ...codes.map((c) => ({ value: c, label: String(c) })),
    { value: 'other', label: t('admin.ops.errorDetails.statusCodeOther') || 'Other' }
  ]
})

const ownerSelectOptions = computed(() => {
  return [
    { value: '', label: t('common.all') },
    { value: 'provider', label: t('admin.ops.errorDetails.owner.provider') || 'provider' },
    { value: 'client', label: t('admin.ops.errorDetails.owner.client') || 'client' },
    { value: 'platform', label: t('admin.ops.errorDetails.owner.platform') || 'platform' }
  ]
})


const viewModeSelectOptions = computed(() => {
  return [
    { value: 'errors', label: t('admin.ops.errorDetails.viewErrors') || 'errors' },
    { value: 'excluded', label: t('admin.ops.errorDetails.viewExcluded') || 'excluded' },
    { value: 'all', label: t('common.all') }
  ]
})

const phaseSelectOptions = computed(() => {
  const options = [
    { value: '', label: t('common.all') },
    { value: 'request', label: t('admin.ops.errorDetails.phase.request') || 'request' },
    { value: 'auth', label: t('admin.ops.errorDetails.phase.auth') || 'auth' },
    { value: 'routing', label: t('admin.ops.errorDetails.phase.routing') || 'routing' },
    { value: 'upstream', label: t('admin.ops.errorDetails.phase.upstream') || 'upstream' },
    { value: 'network', label: t('admin.ops.errorDetails.phase.network') || 'network' },
    { value: 'internal', label: t('admin.ops.errorDetails.phase.internal') || 'internal' }
  ]
  return options
})

function close() {
  emit('update:show', false)
}

function syncSelectedError() {
  if (!usesSplitDetail.value) {
    selectedErrorId.value = null
    return
  }

  if (selectedErrorId.value && rows.value.some((row) => row.id === selectedErrorId.value)) return

  if (rows.value.length > 0) {
    selectedErrorId.value = rows.value[0].id
    return
  }

  selectedErrorId.value = null
}

function openErrorDetail(errorId: number) {
  if (!usesSplitDetail.value) {
    emit('openErrorDetail', errorId)
    return
  }
  selectedErrorId.value = errorId
}

// 当 detail 面板打开时，压入一个高于 BaseDialog 的 ESC 层（后 push 先触发）
// 按 ESC 先关闭 detail，再按一次才关闭整个 modal
let popDetailEscape: (() => void) | null = null

watch(
  () => [props.show, selectedErrorId.value] as const,
  ([show, id]) => {
    popDetailEscape?.()
    popDetailEscape = null
    if (show && usesSplitDetail.value && id != null) {
      popDetailEscape = pushEscape(() => {
        selectedErrorId.value = null
      })
    }
  }
)

onUnmounted(() => {
  popDetailEscape?.()
  popDetailEscape = null
})

async function fetchErrorLogs() {
  if (!props.show) return

  loading.value = true
  try {
    const params: Record<string, any> = {
      page: page.value,
      page_size: pageSize.value,
      time_range: props.timeRange,
      view: viewMode.value
    }

    const platform = String(props.platform || '').trim()
    if (platform) params.platform = platform
    if (typeof props.groupId === 'number' && props.groupId > 0) params.group_id = props.groupId

    if (q.value.trim()) params.q = q.value.trim()
    if (statusCode.value === 'other') params.status_codes_other = '1'
    else if (typeof statusCode.value === 'number') params.status_codes = String(statusCode.value)

    const phaseVal = String(phase.value || '').trim()
    if (phaseVal) params.phase = phaseVal

    const ownerVal = String(errorOwner.value || '').trim()
    if (ownerVal) params.error_owner = ownerVal


    const res = props.errorType === 'upstream'
      ? await opsAPI.listUpstreamErrors(params)
      : await opsAPI.listRequestErrors(params)
    rows.value = res.items || []
    total.value = res.total || 0
    syncSelectedError()
  } catch (err) {
    console.error('[OpsErrorDetailsModal] Failed to fetch error logs', err)
    rows.value = []
    total.value = 0
    selectedErrorId.value = null
  } finally {
    loading.value = false
  }
}

  function resetFilters() {
    q.value = ''
    statusCode.value = null
    phase.value = props.errorType === 'upstream' ? 'upstream' : ''
    errorOwner.value = ''
    viewMode.value = 'errors'
    page.value = 1
    selectedErrorId.value = null
    fetchErrorLogs()
  }


watch(
  () => props.show,
  (open) => {
    if (!open) {
      selectedErrorId.value = null
      return
    }
    page.value = 1
    pageSize.value = 10
    resetFilters()
  },
  { immediate: true }
)

watch(
  () => [props.timeRange, props.platform, props.groupId] as const,
  () => {
    if (!props.show) return
    page.value = 1
    fetchErrorLogs()
  }
)

watch(
  () => [page.value, pageSize.value] as const,
  () => {
    if (!props.show) return
    fetchErrorLogs()
  }
)

let searchTimeout: number | null = null
watch(
  () => q.value,
  () => {
    if (!props.show) return
    if (searchTimeout) window.clearTimeout(searchTimeout)
    searchTimeout = window.setTimeout(() => {
      page.value = 1
      fetchErrorLogs()
    }, 350)
  }
)

watch(
  () => [statusCode.value, phase.value, errorOwner.value, viewMode.value] as const,
  () => {
    if (!props.show) return
    page.value = 1
    fetchErrorLogs()
  }
)
</script>

<template>
  <BaseDialog
    :show="show"
    :title="modalTitle"
    width="full"
    content-class="!h-[88vh]"
    body-class="!p-0 !overflow-hidden flex flex-col"
    @close="close"
  >
    <div class="flex min-h-0 flex-1 flex-col">
      <!-- Filters -->
      <div class="flex-shrink-0 border-b border-gray-200 px-6 py-4 dark:border-dark-700">
        <div class="grid grid-cols-8 gap-2">
          <div class="col-span-2 compact-select">
            <div class="relative group">
              <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
                <svg
                  class="h-3.5 w-3.5 text-gray-400 transition-colors group-focus-within:text-blue-500"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                </svg>
              </div>
              <input
                v-model="q"
                type="text"
                class="w-full rounded-lg border-gray-200 bg-gray-50/50 py-1.5 pl-9 pr-3 text-xs font-medium text-gray-700 transition-all focus:border-blue-500 focus:bg-white focus:ring-2 focus:ring-blue-500/10 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300 dark:focus:bg-dark-800"
                :placeholder="t('admin.ops.errorDetails.searchPlaceholder')"
              />
            </div>
          </div>

          <div class="compact-select">
            <Select :model-value="statusCode" :options="statusCodeSelectOptions" @update:model-value="statusCode = $event as any" />
          </div>

          <div class="compact-select">
            <Select :model-value="phase" :options="phaseSelectOptions" @update:model-value="phase = String($event ?? '')" />
          </div>

          <div class="compact-select">
            <Select :model-value="errorOwner" :options="ownerSelectOptions" @update:model-value="errorOwner = String($event ?? '')" />
          </div>

          <div class="compact-select">
            <Select :model-value="viewMode" :options="viewModeSelectOptions" @update:model-value="viewMode = $event as any" />
          </div>

          <div class="flex items-center justify-end">
            <button type="button" class="rounded-lg bg-gray-100 px-3 py-1.5 text-xs font-semibold text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600" @click="resetFilters">
              {{ t('common.reset') }}
            </button>
          </div>
        </div>
      </div>

      <!-- Body：填满剩余高度，内部各列自管滚动 -->
      <div class="flex min-h-0 flex-1 p-6 pt-4">
        <!-- Request errors: 左右固定分栏 -->
        <div v-if="usesSplitDetail" class="grid min-h-0 w-full gap-4 grid-cols-[minmax(0,1.45fr)_minmax(380px,1fr)]">
          <!-- 左列：表格 + 分页，内部自管滚动 -->
          <div class="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
            <div class="flex-shrink-0 border-b border-gray-200 px-4 py-3 dark:border-dark-700">
              <div class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.errorDetails.total') }} {{ total }}
              </div>
            </div>

            <OpsErrorLogTable
              class="min-h-0 flex-1"
              :rows="rows"
              :total="total"
              :loading="loading"
              :page="page"
              :page-size="pageSize"
              :selected-id="selectedErrorId"
              @openErrorDetail="openErrorDetail"
              @update:page="page = $event"
              @update:pageSize="pageSize = $event"
            />
          </div>

          <!-- 右列：详情面板，内部自管滚动 -->
          <aside class="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-gray-50/70 dark:border-dark-700 dark:bg-dark-950/40">
            <div class="flex-shrink-0 border-b border-gray-200 px-4 py-3 dark:border-dark-700">
              <h3 class="text-sm font-bold text-gray-900 dark:text-white">{{ t('admin.ops.errorDetails.detailPaneTitle') }}</h3>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.errorDetails.detailPaneHint') }}</p>
            </div>

            <!-- overflow-auto 在 Panel 内部，flex-1 撑满剩余高度 -->
            <OpsErrorDetailPanel
              class="min-h-0 flex-1 overflow-auto"
              :show="show"
              :error-id="selectedErrorId"
              :error-type="errorType"
              :empty-text="t('admin.ops.errorDetails.detailPaneEmpty')"
            />
          </aside>
        </div>

        <!-- Upstream errors: 单列布局 -->
        <template v-else>
          <div class="flex w-full min-h-0 flex-col">
            <div class="mb-2 flex-shrink-0 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.errorDetails.total') }} {{ total }}
            </div>
            <OpsErrorLogTable
              class="min-h-0 flex-1"
              :rows="rows"
              :total="total"
              :loading="loading"
              :page="page"
              :page-size="pageSize"
              @openErrorDetail="emit('openErrorDetail', $event)"
              @update:page="page = $event"
              @update:pageSize="pageSize = $event"
            />
          </div>
        </template>
      </div>
    </div>
  </BaseDialog>
</template>

<style>
.compact-select .select-trigger {
  @apply py-1.5 px-3 text-xs rounded-lg;
}
</style>
