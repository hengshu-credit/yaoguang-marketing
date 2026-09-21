import { createContext, useCallback, useContext, useLayoutEffect, useMemo, useRef, useState, useEffect } from 'react'
import type { ReactNode } from 'react'
import { applyConsoleTheme, DEFAULT_CONSOLE_THEME, normalizeConsoleTheme, type ConsoleTheme } from './consoleTheme'

interface ThemeScope { id: string; name: string; theme: ConsoleTheme; owner: symbol }
interface ThemeContextValue {
  value: ConsoleTheme; dark: boolean; workspaceName: string; pageTitle: string
  bind: (id: string, name: string, value?: unknown) => () => void
  preview: (id: string, value?: ConsoleTheme) => void
  confirm: (id: string, value: ConsoleTheme) => void
  title: (value: string) => () => void
}
const ConsoleThemeContext = createContext<ThemeContextValue | null>(null)
// eslint-disable-next-line react-refresh/only-export-components -- Hook belongs to this context.
export const useConsoleTheme = () => useContext(ConsoleThemeContext)

export function ConsoleThemeProvider({ children }: { children: ReactNode }) {
  const [scope, setScope] = useState<ThemeScope>()
  const [draft, setDraft] = useState<{ id: string; theme: ConsoleTheme }>()
  const [page, setPage] = useState<{ text: string; owner: symbol }>()
  const [systemDark, setSystemDark] = useState(() => window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false)
  const activeOwner = useRef<symbol>()
  useEffect(() => {
    const media = window.matchMedia?.('(prefers-color-scheme: dark)')
    if (!media) return
    const update = () => setSystemDark(media.matches)
    update(); media.addEventListener('change',update)
    return () => media.removeEventListener('change',update)
  },[])
  const bind = useCallback((id: string,name: string,value?: unknown) => {
    const owner = Symbol(id)
    activeOwner.current = owner
    setScope({id,name,theme:normalizeConsoleTheme(value),owner}); setDraft(undefined)
    return () => { if(activeOwner.current === owner) { activeOwner.current = undefined; setScope(undefined); setDraft(undefined) } }
  },[])
  const preview = useCallback((id: string,value?: ConsoleTheme) => setDraft(current => value ? {id,theme:normalizeConsoleTheme(value)} : current?.id === id ? undefined : current),[])
  const confirm = useCallback((id: string,value: ConsoleTheme) => {
    setScope(current => current?.id === id ? {...current,theme:normalizeConsoleTheme(value)} : current)
    setDraft(undefined)
  },[])
  const title = useCallback((text: string) => {
    const owner = Symbol(text)
    setPage({text,owner})
    return () => setPage(current => current?.owner === owner ? undefined : current)
  },[])
  const value = scope ? (draft?.id === scope.id ? draft.theme : scope.theme) : DEFAULT_CONSOLE_THEME as ConsoleTheme
  const dark = !!scope && (value.color_scheme === 'dark' || value.color_scheme === 'system' && systemDark)
  const scopeId = scope?.id
  useLayoutEffect(() => {
    if (!scopeId) return
    return applyConsoleTheme(document.documentElement,value,dark)
  },[value,dark,scopeId])
  const context = useMemo(() => ({value,dark,workspaceName:scope?.name ?? '',pageTitle:page?.text ?? '',bind,preview,confirm,title}),[value,dark,scope?.name,page?.text,bind,preview,confirm,title])
  return <ConsoleThemeContext.Provider value={context}>{children}</ConsoleThemeContext.Provider>
}
