import http from '@/api/http'

/** A DCS event an organization can subscribe to (GET /orce/events). */
export interface WebhookEvent {
  name: string
  description: string
}

/** A webhook subscription of the caller's organization. */
export interface WebhookSubscription {
  id: string
  event: string
  callback_url: string
  created_at: string
}

/** The response of a new subscription: the only one that carries its signing secret. */
export interface CreatedWebhookSubscription extends WebhookSubscription {
  secret: string
}

/**
 * Webhook subscriptions of the caller's organization (DCS-FR-TR-22,
 * DCS-NFR-SQ-08). The backend serves them under the API path at /orce, for the
 * Integration Manager, the System Administrator and the machine role
 * Sys. Integration Manager.
 */
export const webhookService = {
  async listEvents(): Promise<WebhookEvent[]> {
    return http.get<WebhookEvent[]>('/orce/events').then((res) => res.data)
  },
  async list(): Promise<WebhookSubscription[]> {
    return http.get<WebhookSubscription[]>('/orce/webhooks').then((res) => res.data)
  },
  async create(event: string, callbackURL: string): Promise<CreatedWebhookSubscription> {
    return http
      .post<CreatedWebhookSubscription>('/orce/webhooks', { event, callback_url: callbackURL })
      .then((res) => res.data)
  },
  async remove(id: string): Promise<void> {
    await http.delete(`/orce/webhooks/${encodeURIComponent(id)}`)
  },
}
