import { describe, it, expect, afterEach, vi, beforeEach } from 'vitest'
import { parseRootEmails, isRootUser, authService } from './auth'
import { ApiError } from './client'

function setRootEmail(value: string | undefined) {
  ;(window as unknown as { ROOT_EMAIL?: string }).ROOT_EMAIL = value as string
}

describe('parseRootEmails', () => {
  it('returns [] for empty or undefined', () => {
    expect(parseRootEmails(undefined)).toEqual([])
    expect(parseRootEmails('')).toEqual([])
  })

  it('parses a single email', () => {
    expect(parseRootEmails('alice@example.com')).toEqual(['alice@example.com'])
  })

  it('parses comma-separated emails', () => {
    expect(parseRootEmails('alice@example.com,bob@example.com')).toEqual([
      'alice@example.com',
      'bob@example.com'
    ])
  })

  it('parses semicolon-separated emails', () => {
    expect(parseRootEmails('alice@example.com;bob@example.com')).toEqual([
      'alice@example.com',
      'bob@example.com'
    ])
  })

  it('trims whitespace and drops empty entries', () => {
    expect(parseRootEmails(' alice@example.com , , bob@example.com ')).toEqual([
      'alice@example.com',
      'bob@example.com'
    ])
  })

  it('splits on whitespace (mirrors backend and the tag separators)', () => {
    expect(parseRootEmails('alice@example.com bob@example.com')).toEqual([
      'alice@example.com',
      'bob@example.com'
    ])
    expect(parseRootEmails('alice@example.com,  bob@example.com\tcarol@example.com')).toEqual([
      'alice@example.com',
      'bob@example.com',
      'carol@example.com'
    ])
  })

  it('preserves case', () => {
    expect(parseRootEmails('Alice@example.com')).toEqual(['Alice@example.com'])
  })
})

describe('isRootUser', () => {
  afterEach(() => {
    setRootEmail(undefined)
  })

  it('returns false when no user email is given', () => {
    setRootEmail('alice@example.com')
    expect(isRootUser(undefined)).toBe(false)
    expect(isRootUser('')).toBe(false)
  })

  it('returns false when ROOT_EMAIL is not set', () => {
    setRootEmail(undefined)
    expect(isRootUser('alice@example.com')).toBe(false)
  })

  it('matches a single configured root (backward compatible)', () => {
    setRootEmail('alice@example.com')
    expect(isRootUser('alice@example.com')).toBe(true)
    expect(isRootUser('bob@example.com')).toBe(false)
  })

  it('matches any email in a configured list', () => {
    setRootEmail('alice@example.com,bob@example.com')
    expect(isRootUser('alice@example.com')).toBe(true)
    expect(isRootUser('bob@example.com')).toBe(true)
    expect(isRootUser('carol@example.com')).toBe(false)
  })

  it('is case-sensitive', () => {
    setRootEmail('alice@example.com')
    expect(isRootUser('Alice@example.com')).toBe(false)
  })
})

describe('oidcExchange', () => {
  beforeEach(() => {
    ;(window as unknown as { API_ENDPOINT?: string }).API_ENDPOINT = 'https://api.example.com/'
  })
  afterEach(() => {
    vi.restoreAllMocks()
    ;(window as unknown as { API_ENDPOINT?: string }).API_ENDPOINT = undefined
  })

  it('POSTs the code in the body to /api/user.oidc.exchange with no credentials', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ token: 'jwt-x' })
    })
    vi.stubGlobal('fetch', fetchMock)

    await authService.oidcExchange('AbC123')

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    // Trailing slash on API_ENDPOINT is stripped.
    expect(url).toBe('https://api.example.com/api/user.oidc.exchange')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ code: 'AbC123' }))
    // Load-bearing cross-origin guard: a credentialed/cookie handoff must NOT regress in.
    expect('credentials' in init).toBe(false)
  })

  it('returns { token } on a 200 response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: async () => ({ token: 'jwt-y' }) })
    )
    const res = await authService.oidcExchange('code')
    expect(res).toEqual({ token: 'jwt-y' })
  })

  it('throws ApiError with the status on a non-ok response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 401,
        json: async () => ({ error: 'not_provisioned' })
      })
    )
    await expect(authService.oidcExchange('bad')).rejects.toMatchObject({
      name: 'ApiError',
      status: 401
    })
    await expect(authService.oidcExchange('bad')).rejects.toBeInstanceOf(ApiError)
  })
})
