import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsErrorDetailsModal from '../OpsErrorDetailsModal.vue'

const mockListRequestErrors = vi.fn()
const mockListUpstreamErrors = vi.fn()

vi.mock('@/api/admin/ops', async (importOriginal) => {
  const actual = await importOriginal() as typeof import('../../../../../api/admin/ops')
  return {
    ...actual,
    default: actual.default,
    opsAPI: {
      ...actual.opsAPI,
      listRequestErrors: (...args: any[]) => mockListRequestErrors(...args),
      listUpstreamErrors: (...args: any[]) => mockListUpstreamErrors(...args),
    },
  }
})

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: '' },
  },
  emits: ['close'],
  template: `
    <div v-if="show" class="base-dialog-stub">
      <div class="base-dialog-title">{{ title }}</div>
      <slot />
    </div>
  `,
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: {
      type: [String, Number, Object, null],
      default: '',
    },
  },
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />',
})

const OpsErrorLogTableStub = defineComponent({
  name: 'OpsErrorLogTable',
  props: {
    rows: { type: Array, default: () => [] },
    total: { type: Number, default: 0 },
    loading: { type: Boolean, default: false },
    page: { type: Number, default: 1 },
    pageSize: { type: Number, default: 10 },
    selectedId: { type: Number, default: null },
  },
  emits: ['openErrorDetail', 'update:page', 'update:pageSize'],
  template: `
    <div class="error-log-table-stub">
      <button
        v-for="row in rows"
        :key="row.id"
        type="button"
        class="row-trigger"
        :data-id="row.id"
        :data-selected="selectedId === row.id"
        @click="$emit('openErrorDetail', row.id)"
      >
        {{ row.id }}
      </button>
    </div>
  `,
})

const OpsErrorDetailPanelStub = defineComponent({
  name: 'OpsErrorDetailPanel',
  props: {
    errorId: { type: Number, default: null },
  },
  template: '<div class="detail-panel-stub">selected={{ errorId ?? "none" }}</div>',
})

const sampleRows = [
  {
    id: 11,
    created_at: '2026-03-06T10:00:00Z',
    phase: 'request',
    error_owner: 'client',
    status_code: 400,
    message: 'first error',
    platform: 'openai',
    model: 'gpt-4o-mini',
  },
  {
    id: 22,
    created_at: '2026-03-06T10:05:00Z',
    phase: 'request',
    error_owner: 'client',
    status_code: 500,
    message: 'second error',
    platform: 'openai',
    model: 'gpt-4.1',
  },
]

describe('OpsErrorDetailsModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockListRequestErrors.mockResolvedValue({ items: sampleRows, total: sampleRows.length })
    mockListUpstreamErrors.mockResolvedValue({ items: sampleRows, total: sampleRows.length })
  })

  it('请求错误使用左右分栏详情，并支持 ESC 只关闭右侧详情', async () => {
    const wrapper = mount(OpsErrorDetailsModal, {
      props: {
        show: true,
        timeRange: '1h',
        errorType: 'request',
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          OpsErrorLogTable: OpsErrorLogTableStub,
          OpsErrorDetailPanel: OpsErrorDetailPanelStub,
        },
      },
    })

    await flushPromises()
    await flushPromises()

    expect(wrapper.find('.base-dialog-stub').exists()).toBe(true)
    expect(wrapper.find('.detail-panel-stub').text()).toContain('selected=11')

    const rowButtons = wrapper.findAll('.row-trigger')
    await rowButtons[1].trigger('click')
    await flushPromises()

    expect(wrapper.find('.base-dialog-stub').exists()).toBe(true)
    expect(wrapper.find('.detail-panel-stub').text()).toContain('selected=22')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()

    expect(wrapper.find('.base-dialog-stub').exists()).toBe(true)
    expect(wrapper.emitted('update:show')).toBeFalsy()
    expect(wrapper.find('.detail-panel-stub').text()).toContain('selected=none')
  })

  it('上游错误仍然走外部详情打开事件', async () => {
    const wrapper = mount(OpsErrorDetailsModal, {
      props: {
        show: true,
        timeRange: '1h',
        errorType: 'upstream',
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          OpsErrorLogTable: OpsErrorLogTableStub,
          OpsErrorDetailPanel: OpsErrorDetailPanelStub,
        },
      },
    })

    await flushPromises()
    await flushPromises()

    const rowButtons = wrapper.findAll('.row-trigger')
    await rowButtons[0].trigger('click')

    expect(wrapper.emitted('openErrorDetail')).toEqual([[11]])
    expect(wrapper.find('.detail-panel-stub').exists()).toBe(false)
  })
})
