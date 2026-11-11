import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, generateSpaceBrief } from './api'

afterEach(() => vi.unstubAllGlobals())

describe('generateSpaceBrief', () => {
  it('uses only the same-origin bodyless POST and retains request metadata', async () => {
    const result = { text: '요약: 조용한 공간\n시설: 와이파이\n주의: 금연\n[SPACE-7]', requestId: 'request-gcp', backend: 'gcp', durationMs: 2500 }
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify(result)))
    vi.stubGlobal('fetch', fetch)
    await expect(generateSpaceBrief(7)).resolves.toEqual(result)
    const [url, init] = fetch.mock.calls[0] as [string, RequestInit]
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(url).toBe('/api/spaces/7/ai-brief')
    expect(init.method).toBe('POST')
    expect(init.body).toBeUndefined()
    expect(new Headers(init.headers).has('X-Host-Code')).toBe(false)
    expect(new Headers(init.headers).has('Authorization')).toBe(false)
    expect(new Headers(init.headers).has('Content-Type')).toBe(false)
  })

  it('forwards cancellation without retrying the POST', async () => {
    const controller = new AbortController()
    const fetch = vi.fn().mockRejectedValue(new DOMException('Aborted', 'AbortError'))
    vi.stubGlobal('fetch', fetch)
    const pending = generateSpaceBrief(7, controller.signal)
    controller.abort()
    await expect(pending).rejects.toBeInstanceOf(ApiError)
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch.mock.calls[0][1].signal).toBe(controller.signal)
    expect(fetch.mock.calls[0][1].signal.aborted).toBe(true)
  })

  it('keeps error request ID without showing upstream details or retrying', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ error: 'http://private-inference.invalid:11434 failed' }),
      { status: 502, headers: { 'X-Request-Id': 'request-failed' } },
    ))
    vi.stubGlobal('fetch', fetch)
    const error = await generateSpaceBrief(7).catch((e: unknown) => e) as ApiError
    expect(error).toBeInstanceOf(ApiError)
    expect(error.status).toBe(502)
    expect(error.requestId).toBe('request-failed')
    expect(error.message).not.toContain('private-inference')
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('retains the existing user-facing 429 message', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ error: '요청이 진행 중입니다. 잠시 후 다시 시도해 주세요.' }),
      { status: 429, headers: { 'X-Request-Id': 'request-busy' } },
    )))
    await expect(generateSpaceBrief(7)).rejects.toMatchObject({
      status: 429, requestId: 'request-busy', message: '요청이 진행 중입니다. 잠시 후 다시 시도해 주세요.',
    })
  })
})
