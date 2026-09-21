import { StrictMode, useLayoutEffect, useState } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { App } from 'antd'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ConsoleThemeProvider, useConsoleTheme } from './ConsoleThemeProvider'
import { ConsoleThemeDrawer } from './ConsoleThemeDrawer'
import { normalizeConsoleTheme } from './consoleTheme'
import { saveConsoleTheme } from './consoleThemeApi'
import { WorkspacePageTitle } from '../components/navigation/WorkspacePageTitle'
import type { Workspace } from '../services/api/workspace'

vi.mock('./consoleThemeApi',()=>({saveConsoleTheme:vi.fn()}))
const workspace={id:'w1',name:'Workspace',settings:{console_theme:normalizeConsoleTheme()}} as Workspace
function Scope({id='w1',canManage=true}:{id?:string;canManage?:boolean}){
  const appearance=useConsoleTheme()!
  const [open,setOpen]=useState(true)
  const bind=appearance.bind
  useLayoutEffect(()=>bind(id,'Workspace',workspace.settings.console_theme),[id,bind])
  return <><WorkspacePageTitle>Customer events</WorkspacePageTitle><output data-testid="page-title">{appearance.pageTitle}</output><output data-testid="theme">{appearance.value.color_scheme}</output>{open && <ConsoleThemeDrawer key={id} workspace={{...workspace,id}} canManage={canManage} onClose={()=>setOpen(false)} onSaved={async()=>undefined}/>}</>
}
function view(id='w1',canManage=true){return render(<ConsoleThemeProvider><App><Scope id={id} canManage={canManage}/></App></ConsoleThemeProvider>)}

describe('workspace console theme preview',()=>{
  beforeEach(()=>vi.clearAllMocks())
  it('previews immediately and restores the saved setting on close',async()=>{
    view();fireEvent.click(screen.getByLabelText('Dark'))
    await waitFor(()=>expect(document.documentElement.dataset.consoleTheme).toBe('dark'))
    fireEvent.click(screen.getByRole('button',{name:'Cancel preview'}))
    await waitFor(()=>expect(document.documentElement.dataset.consoleTheme).toBe('light'))
    expect(saveConsoleTheme).not.toHaveBeenCalled();expect(screen.getByTestId('page-title')).toHaveTextContent('Customer events')
  })
  it('keeps a failed save editable and sends only the selected workspace theme',async()=>{
    vi.mocked(saveConsoleTheme).mockRejectedValueOnce(new Error('Temporary failure')).mockResolvedValueOnce({console_theme:normalizeConsoleTheme({color_scheme:'dark'})})
    view();fireEvent.click(screen.getByLabelText('Dark'));fireEvent.click(screen.getByRole('button',{name:'Save theme'}))
    expect(await screen.findByText('Temporary failure')).toBeVisible();expect(screen.getByTestId('theme')).toHaveTextContent('dark')
    fireEvent.click(screen.getByRole('button',{name:'Save theme'}))
    await waitFor(()=>expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(saveConsoleTheme).toHaveBeenLastCalledWith('w1',expect.objectContaining({color_scheme:'dark'}));expect(screen.getByTestId('theme')).toHaveTextContent('dark')
  })
  it('ignores a save completion from the old workspace',async()=>{
    let resolve!: (v:{console_theme:ReturnType<typeof normalizeConsoleTheme>})=>void
    vi.mocked(saveConsoleTheme).mockImplementation(()=>new Promise(done=>{resolve=done}))
    const result=view();fireEvent.click(screen.getByLabelText('Dark'));fireEvent.click(screen.getByRole('button',{name:'Save theme'}))
    result.rerender(<ConsoleThemeProvider><App><Scope id="w2" /></App></ConsoleThemeProvider>)
    await act(async()=>resolve({console_theme:normalizeConsoleTheme({color_scheme:'dark'})}))
    expect(screen.getByTestId('theme')).toHaveTextContent('light');expect(screen.getByRole('dialog')).toBeVisible()
  })
  it('does not offer shared writes to non-owners',()=>{
    view('w1',false);expect(screen.queryByRole('button',{name:'Save theme'})).not.toBeInTheDocument();expect(screen.getByLabelText('Dark')).toBeDisabled()
  })
  it('retains the page title after StrictMode effect replay',async()=>{
    render(<StrictMode><ConsoleThemeProvider><App><Scope /></App></ConsoleThemeProvider></StrictMode>)
    await waitFor(()=>expect(screen.getByTestId('page-title')).toHaveTextContent('Customer events'))
  })
  it('follows system appearance and removes the listener on unmount',async()=>{
    const fallback=window.matchMedia('(min-width: 1px)')
    let listener: (()=>void)|undefined
    const remove=vi.fn()
    const media={matches:false,addEventListener:vi.fn((_name:string,callback:()=>void)=>{listener=callback}),removeEventListener:remove}
    const match=vi.spyOn(window,'matchMedia').mockImplementation(query=>query==='(prefers-color-scheme: dark)'?media as unknown as MediaQueryList:{...fallback,media:query})
    const result=view();fireEvent.click(screen.getByLabelText('Follow system'))
    act(()=>{media.matches=true;listener?.()})
    await waitFor(()=>expect(document.documentElement.dataset.consoleTheme).toBe('dark'))
    result.unmount();expect(remove).toHaveBeenCalled();expect(document.documentElement.dataset.consoleTheme).toBeUndefined()
    match.mockRestore()
  })
})
