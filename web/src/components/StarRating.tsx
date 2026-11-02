interface InputProps {
  value: number
  onChange: (v: number) => void
}

export function StarInput({ value, onChange }: InputProps) {
  return (
    <div className="star-input" role="radiogroup" aria-label="별점">
      {[1, 2, 3, 4, 5].map((n) => (
        <button
          key={n}
          type="button"
          role="radio"
          aria-checked={value === n}
          aria-label={`${n}점`}
          className={n <= value ? 'on' : ''}
          onClick={() => onChange(n)}
        >
          ★
        </button>
      ))}
    </div>
  )
}

export function Stars({ value }: { value: number }) {
  const full = Math.round(value)
  return (
    <span className="stars" aria-label={`5점 만점에 ${value}점`}>
      {'★'.repeat(full)}
      <span className="stars__off">{'★'.repeat(5 - full)}</span>
    </span>
  )
}
