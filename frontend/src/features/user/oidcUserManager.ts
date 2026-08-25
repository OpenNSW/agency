import { UserManager, WebStorageStateStore } from 'oidc-client-ts'
import { getEnv, getQueryParamsEnv, getRequiredEnv } from '@/runtimeConfig'

const rawScopes = getEnv('VITE_IDP_SCOPES')
const scope = rawScopes
  ? rawScopes
      .split(',')
      .map((s) => s.trim())
      .join(' ')
  : 'openid profile email ou role agency:application:read agency:application:review agency:application:feedback agency:consignment:read agency:storage:read agency:storage:write'

// Named by config rather than hard-coded, so the portal is not tied to one IdP's
// dialect: ThunderID wants RFC 8707 `resource`, another might want `audience`.
const idpExtraQueryParams = getQueryParamsEnv('VITE_IDP_EXTRA_QUERY_PARAMS')

export const userManager = new UserManager({
  authority: getRequiredEnv('VITE_IDP_BASE_URL'),
  client_id: getRequiredEnv('VITE_IDP_CLIENT_ID'),
  redirect_uri: getEnv('VITE_APP_URL') ?? window.location.origin,
  post_logout_redirect_uri: getEnv('VITE_APP_URL') ?? window.location.origin,
  scope,
  extraQueryParams: idpExtraQueryParams,
  userStore: new WebStorageStateStore({ store: window.sessionStorage }),
  automaticSilentRenew: true,
})
