import { Outlet, useNavigate, useMatch } from '@tanstack/react-router'
import { Button, Spin } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useAuth } from '../contexts/AuthContext'
import { useEffect, useState } from 'react'
import { setupApi } from '../services/api/setup'

type InstallationStatus = 'checking' | 'installed' | 'uninstalled' | 'error'

async function fetchInstallationStatus(): Promise<'installed' | 'uninstalled'> {
  const status = await setupApi.getStatus()
  if (status.is_installed) {
    window.IS_INSTALLED = true
    return 'installed'
  }
  return 'uninstalled'
}

export function RootLayout() {
  const { t } = useLingui()
  const { isAuthenticated, loading, workspaces } = useAuth()
  const navigate = useNavigate()

  const isSigninRoute = useMatch({ from: '/console/signin', shouldThrow: false })
  const isAcceptInvitationRoute = useMatch({
    from: '/console/accept-invitation',
    shouldThrow: false
  })
  const isLogoutRoute = useMatch({ from: '/console/logout', shouldThrow: false })
  const isWorkspaceCreateRoute = useMatch({ from: '/console/workspace/create', shouldThrow: false })
  const isSetupRoute = useMatch({ from: '/console/setup', shouldThrow: false })

  const [installationStatus, setInstallationStatus] = useState<InstallationStatus>(() =>
    window.IS_INSTALLED === true ? 'installed' : 'checking'
  )

  const retryInstallation = () => {
    setInstallationStatus('checking')
    void fetchInstallationStatus().then(setInstallationStatus, () => setInstallationStatus('error'))
  }

  useEffect(() => {
    if (window.IS_INSTALLED === true) return

    let active = true
    void fetchInstallationStatus().then(
      (status) => {
        if (active) setInstallationStatus(status)
      },
      () => {
        if (active) setInstallationStatus('error')
      }
    )
    return () => {
      active = false
    }
  }, [])

  const isPublicRoute = isSigninRoute || isAcceptInvitationRoute || isLogoutRoute || isSetupRoute

  // A missing or stale bootstrap flag is not proof that setup is required. Only
  // the live database-backed status endpoint may send a visitor to the wizard.
  const shouldRedirectToSetup = installationStatus === 'uninstalled' && !isSetupRoute
  const shouldRedirectInstalledSetup = installationStatus === 'installed' && isSetupRoute

  // If not authenticated and not on public routes, redirect to signin
  const shouldRedirectToSignin =
    installationStatus === 'installed' &&
    !isLogoutRoute &&
    !isSigninRoute &&
    !isAuthenticated &&
    !isPublicRoute

  // If authenticated and has no workspaces, redirect to workspace creation
  const shouldRedirectToCreateWorkspace =
    installationStatus === 'installed' &&
    isAuthenticated &&
    workspaces.length === 0 &&
    !isWorkspaceCreateRoute &&
    !isLogoutRoute &&
    !isSetupRoute

  // console.log('isAuthenticated', isAuthenticated)
  // handle redirection...
  useEffect(() => {
    if (loading || installationStatus === 'checking' || installationStatus === 'error') return

    if (shouldRedirectInstalledSetup) {
      navigate({ to: '/console/signin', replace: true })
      return
    }

    if (shouldRedirectToSetup) {
      navigate({ to: '/console/setup' })
      return
    }

    if (shouldRedirectToSignin) {
      // Check if we're already on the signin pathname to avoid unnecessary navigation
      // This handles race conditions where route matching hasn't completed yet
      const currentPathname = window.location.pathname
      if (currentPathname === '/console/signin') {
        // Already on signin route, don't navigate
        return
      }

      // Preserve search parameters when redirecting to signin
      const currentSearch = window.location.search
      const searchParams = new URLSearchParams(currentSearch)
      const search: { email?: string } = {}
      
      // Preserve email parameter if present
      if (searchParams.has('email')) {
        search.email = searchParams.get('email') || undefined
      }

      navigate({ 
        to: '/console/signin',
        search: Object.keys(search).length > 0 ? search : undefined,
        replace: true
      })
      return
    }

    if (shouldRedirectToCreateWorkspace) {
      navigate({ to: '/console/workspace/create' })
      return
    }
  }, [
    loading,
    installationStatus,
    shouldRedirectInstalledSetup,
    shouldRedirectToSetup,
    shouldRedirectToSignin,
    shouldRedirectToCreateWorkspace,
    navigate
  ])

  if (installationStatus === 'error') {
    return (
      <div
        style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}
      >
        <div style={{ textAlign: 'center' }}>
          <p>{t`Failed to fetch setup status`}</p>
          <Button type="primary" onClick={retryInstallation}>
            {t`Retry`}
          </Button>
        </div>
      </div>
    )
  }

  if (
    loading ||
    installationStatus === 'checking' ||
    shouldRedirectInstalledSetup ||
    shouldRedirectToSetup ||
    shouldRedirectToSignin ||
    shouldRedirectToCreateWorkspace
  ) {
    return (
      <div
        style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}
      >
        <Spin size="large" description={t`Loading...`} fullscreen />
      </div>
    )
  }

  return <Outlet />
}
