import { afterEach, describe, expect, it } from 'vitest'
import { appConfig, initAppConfig } from './config'

afterEach(() => {
  delete window.__APP_CONFIG__
})

describe('initAppConfig', () => {
  it('fills in portalName/description when a deployment sets only the required fields', () => {
    window.__APP_CONFIG__ = {
      branding: { systemName: 'NSW', appName: 'FCAU Officer Portal' },
    }

    initAppConfig()

    // TopBar renders portalName, and LoginScreen renders description,
    // unconditionally with no fallback of their own — a valid-but-partial
    // payload must not leave these blank.
    expect(appConfig.branding.portalName).toBeTruthy()
    expect(appConfig.branding.description).toBeTruthy()
  })

  it('keeps a deployment-provided portalName/description as-is', () => {
    window.__APP_CONFIG__ = {
      branding: {
        systemName: 'NSW',
        appName: 'FCAU Officer Portal',
        portalName: 'FCAU Portal',
        description: 'FCAU-specific description',
      },
    }

    initAppConfig()

    expect(appConfig.branding.portalName).toBe('FCAU Portal')
    expect(appConfig.branding.description).toBe('FCAU-specific description')
  })

  it('falls back to hardcoded defaults when branding is missing entirely', () => {
    initAppConfig()

    expect(appConfig.branding.systemName).toBeTruthy()
    expect(appConfig.branding.appName).toBeTruthy()
    expect(appConfig.branding.portalName).toBeTruthy()
    expect(appConfig.branding.description).toBeTruthy()
  })

  it('falls back to hardcoded defaults when branding fails validation', () => {
    window.__APP_CONFIG__ = { branding: { systemName: '', appName: '' } }

    initAppConfig()

    expect(appConfig.branding.systemName).toBeTruthy()
    expect(appConfig.branding.appName).toBeTruthy()
  })
})
