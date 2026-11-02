import type { ReactNode } from 'react'
import { Link, NavLink } from 'react-router'

export function Logo() {
  return (
    <Link to="/" className="logo" aria-label="모임룸 홈">
      <svg viewBox="0 0 64 64" width="28" height="28" aria-hidden="true">
        <rect width="64" height="64" rx="16" fill="currentColor" />
        <path
          d="M20 48V22a6 6 0 0 1 6-6h12a6 6 0 0 1 6 6v26M14 48h36"
          fill="none"
          stroke="#fff"
          strokeWidth="5"
          strokeLinecap="round"
        />
        <circle cx="37" cy="34" r="3" fill="#fff" />
      </svg>
      <span>모임룸</span>
    </Link>
  )
}

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <div className="app">
      <header className="site-header">
        <div className="container site-header__inner">
          <Logo />
          <nav className="site-nav" aria-label="주요 메뉴">
            <NavLink to="/spaces">공간 찾기</NavLink>
            <NavLink to="/reservations">예약 확인</NavLink>
            <NavLink to="/host">호스트</NavLink>
          </nav>
        </div>
      </header>
      <main className="site-main">{children}</main>
      <footer className="site-footer">
        <div className="container site-footer__inner">
          <div>
            <strong>모임룸</strong>
            <p>스터디룸, 회의실, 파티룸을 시간 단위로 예약하는 공간 예약 서비스</p>
          </div>
          <nav aria-label="바로가기">
            <Link to="/spaces">공간 찾기</Link>
            <Link to="/reservations">예약 확인</Link>
            <Link to="/host">호스트 센터</Link>
          </nav>
          <p className="copyright">© 2026 Moimroom</p>
        </div>
      </footer>
    </div>
  )
}

export function Loading({ label = '불러오는 중입니다' }: { label?: string }) {
  return (
    <div className="state" role="status">
      <span className="spinner" aria-hidden="true" />
      {label}
    </div>
  )
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="state state--error" role="alert">
      <p>{message}</p>
      {onRetry && (
        <button type="button" className="btn btn--ghost" onClick={onRetry}>
          다시 시도
        </button>
      )}
    </div>
  )
}
