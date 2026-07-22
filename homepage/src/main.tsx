import Lenis from 'lenis'
import { StrictMode, useEffect } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { useSession } from './hooks/useSession'
import { useSiteConfig } from './hooks/useSiteConfig'
import './styles.css'

function useSmoothScroll() {
  useEffect(() => {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    const lenis = new Lenis({ duration: 1.05, smoothWheel: true })
    let frame = 0
    const tick = (time: number) => {
      lenis.raf(time)
      frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => {
      cancelAnimationFrame(frame)
      lenis.destroy()
    }
  }, [])
}

function RuntimeApp() {
  useSmoothScroll()
  const config = useSiteConfig()
  const session = useSession()
  return <App config={config} session={session} />
}

createRoot(document.getElementById('root')!).render(
  <StrictMode><RuntimeApp /></StrictMode>,
)
