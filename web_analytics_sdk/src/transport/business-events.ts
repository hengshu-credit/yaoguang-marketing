import { Storage } from '../storage/storage';
import type { BusinessEventInput } from '../types';

type Occurrence = { report_id: string; event_code: string; event_name?: string; data: Record<string, unknown>; triggered_at: string; expires_at?: string | null };
type QueuedOccurrence = { event: Occurrence; touchpoint_token?: string; queued_at: number; identity_customer?: string };

function credentialScope(credential: string): { customer_id: string; expires_at: number } | null {
  try { return JSON.parse(atob(credential.split('.')[0].replace(/-/g, '+').replace(/_/g, '/'))) as { customer_id: string; expires_at: number }; } catch { return null; }
}

/** One immutable queued payload per real occurrence. Credentials are only held in memory. */
export class BusinessEventSender {
  private readonly key: string;
  private credential = '';
  private flushing: Promise<void> | null = null;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private readonly online = () => { void this.flush(); };
  constructor(private endpoint: string, private workspace: string, private session: string, private storage: Storage, tabId: string | number = 'default') {
    this.key = `business_events_${workspace}_${session}_${tabId}`;
    if (typeof window !== 'undefined') window.addEventListener('online', this.online);
  }
  async track(input: BusinessEventInput, touchpointToken?: string): Promise<string> {
    if (!input.event_code || !input.data || Array.isArray(input.data) || typeof input.data !== 'object') throw new Error('event_code and data object are required');
    const queue = this.storage.get<QueuedOccurrence[]>(this.key) ?? [];
    if (queue.length >= 100) throw new Error('Event queue is full; retry when the connection is available');
    const reportId = input.report_id ?? crypto.randomUUID();
    const event: Occurrence = JSON.parse(JSON.stringify({ ...input, report_id: reportId, triggered_at: input.triggered_at ?? new Date().toISOString() })) as Occurrence;
    queue.push({ event, touchpoint_token: touchpointToken, queued_at: Date.now(), identity_customer: credentialScope(this.credential)?.customer_id });
    this.storage.set(this.key, queue);
    await this.flush();
    return reportId;
  }
  async identify(credential: string): Promise<void> {
    if (!credential) throw new Error('A server-issued browser credential is required');
    const response = await fetch(`${this.endpoint.replace(/\/$/, '')}/track/identify`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, credentials: 'omit',
      body: JSON.stringify({ workspace_id: this.workspace, session_id: this.session, credential })
    });
    if (!response.ok) throw new Error(`Browser identity was rejected (${response.status})`);
    this.credential = credential;
    await this.flush();
    // Flush may have persisted previously offline anonymous events after the first association.
    const linked = await fetch(`${this.endpoint.replace(/\/$/, '')}/track/identify`, { method:'POST',headers:{'Content-Type':'application/json'},credentials:'omit',body:JSON.stringify({workspace_id:this.workspace,session_id:this.session,credential}) });
    if (!linked.ok) throw new Error(`Historical identity association was rejected (${linked.status})`);
  }
  clearIdentity(): void { this.credential = ''; }
  dispose(): void { if (this.retryTimer) clearTimeout(this.retryTimer); if (typeof window !== 'undefined') window.removeEventListener('online', this.online); }
  flush(): Promise<void> {
    if (this.flushing) return this.flushing;
    this.flushing = this.sendQueue().finally(() => { this.flushing = null; });
    return this.flushing;
  }
  private async sendQueue(): Promise<void> {
    if (this.retryTimer) { clearTimeout(this.retryTimer); this.retryTimer = null; }
    for (;;) {
      const queue = this.storage.get<QueuedOccurrence[]>(this.key) ?? [];
      const scope = credentialScope(this.credential);
      // Preserve other accounts' occurrences while allowing this account's queue
      // to progress. An identity change must neither relabel nor starve events.
      const item = queue.find(entry => !entry.identity_customer || (scope?.customer_id === entry.identity_customer && scope.expires_at * 1000 > Date.now()));
      if (!item) return;
      let retry = false;
      try {
        const response = await fetch(`${this.endpoint.replace(/\/$/, '')}/track/events`, {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, credentials: 'omit', keepalive: true,
          body: JSON.stringify({ workspace_id: this.workspace, session_id: this.session, credential: item.identity_customer ? this.credential : undefined, event: item.event, touchpoint_token: item.touchpoint_token })
        });
        retry = !response.ok && (response.status >= 500 || response.status === 408 || response.status === 429);
        if (!response.ok && !retry) console.warn(`[NotifuseAnalytics] Event ${item.event.report_id} rejected (${response.status})`);
      } catch { retry = true; }
      if (retry) { this.retryTimer = setTimeout(() => { void this.flush(); }, 5000); return; }
      // Re-read because track() may have appended while the network request was in flight.
      const current = this.storage.get<QueuedOccurrence[]>(this.key) ?? [];
      this.storage.set(this.key, current.filter(entry => entry.event.report_id !== item.event.report_id));
    }
  }
}
