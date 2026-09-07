import { describe, expect, it, vi } from 'vitest'
import type { BusinessDefinition } from '../../services/api/business'
import { businessDefinitionDisplayName } from './builtin-event-labels'

describe('native business event labels', () => {
 it('translates protected native events while preserving custom display names', () => {
  const translate = vi.fn(() => 'translated native label')
  for (const channel of ['sms', 'push', 'whatsapp', 'webhook']) {
   for (const suffix of ['sent', 'delivered', 'opened', 'clicked', 'failed', 'bounced', 'complained', 'unsubscribed', 'updated']) {
    const definition: BusinessDefinition = { kind: 'event', key: `${channel}.${suffix}`, name: 'server name', schema: { type: 'object' }, status: 'active', settings: { builtin: true } }
    expect(businessDefinitionDisplayName(definition, { _: translate })).toBe('translated native label')
    expect(businessDefinitionDisplayName({ ...definition, settings: {} }, { _: translate })).toBe('server name')
   }
  }
  expect(translate).toHaveBeenCalledTimes(36)
 })
})
