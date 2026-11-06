import { AxiosError, AxiosHeaders, type InternalAxiosRequestConfig } from 'axios'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it, vi } from 'vitest'

const refresh = vi.hoisted(() => vi.fn())
vi.mock('@/services/authentication-service', () => ({ authenticationService: { refresh } }))

import http from './http'

beforeEach(() => {
  setActivePinia(createPinia())
  refresh.mockReset()
})

// A request refused with 401 is retried once with a refreshed token; a second
// 401 is final, so a server that keeps refusing is not called in a loop.
it('retries a refused request once after a refresh', async () => {
  refresh.mockResolvedValue(true)
  let calls = 0
  http.defaults.adapter = (config: InternalAxiosRequestConfig) => {
    calls++
    const response = { data: { message: 'missing JWT token' }, status: 401, statusText: '', headers: {}, config }
    return Promise.reject(new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, null, response))
  }
  await expect(http.get('/contract/retrieve/x', { headers: new AxiosHeaders() })).rejects.toThrow('missing JWT token')
  expect(calls).toBe(2)
  expect(refresh).toHaveBeenCalledTimes(1)
})

// DCS-FR-PACM-03: a transition held for review names the gate run, so the user
// can give it to the Compliance Officer who decides it.
it('names the workflow-gate run of a transition held for review', async () => {
  http.defaults.adapter = (config: InternalAxiosRequestConfig) => {
    const response = {
      data: { name: 'workflow_gate_blocked', message: 'workflow gate result is REVIEW', gate_run_id: 'run-42' },
      status: 409,
      statusText: '',
      headers: {},
      config,
    }
    return Promise.reject(new AxiosError('Conflict', 'ERR_BAD_REQUEST', config, null, response))
  }
  await expect(http.post('/contract/approve', {})).rejects.toThrow(/workflow gate result is REVIEW.*run-42/)
})

// UC-IF-19: a lockout refusal carries Retry-After. No refresh is tried, since
// the lock refuses every credential, and the message says when to try again.
it('shows the lockout instead of refreshing', async () => {
  let calls = 0
  http.defaults.adapter = (config: InternalAxiosRequestConfig) => {
    calls++
    const response = {
      data: { message: 'Credential invalid or access revoked.' },
      status: 401,
      statusText: '',
      headers: { 'retry-after': '600' },
      config,
    }
    return Promise.reject(new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, null, response))
  }
  await expect(http.get('/contract/retrieve/x', { headers: new AxiosHeaders() })).rejects.toThrow(
    'Credential invalid or access revoked. Too many invalid credentials came from this address. Try again in 10 minutes.',
  )
  expect(calls).toBe(1)
  expect(refresh).not.toHaveBeenCalled()
})
