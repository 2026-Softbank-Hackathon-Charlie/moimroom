import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, generateSpaceBrief, type AIBrief } from '../api'
import AIUsageGuide from './AIUsageGuide'

vi.mock('../api', async (original) => ({
  ...await original<typeof import('../api')>(),
  generateSpaceBrief: vi.fn(),
}))

const generate = vi.mocked(generateSpaceBrief)
const gcp: AIBrief = { text: '요약: 조용한 공간\n시설: 와이파이\n주의: 금연\n[SPACE-7]', requestId: 'gcp-request', backend: 'gcp', durationMs: 2500 }
beforeEach(() => { generate.mockReset() })

describe('AIUsageGuide', () => {
  it('waits for a click, disables duplicate requests, and shows actual response provenance', async () => {
    const user = userEvent.setup()
    let complete!: (value: AIBrief) => void
    generate.mockImplementationOnce(() => new Promise((resolve) => { complete = resolve }))
    render(<AIUsageGuide spaceId={7} />)
    const panel = screen.getByRole('region', { name: 'AI 이용 안내' })
    expect(generate).not.toHaveBeenCalled()
    await user.click(within(panel).getByRole('button', { name: 'AI 이용 안내 만들기' }))
    const pending = within(panel).getByRole('button', { name: 'AI 안내 생성 중' })
    expect(pending).toBeDisabled()
    await user.click(pending)
    expect(generate).toHaveBeenCalledExactlyOnceWith(7, expect.any(AbortSignal))
    expect(panel).toHaveAttribute('aria-busy', 'true')
    expect(within(panel).queryByText('GCP')).not.toBeInTheDocument()
    await act(async () => { complete(gcp) })
    expect(within(panel).getByText('GCP')).toBeInTheDocument()
    expect(within(panel).getByText('gcp-request')).toBeInTheDocument()
    expect(within(panel).getByText('2500ms')).toBeInTheDocument()
    expect(panel).toHaveAttribute('aria-busy', 'false')
  })

  it('contains errors and retries only on the explicit retry click', async () => {
    const user = userEvent.setup()
    generate.mockRejectedValueOnce(new ApiError(504, '잠시 후 다시 시도해 주세요.', 'failed-request'))
    generate.mockResolvedValueOnce({ ...gcp, requestId: 'laptop-request', backend: 'laptop', durationMs: 5250 })
    render(<AIUsageGuide spaceId={7} />)
    await user.click(screen.getByRole('button', { name: 'AI 이용 안내 만들기' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('잠시 후 다시 시도해 주세요.')
    expect(screen.getByText('요청 ID: failed-request')).toBeInTheDocument()
    expect(generate).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: '다시 시도' }))
    expect(await screen.findByText('노트북')).toBeInTheDocument()
    expect(screen.getByText('laptop-request')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(generate).toHaveBeenCalledTimes(2)
  })

  it('keeps an existing response bound to GCP until a new laptop response completes', async () => {
    const user = userEvent.setup()
    generate.mockResolvedValueOnce(gcp)
    const { rerender } = render(<AIUsageGuide spaceId={7} />)
    await user.click(screen.getByRole('button', { name: 'AI 이용 안내 만들기' }))
    await screen.findByText('GCP')
    rerender(<AIUsageGuide spaceId={7} />)
    expect(screen.getByText('gcp-request')).toBeInTheDocument()
    expect(generate).toHaveBeenCalledTimes(1)
    generate.mockResolvedValueOnce({ ...gcp, backend: 'laptop', requestId: 'next-request' })
    await user.click(screen.getByRole('button', { name: 'AI 안내 다시 만들기' }))
    expect(await screen.findByText('노트북')).toBeInTheDocument()
    expect(screen.queryByText('gcp-request')).not.toBeInTheDocument()
  })

  it('renders generated text as text and clears the result for a different space key', async () => {
    const user = userEvent.setup()
    generate.mockResolvedValueOnce({ ...gcp, text: '<img src=x onerror=alert(1)>' })
    const { rerender } = render(<AIUsageGuide key={7} spaceId={7} />)
    await user.click(screen.getByRole('button', { name: 'AI 이용 안내 만들기' }))
    expect(await screen.findByText('<img src=x onerror=alert(1)>')).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    rerender(<AIUsageGuide key={8} spaceId={8} />)
    expect(screen.queryByText('gcp-request')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'AI 이용 안내 만들기' })).toBeEnabled()
    expect(generate).toHaveBeenCalledTimes(1)
  })

  it('blocks duplicate clicks in the same update', () => {
    generate.mockImplementation(() => new Promise(() => {}))
    render(<AIUsageGuide spaceId={7} />)
    const button = screen.getByRole('button', { name: 'AI 이용 안내 만들기' })
    act(() => {
      fireEvent.click(button)
      fireEvent.click(button)
    })
    expect(generate).toHaveBeenCalledTimes(1)
  })

  it('aborts a previous space request and ignores its late response while the new request is pending', async () => {
    const user = userEvent.setup()
    let completeOld!: (value: AIBrief) => void
    let completeNew!: (value: AIBrief) => void
    generate.mockImplementationOnce(() => new Promise((resolve) => { completeOld = resolve }))
    generate.mockImplementationOnce(() => new Promise((resolve) => { completeNew = resolve }))
    const { rerender } = render(<AIUsageGuide key={7} spaceId={7} />)
    await user.click(screen.getByRole('button', { name: 'AI 이용 안내 만들기' }))
    const oldSignal = generate.mock.calls[0][1]!
    rerender(<AIUsageGuide key={8} spaceId={8} />)
    expect(oldSignal.aborted).toBe(true)
    expect(generate).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: 'AI 이용 안내 만들기' }))
    await act(async () => { completeOld(gcp) })
    expect(screen.queryByText('gcp-request')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'AI 안내 생성 중' })).toBeDisabled()
    await act(async () => { completeNew({ ...gcp, text: '[SPACE-8]', backend: 'laptop', requestId: 'new-space-request' }) })
    expect(screen.getByText('new-space-request')).toBeInTheDocument()
    expect(screen.getByText('[SPACE-8]')).toBeInTheDocument()
    expect(screen.queryByText('gcp-request')).not.toBeInTheDocument()
  })

  it('aborts on unmount and safely ignores a late rejection', async () => {
    const user = userEvent.setup()
    let fail!: (error: unknown) => void
    generate.mockImplementationOnce(() => new Promise((_resolve, reject) => { fail = reject }))
    const { unmount } = render(<AIUsageGuide spaceId={7} />)
    await user.click(screen.getByRole('button', { name: 'AI 이용 안내 만들기' }))
    const signal = generate.mock.calls[0][1]!
    unmount()
    expect(signal.aborted).toBe(true)
    await act(async () => { fail(new ApiError(0, 'cancelled')) })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(generate).toHaveBeenCalledTimes(1)
  })
})
