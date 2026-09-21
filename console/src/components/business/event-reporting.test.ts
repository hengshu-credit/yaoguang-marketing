import { describe, expect, it } from 'vitest'
import type { BusinessDefinition } from '../../services/api/business'
import { durationUnitFor, exactBrowserOrigin, isObjectPath, reportingIssues, reportingRequest, reportingSample, schemaExample, setOptionalMappingPath } from './event-reporting'

const event: BusinessDefinition = {
  kind: 'event', key: 'order.paid', name: 'Paid', status: 'draft',
  schema: { type: 'object', properties: { order_id: { type: 'string' }, amount: { type: 'number', minimum: 5 }, tags: { type: 'array', items: { type: 'string', enum: ['vip'] } }, approved: { type: 'boolean' } } },
  settings: { subject_type: 'order', subject_id_path: '$.data.order_id', expiry_mode: 'duration', ttl_seconds: 90 }
}
const now = '2026-09-21T10:00:00.000Z'

describe('event reporting configuration contract', () => {
  it('preserves legacy required expiration and explicit whole-second durations', () => {
    expect(durationUnitFor(90)).toBe('seconds')
    expect(durationUnitFor(3600)).toBe('hours')
    expect(reportingSample({ ...event, settings: {} }, [], now)).toHaveProperty('expires_at', '2026-09-22T10:00:00.000Z')
    expect(reportingSample(event, [], now)).not.toHaveProperty('expires_at')
    expect(reportingIssues({ ...event, settings: { expiry_mode: 'duration', ttl_seconds: 0.5 } })).toContainEqual({ field: 'settings.ttl_seconds', code: 'ttl' })
  })
  it('does not treat an empty source restriction as unrestricted', () => {
    expect(reportingIssues({ ...event, settings: { allowed_sources: [] } })).toContainEqual({ field: 'settings.allowed_sources', code: 'sources' })
    expect(reportingIssues({ ...event, settings: {} })).toEqual([])
  })
  it('normalizes origins without turning a page URL into broader access', () => {
    expect(exactBrowserOrigin(' HTTPS://App.Example.com:443/ ')).toBe('https://app.example.com')
    expect(exactBrowserOrigin('https://app.example.com/path')).toBeUndefined()
    expect(exactBrowserOrigin('https://user:secret@app.example.com')).toBeUndefined()
    expect(exactBrowserOrigin('https://*.example.com')).toBeUndefined()
    expect(reportingIssues({ ...event, settings: { browser_enabled: true, authoritative: true } }).map(issue => issue.code)).toContain('browser_conflict')
  })
  it('removes optional mappings instead of serializing an invalid empty path', () => {
    expect(setOptionalMappingPath({ data: '$.payload', expires_at: '$.until' }, 'expires_at', '')).toEqual({ data: '$.payload' })
    expect(isObjectPath('$.data.orders[0]')).toBe(false)
    expect(isObjectPath('$.data.__proto__')).toBe(false)
  })
  it('requires customer resolution and the fixed event expiration in a publishable mapping', () => {
    const mapping: BusinessDefinition = { ...event, kind: 'event_mapping', settings: { event_code: event.key, fields: { report_id: '$.id', data: '$.data', triggered_at: '$.time' } } }
    const target = { ...event, settings: { expiry_mode: 'required' } }
    expect(reportingIssues(mapping, true, [target])).toEqual([
      { field: 'settings.fields.customer_attributes', code: 'required_mapping' },
      { field: 'settings.fields.expires_at', code: 'required_mapping' }
    ])
    mapping.settings.attribute_mappings = [{ attribute: 'customer_id', path: '$.data.customer_id', action: 'lookup' }]
    expect(reportingIssues(mapping).map(issue => issue.field)).not.toContain('settings.fields.customer_attributes')
  })
  it('generates a typed nested data example without confusing it with the request wrapper', () => {
    expect(reportingSample({ ...event, schema: {}, settings: {} }, [], now).data).toEqual({})
    const sample = reportingSample(event, [], now)
    expect(sample.data).toEqual({ order_id: 'example', amount: 5, tags: ['vip'], approved: false })
    expect(sample).toHaveProperty('customer_attributes.external_user_id')
    expect(sample).not.toHaveProperty('workspace_id')
    expect(reportingRequest('ws', event, sample, 'erp')).toEqual({ workspace_id: 'ws', source: 'erp', event: sample })
    expect(schemaExample({ type: 'integer', minimum: 1.2 }, now)).toBe(2)
  })
  it('inverts an external mapping once and carries its mapping key', () => {
    const mapping: BusinessDefinition = { ...event, kind: 'event_mapping', key: 'external', settings: { event_code: event.key, fields: { report_id: '$.request.id', data: '$.payload', customer_attributes: '$.customer', triggered_at: '$.time' } } }
    const sample = reportingSample(mapping, [event], now)
    expect(sample).toHaveProperty('request.id', 'example-occurrence-001')
    expect(sample).toHaveProperty('payload.amount', 5)
    expect(sample).not.toHaveProperty('event_code')
    expect(reportingRequest('ws', mapping, sample, 'erp')).toHaveProperty('mapping_key', 'external')
  })
  it('reports path conflicts instead of silently overwriting another field', () => {
    const mapping: BusinessDefinition = { ...event, kind: 'event_mapping', settings: { fields: { report_id: '$.id', data: '$.id.data' } } }
    expect(() => reportingSample(mapping, [], now)).toThrow('overlapping_paths')
  })
})
