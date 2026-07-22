import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { useSession } from './hooks/useSession'
import { useSiteConfig } from './hooks/useSiteConfig'
import './styles.css'

function RuntimeApp() {
  const config = useSiteConfig()
  const session = useSession()
  return <App config={config} session={session} />
}

createRoot(document.getElementById('root')!).render(
  <StrictMode><RuntimeApp /></StrictMode>,
)
