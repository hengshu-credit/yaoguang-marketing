import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { previewRendererKind } from './clientProfiles'

const registry = readFileSync(resolve(process.cwd(), '../internal/domain/channel_catalog.go'), 'utf8')
const profiles = [...registry.matchAll(/profile\("([^"]+)"/g)].map(match => match[1])
const nativeHtmlProfiles = new Set(['email_mobile', 'email_desktop', 'web_page'])

describe('backend catalogue / frontend preview coverage', () => {
  it.each(profiles.filter(profile => !nativeHtmlProfiles.has(profile)))('renders the declared %s profile', profile => {
    expect(previewRendererKind(profile)).not.toBe('unknown')
  })
})
