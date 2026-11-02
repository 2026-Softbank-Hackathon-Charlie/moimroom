import { useEffect } from 'react'
import { Link, Route, Routes, useLocation } from 'react-router'
import Layout from './components/Layout'
import BookingCompletePage from './pages/BookingCompletePage'
import BookingPage from './pages/BookingPage'
import HomePage from './pages/HomePage'
import HostPage from './pages/HostPage'
import LookupPage from './pages/LookupPage'
import ReviewPage from './pages/ReviewPage'
import SpaceDetailPage from './pages/SpaceDetailPage'
import SpaceListPage from './pages/SpaceListPage'

function NotFound() {
  return (
    <div className="container page narrow center">
      <h1 className="page__title">페이지를 찾을 수 없습니다</h1>
      <p className="muted">주소가 바뀌었거나 삭제된 페이지입니다.</p>
      <Link to="/" className="btn btn--primary">
        홈으로
      </Link>
    </div>
  )
}

function ScrollToTop() {
  const { pathname } = useLocation()
  useEffect(() => window.scrollTo(0, 0), [pathname])
  return null
}

export default function App() {
  return (
    <Layout>
      <ScrollToTop />
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/spaces" element={<SpaceListPage />} />
        <Route path="/spaces/:id" element={<SpaceDetailPage />} />
        <Route path="/spaces/:id/book" element={<BookingPage />} />
        <Route path="/booking/complete" element={<BookingCompletePage />} />
        <Route path="/reservations" element={<LookupPage />} />
        <Route path="/reviews/new" element={<ReviewPage />} />
        <Route path="/host" element={<HostPage />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </Layout>
  )
}
