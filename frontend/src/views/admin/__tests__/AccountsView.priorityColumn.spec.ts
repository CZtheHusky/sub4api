import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const { listAccounts, listWithEtag, getSettings, getById } = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getSettings: vi.fn(),
  getById: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    settings: { getSettings },
    accounts: {
      list: listAccounts,
      listWithEtag,
      getById,
      getBatchTodayStats: vi.fn(async () => ({ stats: {} })),
      getUpstreamBillingProbeSettings: vi.fn(async () => ({ enabled: true, interval_minutes: 30 })),
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: { getAll: vi.fn(async () => []) },
    groups: { getAll: vi.fn(async () => []) }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token', isSimpleMode: false })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const DataTableStub = {
  props: ['columns', 'data'],
  emits: ['sort'],
  template: `
    <div data-test="data-table">
      <span v-for="column in columns" :key="column.key" :data-column="column.key">
        {{ column.sortable ? 'sortable' : 'fixed' }}
      </span>
      <div v-for="row in data" :key="row.id" :data-account-name="row.name">
        <slot name="cell-status" :row="row" />
      </div>
      <button data-test="sort-priority" @click="$emit('sort', 'priority', 'desc')" />
    </div>
  `
}

function mountView() {
  return mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        DataTable: DataTableStub,
        AccountTableActions: {
          emits: ['refresh'],
          template: `<div><button data-test="refresh" @click="$emit('refresh')" /><slot name="after" /></div>`
        },
        AccountTableFilters: true,
        AccountBulkActionsBar: true,
        Pagination: true,
        ConfirmDialog: true,
        AccountActionMenu: true,
        CodexDiagnosticModal: true,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: true,
        AccountStatsModal: true,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: true,
        BulkEditAccountModal: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: true,
        AccountUsageCell: true,
        HelpTooltip: true,
        Icon: true,
        Teleport: true
      }
    }
  })
}

enableAutoUnmount(afterEach)
afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

beforeEach(() => {
  localStorage.clear()
  getSettings.mockReset().mockResolvedValue({})
  listWithEtag.mockReset().mockResolvedValue({ notModified: true, etag: 'accounts-etag', data: null })
  getById.mockReset()
})

describe('admin AccountsView priority column preferences', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset().mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0
    })
  })

  it('shows priority as a sortable column for fresh preferences', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-column="priority"]').text()).toBe('sortable')

    await wrapper.get('[data-test="sort-priority"]').trigger('click')
    await flushPromises()

    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      20,
      expect.objectContaining({ sort_by: 'priority', sort_order: 'desc' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })

  it('preserves an existing preference that explicitly hides priority', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify(['priority', 'today_stats']))
    localStorage.setItem('account-hidden-columns-version', 'scheduler-score-hidden-by-default')

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-column="priority"]').exists()).toBe(false)
    expect(JSON.parse(localStorage.getItem('account-hidden-columns') || '[]')).toEqual([
      'priority',
      'today_stats'
    ])
  })

  it('keeps priority visible while migrating older saved preferences', async () => {
    localStorage.setItem('account-hidden-columns', JSON.stringify(['today_stats']))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-column="priority"]').text()).toBe('sortable')
    expect(JSON.parse(localStorage.getItem('account-hidden-columns') || '[]')).toEqual(
      expect.arrayContaining(['today_stats', 'scheduler_score'])
    )
    expect(JSON.parse(localStorage.getItem('account-hidden-columns') || '[]')).not.toContain('priority')
  })
})


describe('independent model diagnostic entry', () => {
  it('opens the diagnostic modal from the account action menu', async () => {
    listAccounts.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    const wrapper = mountView()
    await flushPromises()
    const selected = { id: 19, name: 'Diagnostic account', platform: 'openai', type: 'oauth', extra: {} }
    const diagnostic = wrapper.getComponent({ name: 'CodexDiagnosticModal' })
    wrapper.getComponent({ name: 'AccountActionMenu' }).vm.$emit('codex-diagnostic', selected)
    await flushPromises()
    expect(diagnostic.props('show')).toBe(true)
    expect(diagnostic.props('account')).toEqual(selected)
    diagnostic.vm.$emit('close')
    await flushPromises()
    expect(diagnostic.props('show')).toBe(false)
  })
})
