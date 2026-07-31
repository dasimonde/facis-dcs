import axios from 'axios'
import authHttp from '@/api/auth-http'
import {
  PID_POLL_ERROR,
  type PidPresentationPollStatus,
  type PidPresentationResponse,
  type PidPresentationStatusResponse,
} from '@/models/responses/pid-presentation-response'

export const pidPresentationService = {
  async start() {
    return authHttp
      .post<PidPresentationResponse>('/auth/pid/presentation')
      .then((res) => ({
        presentationUrl: res.data.presentation_url,
        state: res.data.state,
        expiresIn: res.data.expires_in,
      }))
      .catch((err: unknown) => {
        console.error('PID presentation start error:', err)
        return null
      })
  },

  async renew(state: string) {
    return authHttp
      .post<PidPresentationResponse>('/auth/pid/presentation/renew', { state })
      .then((res) => ({
        presentationUrl: res.data.presentation_url,
        state: res.data.state,
        expiresIn: res.data.expires_in,
      }))
      .catch((err: unknown) => {
        console.error('PID presentation renew error:', err)
        return null
      })
  },

  async pollStatus(state: string): Promise<PidPresentationPollStatus> {
    try {
      const res = await authHttp.get<PidPresentationStatusResponse>('/auth/pid/presentation/status', {
        params: { state },
        timeout: 30_000,
      })
      return res.data
    } catch (err: unknown) {
      if (axios.isAxiosError(err) && err.code === 'ECONNABORTED') {
        return PID_POLL_ERROR.TIMEOUT
      }
      if (axios.isAxiosError(err) && err.response?.status === 404) {
        return PID_POLL_ERROR.NOT_FOUND
      }
      console.error('PID presentation status error:', err)
      return null
    }
  },
}
