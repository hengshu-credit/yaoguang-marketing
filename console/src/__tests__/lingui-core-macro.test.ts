import { afterEach, describe, expect, it } from 'vitest'
import { i18n } from '@lingui/core'
import { msg, t } from '@lingui/core/macro'
import { getNodeLabel } from '../components/automations/utils/flowConverter'

afterEach(() => {
  i18n.loadAndActivate({ locale: 'en', messages: {} })
})

describe('shared Lingui core macro test adapter', () => {
  it('resolves the real node label through the active catalogue', () => {
    i18n.loadAndActivate({ locale: 'zh-CN', messages: { 'Event or state condition': '事件或状态条件' } })
    expect(getNodeLabel('business_condition')).toBe('事件或状态条件')
    i18n.loadAndActivate({ locale: 'fr', messages: { 'Event or state condition': "Condition d’événement ou d’état" } })
    expect(getNodeLabel('business_condition')).toBe('Condition d’événement ou d’état')
  })
  it('keeps numeric and empty-string interpolation values', () => {
    i18n.loadAndActivate({ locale: 'en', messages: {} })
    expect(t`Completed ${0} tasks`).toBe('Completed 0 tasks')
    expect(t`Value: ${''}`).toBe('Value: ')
  })
  it('translates descriptors without dropping their named values', () => {
    i18n.loadAndActivate({ locale: 'fr', messages: { 'Welcome {name}': 'Bienvenue {name}', 'static.greeting': 'Bonjour' } })
    expect(t`Welcome ${{ name: 'Ada' }}`).toBe('Bienvenue Ada')
    expect(t({ id: 'static.greeting', message: 'Hello' })).toBe('Bonjour')
    expect(msg`Static label`).toMatchObject({ id: 'Static label', message: 'Static label' })
  })
})
