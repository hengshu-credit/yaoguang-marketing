import { beforeEach, describe, expect, it, vi } from 'vitest';
import { BusinessEventSender } from '../../src/transport/business-events';
import { Storage } from '../../src/storage/storage';

describe('business event occurrence queue', () => {
  beforeEach(() => { localStorage.clear(); vi.unstubAllGlobals(); });
  it('keeps queues independent for tabs sharing a session', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('offline')); vi.stubGlobal('fetch', fetcher);
    const one = new BusinessEventSender('https://api.example','w','s',new Storage(),'tab-one');
    const two = new BusinessEventSender('https://api.example','w','s',new Storage(),'tab-two');
    const first = await one.track({event_code:'login',data:{}});
    const second = await two.track({event_code:'login',data:{}});
    expect(JSON.parse(fetcher.mock.calls[1][1].body).event.report_id).toBe(second);
    expect(second).not.toBe(first);
    one.dispose(); two.dispose();
  });
  it('keeps the report ID across three transports and creates a new ID for the next occurrence', async () => {
    const fetcher = vi.fn().mockRejectedValueOnce(new Error('offline')).mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ ok: true, status: 200 });
    vi.stubGlobal('fetch', fetcher);
    const sender = new BusinessEventSender('https://api.example', 'workspace', 'session', new Storage());
    const first = await sender.track({ event_code: 'login', data: {} });
    await sender.flush(); await sender.flush();
    const ids = fetcher.mock.calls.slice(0,3).map(call => JSON.parse(call[1].body).event.report_id);
    expect(ids).toEqual([first,first,first]);
    const second = await sender.track({ event_code: 'login', data: {} });
    expect(second).not.toBe(first);
  });
  it('freezes caller data when queued', async () => {
    const fetcher = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ok:true,status:200});vi.stubGlobal('fetch',fetcher);
    const sender = new BusinessEventSender('https://api.example','w','s',new Storage());
    const data = {amount:10};await sender.track({event_code:'login',data});data.amount=999;await sender.flush();
    expect(JSON.parse(fetcher.mock.calls[1][1].body).event.data.amount).toBe(10);
  });
  it('does not reassign an offline identified occurrence after another customer logs in', async () => {
    const fetcher = vi.fn().mockResolvedValue({ok:true,status:200});vi.stubGlobal('fetch',fetcher);
    const token = (customer:string) => `${btoa(JSON.stringify({customer_id:customer,expires_at:Math.floor(Date.now()/1000)+900}))}.signature`;
    const sender = new BusinessEventSender('https://api.example','w','session',new Storage());
    await sender.identify(token('customer-a'));
    fetcher.mockRejectedValueOnce(new Error('offline'));
    await sender.track({event_code:'login',data:{}});
    await sender.identify(token('customer-b'));
    expect(fetcher.mock.calls.filter(call=>call[0].endsWith('/track/events'))).toHaveLength(1);
    const second = await sender.track({event_code:'login',data:{account:'b'}});
    const sent = fetcher.mock.calls.filter(call=>call[0].endsWith('/track/events'));
    expect(sent).toHaveLength(2);
    expect(JSON.parse(sent[1][1].body).event.report_id).toBe(second);
    // Returning to A can still flush its preserved occurrence.
    await sender.identify(token('customer-a'));
    expect(fetcher.mock.calls.filter(call=>call[0].endsWith('/track/events'))).toHaveLength(3);
    sender.dispose();
  });
});
