import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { readFileSync } from 'node:fs'
import type { TicketAttempt } from '@/api/admin/codexTickets'
import ModelTraceHistoryView from '../ModelTraceHistoryView.vue'

const { history } = vi.hoisted(() => ({ history: vi.fn() }))
vi.mock('@/api/admin/codexTickets', () => ({ history, fingerprint: vi.fn(async () => ({ commit: 'bank' })) }))
vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn(async () => ({ data: { items: [{ id: 41, name: 'test' }] } })) } }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key, te: () => false, locale: ref('en') }) }))

const fixture = process.env.CODEX_WS_PROBE_HISTORY_FIXTURE
const persisted: TicketAttempt[] = fixture ? JSON.parse(readFileSync(fixture, 'utf8')).data.items : []

describe('ModelTrace history WS probe observations', () => {
  it.each([
    { transport: 'ws', safety_buffering_enabled: false, safety_buffering_faster_model: 'marker', turn_state_len: 0 },
    { transport: 'ws', safety_buffering_enabled: true, turn_state_len: 780 },
    { transport: 'http' },
    {},
    ...persisted
  ])('preserves explicit values and old-record unknowns: %j', async (observations) => {
    history.mockResolvedValue({ items: [{ id: 1, account_id: 41, model: 'gpt-6-astra', outcome: 'success', occurred_at: '2026-10-20T00:00:00Z', fingerprint_matched: true, ...observations }], total: 1 })
    const wrapper = mount(ModelTraceHistoryView, { global: { stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: { template: '<div><slot name="filters"/><slot name="table"/></div>' },
      ConfirmDialog: true, Icon: true
    } } })
    await flushPromises()
    await wrapper.get('tbody tr').trigger('click')
    const details = Object.fromEntries(wrapper.findAll('dl > div').map(item => [item.get('dt').text().split('.').at(-1), item.get('dd').text()]))
    expect(details.transport).toBe(observations.transport || '—')
    expect(details.safetyBufferingEnabled).toBe(observations.safety_buffering_enabled == null ? '—' : String(observations.safety_buffering_enabled))
    expect(details.safetyBufferingFasterModel).toBe(observations.safety_buffering_faster_model || '—')
    expect(details.turnStateLen).toBe(observations.turn_state_len == null ? '—' : String(observations.turn_state_len))
    expect(wrapper.text()).toContain('admin.accounts.codexDiagnosticDialog.observationHint')
    wrapper.unmount()
  })
})
