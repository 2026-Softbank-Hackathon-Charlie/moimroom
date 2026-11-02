import type { Category } from '../api'

const PALETTE: Record<Category, [string, string]> = {
  study: ['#dff3ec', '#b9e3d4'],
  meeting: ['#e3ebfb', '#bfd0f3'],
  party: ['#fbe9e3', '#f4c9bb'],
}

interface Props {
  category: Category
  label: string
}

/** Soft illustrated stand-in used until a host uploads photos. */
export default function PhotoPlaceholder({ category, label }: Props) {
  const [from, to] = PALETTE[category]
  const gid = `ph-${category}`
  return (
    <svg className="photo-placeholder" viewBox="0 0 320 200" role="img" aria-label={`${label} 사진 준비 중`}>
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={from} />
          <stop offset="1" stopColor={to} />
        </linearGradient>
      </defs>
      <rect width="320" height="200" fill={`url(#${gid})`} />
      <g fill="none" stroke="#ffffff" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" opacity="0.9">
        <rect x="196" y="36" width="72" height="56" rx="6" />
        <path d="M232 36v56M196 64h72" />
        {category === 'party' ? (
          <>
            <circle cx="92" cy="70" r="16" />
            <circle cx="122" cy="58" r="14" />
            <path d="M92 86l-4 40M122 72l2 54" />
          </>
        ) : (
          <path d="M60 128h140M76 128v34M184 128v34M96 128v-22h28v22" />
        )}
        <path d="M40 166h240" />
      </g>
    </svg>
  )
}
