import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { StarInput, Stars } from './StarRating'

describe('StarRating', () => {
  it('reports the chosen rating', async () => {
    const onChange = vi.fn()
    render(<StarInput value={5} onChange={onChange} />)
    expect(screen.getByRole('radio', { name: '5점' })).toHaveAttribute('aria-checked', 'true')
    await userEvent.click(screen.getByRole('radio', { name: '3점' }))
    expect(onChange).toHaveBeenCalledWith(3)
  })

  it('renders filled and empty stars', () => {
    render(<Stars value={4} />)
    expect(screen.getByLabelText('5점 만점에 4점')).toHaveTextContent('★★★★★')
  })
})
