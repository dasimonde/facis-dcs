import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { useAuthTokenStore } from '@/stores/auth-token-store'
import { parseContractChanges, useContractChanges } from './useContractChanges'

const refresh = vi.hoisted(() => vi.fn())

const mountChanges = (onChange: () => void, onGap?: () => void) =>
  mount(defineComponent({ setup: () => useContractChanges(onChange, onGap), render: () => null }))
vi.mock('@/services/authentication-service', () => ({ authenticationService: { refresh } }))

// DCS-FR-CWE-24: the stream parser keeps an event split across chunks and
// drops the events that name no contract.
it('parses complete events and keeps the unfinished rest', () => {
  const first = parseContractChanges(
    'data: {"event_type":"CONNECTED","did":""}\n\ndata: {"did":"a","event_type":"SUBMIT_CONTRACT"}\n\ndata: {"did":"b",',
  )
  expect(first.changes).toEqual([{ did: 'a', event_type: 'SUBMIT_CONTRACT' }])

  const second = parseContractChanges(first.rest + '"event_type":"TERMINATE_CONTRACT"}\n\n')
  expect(second.changes).toEqual([{ did: 'b', event_type: 'TERMINATE_CONTRACT' }])
  expect(second.rest).toBe('')
})

// A refused stream waits before the next attempt, and longer after each
// further refusal, instead of reconnecting at once.
it('backs off after a refused stream', async () => {
  vi.useFakeTimers()
  setActivePinia(createPinia())
  useAuthTokenStore().setTokens('Bearer', 'header.payload.signature')
  const fetchMock = vi.fn().mockResolvedValue(new Response('', { status: 401 }))
  vi.stubGlobal('fetch', fetchMock)
  refresh.mockResolvedValue(true)
  const wrapper = mountChanges(() => undefined)
  await vi.advanceTimersByTimeAsync(100)
  expect(fetchMock).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(4_800)
  expect(fetchMock).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(300)
  expect(fetchMock).toHaveBeenCalledTimes(2)
  await vi.advanceTimersByTimeAsync(9_000)
  expect(fetchMock).toHaveBeenCalledTimes(2)
  await vi.advanceTimersByTimeAsync(1_200)
  expect(fetchMock).toHaveBeenCalledTimes(3)
  wrapper.unmount()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const openStream = (text: string) =>
  new Response(
    new ReadableStream({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(text))
      },
    }),
    { status: 200 },
  )

// DCS-FR-CWE-24, DCS-FR-CWE-27: changes made while the stream is down are
// reported as a gap, so the list can be marked outdated.
it('reports a gap when a stream connects after a failed one', async () => {
  vi.useFakeTimers()
  setActivePinia(createPinia())
  useAuthTokenStore().setTokens('Bearer', 'header.payload.signature')
  const fetchMock = vi
    .fn()
    .mockRejectedValueOnce(new TypeError('network down'))
    .mockResolvedValueOnce(openStream('data: {"event_type":"CONNECTED","did":""}\n\n'))
  vi.stubGlobal('fetch', fetchMock)
  const onGap = vi.fn()
  const wrapper = mountChanges(() => undefined, onGap)
  await vi.advanceTimersByTimeAsync(100)
  expect(onGap).not.toHaveBeenCalled()
  await vi.advanceTimersByTimeAsync(5_100)
  expect(fetchMock).toHaveBeenCalledTimes(2)
  expect(onGap).toHaveBeenCalledOnce()
  wrapper.unmount()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

it('reports a gap when the server drops changes for the stream', async () => {
  vi.useFakeTimers()
  setActivePinia(createPinia())
  useAuthTokenStore().setTokens('Bearer', 'header.payload.signature')
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(openStream('data: {"event_type":"RESYNC"}\n\n')))
  const onGap = vi.fn()
  const onChange = vi.fn()
  const wrapper = mountChanges(onChange, onGap)
  await vi.advanceTimersByTimeAsync(100)
  expect(onGap).toHaveBeenCalledOnce()
  expect(onChange).not.toHaveBeenCalled()
  wrapper.unmount()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

// CWE-23: a stream the server ends normally (token lifetime, shutdown of a
// replica) is followed by a new one at once; a change made in between reaches
// neither, so it is reported as a gap too.
it('reports a gap when a stream connects after a stream that ended', async () => {
  vi.useFakeTimers()
  setActivePinia(createPinia())
  useAuthTokenStore().setTokens('Bearer', 'header.payload.signature')
  const ended = () => new Response('data: {"event_type":"CONNECTED","did":""}\n\n', { status: 200 })
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(ended())
    .mockResolvedValueOnce(openStream('data: {"event_type":"CONNECTED","did":""}\n\n'))
  vi.stubGlobal('fetch', fetchMock)
  const onGap = vi.fn()
  const wrapper = mountChanges(() => undefined, onGap)
  await vi.advanceTimersByTimeAsync(100)
  expect(fetchMock).toHaveBeenCalledTimes(2)
  expect(onGap).toHaveBeenCalledOnce()
  wrapper.unmount()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})
