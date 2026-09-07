import { Storage } from '../storage/storage';
import type { BusinessEventInput } from '../types';
/** One immutable queued payload per real occurrence. Credentials are only held in memory. */
export declare class BusinessEventSender {
    private endpoint;
    private workspace;
    private session;
    private storage;
    private readonly key;
    private credential;
    private flushing;
    private retryTimer;
    private readonly online;
    constructor(endpoint: string, workspace: string, session: string, storage: Storage, tabId?: string | number);
    track(input: BusinessEventInput, touchpointToken?: string): Promise<string>;
    identify(credential: string): Promise<void>;
    clearIdentity(): void;
    dispose(): void;
    flush(): Promise<void>;
    private sendQueue;
}
