/**
 * API error types.
 *
 * These live apart from client.ts because that module imports the router, and the router
 * builds routes at module scope. A component that only wants to inspect an error's status
 * would otherwise drag the whole router graph into its test environment, where a shallow
 * router mock cannot satisfy it.
 */
import { msg } from '@lingui/core/macro'
import type { MessageDescriptor } from '@lingui/core'
import { i18n } from '../../i18n'
import { ALL_PERMISSION_RESOURCES } from './permissions'
import type { PermissionResource, ResourcePermissions } from './permissions'

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public data?: unknown
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export interface APIFieldError {
  field: string
  message: string
}

export interface APIErrorDetails {
  code?: string
  message?: string
  requestId?: string
  retryAfter?: string
  fieldErrors: APIFieldError[]
}

export interface ActionableErrorDescription extends APIErrorDetails {
  title: string
  impact: string
  nextStep: string
  retryable: boolean
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

function optionalString(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function normalizeFieldErrors(value: unknown): APIFieldError[] {
  if (Array.isArray(value)) {
    return value.flatMap((item) => {
      const record = asRecord(item)
      const field = optionalString(record?.field)
      const message = optionalString(record?.message)
      return field && message ? [{ field, message }] : []
    })
  }
  const record = asRecord(value)
  if (!record) return []
  return Object.entries(record).flatMap(([field, message]) => {
    const normalized = optionalString(message)
    return normalized ? [{ field, message: normalized }] : []
  })
}

/** Normalize both the Yaoguang structured envelope and legacy `{ error: string }` bodies. */
export function apiErrorDetails(body: unknown): APIErrorDetails {
  const root = asRecord(body)
  if (!root) return { fieldErrors: [] }
  const nested = asRecord(root.error)
  const message = nested
    ? optionalString(nested.message)
    : optionalString(root.error) ?? optionalString(root.message)
  return {
    ...(optionalString(nested?.code ?? root.code) ? { code: optionalString(nested?.code ?? root.code) } : {}),
    ...(message ? { message } : {}),
    ...(optionalString(root.request_id ?? root.trace_id ?? nested?.request_id ?? nested?.trace_id)
      ? { requestId: optionalString(root.request_id ?? root.trace_id ?? nested?.request_id ?? nested?.trace_id) }
      : {}),
    ...(optionalString(root.retry_after ?? nested?.retry_after)
      ? { retryAfter: optionalString(root.retry_after ?? nested?.retry_after) }
      : {}),
    fieldErrors: normalizeFieldErrors(nested?.field_errors ?? root.field_errors)
  }
}

/** Convert a technical failure into language that tells an operator what happened and what to do. */
export function describeApiError(error: unknown): ActionableErrorDescription {
  if (!(error instanceof ApiError)) {
    return {
      title: i18n._(msg`Unable to reach the server`),
      impact: i18n._(msg`Your changes were not submitted.`),
      nextStep: i18n._(msg`Check your connection and try again.`),
      retryable: true,
      message: error instanceof Error ? error.message : undefined,
      fieldErrors: []
    }
  }

  const details = apiErrorDetails(error.data)
  const code = details.code?.toLowerCase() ?? ''
  const technicalMessage = details.message ?? error.message
  const base = { ...details, message: technicalMessage }

  if (code.includes('unknown_delivery')) {
    return {
      ...base,
      title: i18n._(msg`Delivery outcome is uncertain`),
      impact: i18n._(msg`The provider has not confirmed whether the customer was contacted.`),
      nextStep: i18n._(msg`Verify the outcome with the provider before retrying to avoid a duplicate contact.`),
      retryable: false
    }
  }
  if (code.includes('frequency') || (error.status === 429 && /frequency/i.test(technicalMessage))) {
    return {
      ...base,
      title: i18n._(msg`Delivery blocked by frequency control`),
      impact: i18n._(msg`The customer will not receive this delivery.`),
      nextStep: i18n._(msg`Review the activity and global frequency policies before changing the limit.`),
      retryable: false
    }
  }
  if (error.status === 409) {
    return {
      ...base,
      title: i18n._(msg`This item changed before your update was saved`),
      impact: i18n._(msg`Your changes were not applied.`),
      nextStep: i18n._(msg`Reload the latest version, review the differences, and submit again.`),
      retryable: false
    }
  }
  if (error.status === 429) {
    return {
      ...base,
      title: i18n._(msg`Too many requests right now`),
      impact: i18n._(msg`This operation has not completed.`),
      nextStep: details.retryAfter
        ? i18n._(msg`Wait until ${details.retryAfter}, then try again.`)
        : i18n._(msg`Wait briefly, then try again.`),
      retryable: true
    }
  }
  if (error.status === 503) {
    return {
      ...base,
      title: i18n._(msg`Service temporarily unavailable`),
      impact: i18n._(msg`This operation is delayed; no data was discarded.`),
      nextStep: i18n._(msg`Try again in a moment. If it continues, give support the request ID below.`),
      retryable: true
    }
  }
  if (details.fieldErrors.length > 0 || error.status === 400 || error.status === 422) {
    return {
      ...base,
      title: i18n._(msg`Some information needs attention`),
      impact: i18n._(msg`This form was not submitted.`),
      nextStep: i18n._(msg`Correct the fields listed below and submit again.`),
      retryable: false
    }
  }
  if (error.status === 401 || error.status === 403) {
    return {
      ...base,
      title: i18n._(msg`You cannot complete this operation`),
      impact: technicalMessage,
      nextStep: i18n._(msg`Ask a workspace owner to check your access, then try again.`),
      retryable: false
    }
  }
  return {
    ...base,
    title: i18n._(msg`We could not complete this operation`),
    impact: i18n._(msg`The requested change may not have been applied.`),
    nextStep: i18n._(msg`Try again. If it continues, give support the request ID below.`),
    retryable: true
  }
}

/**
 * The query client's retry policy.
 *
 * An expired session and a missing permission are not transient: the second attempt is refused
 * exactly like the first, and repeating it only holds the error back by one retryDelay — long
 * enough that a scoped API key's denial looks like a hung page rather than an answer. Everything
 * else keeps the single retry the client had before.
 */
export function shouldRetryQuery(failureCount: number, error: Error): boolean {
  if (error instanceof ApiError && (error.status === 401 || error.status === 403)) {
    return false
  }
  return failureCount < 1
}

// The two verbs a grant is made of, taken from the grant itself so the pair cannot drift.
export type PermissionVerb = keyof ResourcePermissions

export interface PermissionDenial {
  resource: PermissionResource
  permission: PermissionVerb
}

// Wording follows the Read/Write matrix on the Team page, so an owner reading a denial knows
// which switch to flip. Lazy descriptors, not `t`: these are built at module scope, before any
// catalog is active, and are rendered through i18n._() at the moment the message is needed.
export const RESOURCE_LABELS: Record<PermissionResource, MessageDescriptor> = {
  contacts: msg`Contacts`,
  customers: msg`Customers`,
  lists: msg`Lists`,
  templates: msg`Templates`,
  broadcasts: msg`Broadcasts`,
  transactional: msg`Transactional`,
  workspace: msg`Workspace`,
  message_history: msg`Message History`,
  blog: msg`Blog`,
  automations: msg`Automations`,
  llm: msg`LLM`,
  web_analytics: msg`Web Analytics`,
  segments: msg`Segments`,
  webhook_subscriptions: msg`Webhook Subscriptions`,
  webhook_events: msg`Webhook Events`
}

const PERMISSION_VERBS: PermissionVerb[] = ['read', 'write']

/**
 * Reads the denial fields off a parsed error body.
 *
 * Detection is by the presence of `resource` and `permission`, never by status: 403 is also the
 * answer to a workspace-access failure and to the quota errors (workspace limit, team member
 * limit) that two call sites string-match to raise an upgrade prompt. Those bodies carry the
 * `error` key alone, so they fall through here and keep their own message.
 *
 * A resource the console does not know — a backend newer than this bundle — also returns null,
 * which leaves the server's own English sentence in place rather than inventing a label for it.
 */
export function permissionDenialFromBody(body: unknown): PermissionDenial | null {
  if (!body || typeof body !== 'object') return null

  const { resource, permission } = body as { resource?: unknown; permission?: unknown }
  if (typeof resource !== 'string' || typeof permission !== 'string') return null
  if (!ALL_PERMISSION_RESOURCES.includes(resource as PermissionResource)) return null
  if (!PERMISSION_VERBS.includes(permission as PermissionVerb)) return null

  return {
    resource: resource as PermissionResource,
    permission: permission as PermissionVerb
  }
}

/**
 * Reads the denial fields off a thrown error, for call sites that want the missing grant rather
 * than the sentence — ApiError keeps the parsed body on `data`.
 */
export function permissionDenial(err: unknown): PermissionDenial | null {
  return err instanceof ApiError ? permissionDenialFromBody(err.data) : null
}

/**
 * The console's own sentence for a denial, built from the resource and verb rather than by
 * decorating the server's English string — which is not translatable and would read twice.
 */
export function permissionDeniedMessage({ resource, permission }: PermissionDenial): string {
  const label = i18n._(RESOURCE_LABELS[resource])
  return permission === 'read'
    ? i18n._(msg`You do not have read access to ${{ resource: label }}.`)
    : i18n._(msg`You do not have write access to ${{ resource: label }}.`)
}
