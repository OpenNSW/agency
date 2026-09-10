import { useTranslation } from 'react-i18next'
import { SignedIn } from './Auth'
import { Button } from '@radix-ui/themes'
import { useSignOutHandler } from './hooks/useSignOutHandler'
import { Footer } from '@/components/Layout/Footer'

export function UnauthorizedScreen() {
  const { t } = useTranslation()
  const handleSignOut = useSignOutHandler()

  return (
    <div className="min-h-screen bg-gray-50 relative pb-16 sm:pb-12">
      <main className="mt-16 min-h-[calc(100vh-64px)] flex items-center justify-center px-6">
        <div className="w-full max-w-lg rounded-xl border border-gray-200 bg-white p-8 shadow-sm text-center">
          <h1 className="text-2xl font-semibold text-gray-900">{t('auth.unauthorized.title')}</h1>
          <p className="mt-3 text-gray-600">{t('auth.unauthorized.message')}</p>
          <div className="mt-8 flex items-center justify-center">
            <SignedIn>
              <Button onClick={handleSignOut} size="4" style={{ cursor: 'pointer' }}>
                {t('auth.signOut')}
              </Button>
            </SignedIn>
          </div>
        </div>
      </main>
      <Footer />
    </div>
  )
}
