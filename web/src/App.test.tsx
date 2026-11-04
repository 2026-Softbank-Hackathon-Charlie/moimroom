import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'

afterEach(() => vi.unstubAllGlobals())

describe('App', () => {
  // Chrome 156 returns a Promise from window.scrollTo. If that value leaks out
  // of an effect, React calls it as the cleanup on the next route change and
  // the whole page goes blank.
  it('keeps rendering after a route change when scrollTo returns a Promise', async () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})))
    vi.stubGlobal('scrollTo', vi.fn(() => Promise.resolve()))

    render(
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>,
    )
    await userEvent.click(screen.getByRole('button', { name: '공간 검색' }))

    expect(await screen.findByRole('heading', { name: '서울 전체 공간' })).toBeInTheDocument()
    expect(window.scrollTo).toHaveBeenCalledTimes(2)
  })
})
