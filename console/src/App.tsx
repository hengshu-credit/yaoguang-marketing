import { ConfigProvider, App as AntApp } from 'antd'
import { useEffect } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { I18nProvider } from '@lingui/react'
import { useLingui } from '@lingui/react/macro'
import { router } from './router'
import { AuthProvider } from './contexts/AuthContext'
import { LocaleProvider, useLocale, i18n } from './contexts/LocaleContext'
import { initializeAnalytics } from './utils/analytics-config'
import { shouldRetryQuery } from './services/api/errors'
import enUS from 'antd/locale/en_US'
import frFR from 'antd/locale/fr_FR'
import esES from 'antd/locale/es_ES'
import deDE from 'antd/locale/de_DE'
import caES from 'antd/locale/ca_ES'
import ptBR from 'antd/locale/pt_BR'
import jaJP from 'antd/locale/ja_JP'
import itIT from 'antd/locale/it_IT'
import zhCN from 'antd/locale/zh_CN'
import type { Locale as AntdLocale } from 'antd/es/locale'
import type { Locale } from './i18n'
import { ConsoleThemeProvider, useConsoleTheme } from './theme/ConsoleThemeProvider'
import { antdConsoleTheme, DEFAULT_CONSOLE_THEME, type ConsoleTheme } from './theme/consoleTheme'

// Every locale in the app's supported set needs an entry: a missing key leaves
// ConfigProvider without a locale and antd's own strings fall back to English.
const antdLocales: Record<Locale, AntdLocale> = {
  en: enUS,
  fr: frFR,
  es: esES,
  de: deDE,
  ca: caES,
  'pt-BR': ptBR,
  ja: jaJP,
  it: itIT,
  'zh-CN': zhCN,
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: shouldRetryQuery
    }
  }
})


// Initialize analytics service
initializeAnalytics()

// Inner component that uses LocaleContext
function AppContent() {
  const { locale } = useLocale()
  const appearance = useConsoleTheme()
  const theme = antdConsoleTheme(appearance?.value ?? DEFAULT_CONSOLE_THEME as ConsoleTheme, appearance?.dark ?? false)

  return (
    // key={locale} forces I18nProvider and all children to remount when locale changes,
    // ensuring all components re-render with the new translations
    <I18nProvider i18n={i18n} key={locale}>
      <BrowserTitle />
      <ConfigProvider theme={theme} locale={antdLocales[locale]}>
        <AntApp>
          <RouterProvider router={router} />
        </AntApp>
      </ConfigProvider>
    </I18nProvider>
  )
}

function BrowserTitle() {
  const { t } = useLingui()
  const appearance = useConsoleTheme()
  const title = [appearance?.pageTitle,appearance?.workspaceName,t`Yaoguang Marketing Platform`].filter(Boolean).join(' · ')

  useEffect(() => {
    document.title = title
  }, [title])

  return null
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <ConsoleThemeProvider>
        <LocaleProvider>
          <AppContent />
        </LocaleProvider>
        </ConsoleThemeProvider>
      </AuthProvider>
    </QueryClientProvider>
  )
}

export default App
