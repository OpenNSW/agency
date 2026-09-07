import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { Theme } from '@radix-ui/themes'
import { AuthProvider } from 'react-oidc-context'
import '@radix-ui/themes/styles.css'
import './index.css'
import App from './App.tsx'
import { initAppConfig } from './config.ts'
import './i18n'
import { userManager } from '@/features/user/oidcUserManager'

initAppConfig()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AuthProvider
      userManager={userManager}
      onSigninCallback={() => {
        window.history.replaceState({}, document.title, window.location.pathname)
      }}
    >
      <Theme scaling="110%">
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </Theme>
    </AuthProvider>
  </StrictMode>,
)
