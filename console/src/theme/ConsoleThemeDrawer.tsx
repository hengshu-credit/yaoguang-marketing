import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Drawer, Form, InputNumber, Radio, Select, Space, Typography } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { ACCENT_PRESETS, consoleAccent, DEFAULT_CONSOLE_THEME, normalizeConsoleTheme, type ConsoleTheme } from './consoleTheme'
import { useConsoleTheme } from './ConsoleThemeProvider'
import { saveConsoleTheme } from './consoleThemeApi'
import type { Workspace } from '../services/api/workspace'

interface Props { workspace: Workspace; canManage: boolean; onClose: () => void; onSaved: () => Promise<void> }

export function ConsoleThemeDrawer({ workspace,canManage,onClose,onSaved }: Props) {
  const {t} = useLingui()
  const appearance = useConsoleTheme()
  const [draft,setDraft] = useState(() => normalizeConsoleTheme(workspace.settings.console_theme))
  const [saving,setSaving] = useState(false)
  const [error,setError] = useState('')
  const alive = useRef(true)
  const busy = useRef(false)
  const preview = appearance?.preview
  const confirm = appearance?.confirm
  useEffect(() => {
    alive.current = true
    return () => { alive.current = false; preview?.(workspace.id) }
  },[workspace.id,preview])
  useEffect(() => { preview?.(workspace.id,canManage ? draft : undefined) },[workspace.id,draft,canManage,preview])
  const patch = (value: Partial<ConsoleTheme>) => { if(!busy.current && canManage) {setDraft(current => normalizeConsoleTheme({...current,...value}));setError('')} }
  const save = async () => {
    if(busy.current || !canManage) return
    busy.current = true;setSaving(true);setError('')
    try {
      const result = await saveConsoleTheme(workspace.id,draft)
      if(!alive.current) return
      confirm?.(workspace.id,result.console_theme)
      onClose()
      // Reload authority separately: a refresh failure must not claim a saved write failed.
      void onSaved().catch(() => undefined)
    } catch(cause) {if(alive.current)setError(cause instanceof Error ? cause.message : t`Failed to save theme settings`)}
    finally {busy.current = false;if(alive.current)setSaving(false)}
  }
  const labels: Record<ConsoleTheme['accent_preset'],string> = {yaoguang:t`Yaoguang purple`,blue:t`Theme blue`,purple:t`Liquid purple`,teal:t`Teal`,rose:t`Rose`,indigo:t`Indigo`,nebula:t`Nebula gradient`,dusk:t`Dusk gradient`}
  const accent = consoleAccent(draft)
  const disabled = !canManage || saving
  const colorInput = (label:string,color:string,onChange:(color:string)=>void) => <label key={label} className="console-theme-color"><span>{label}</span><input type="color" aria-label={label} value={color} disabled={disabled} onChange={event=>onChange(event.target.value)} /><code>{color}</code></label>
  return <Drawer open size={440} title={t`Theme settings`} onClose={()=>{if(!busy.current)onClose()}} mask={false} keyboard={!saving} className="console-theme-drawer" footer={<div className="console-theme-footer"><Button disabled={saving} onClick={onClose}>{t`Cancel preview`}</Button>{canManage && <Button type="primary" loading={saving} onClick={()=>void save()}>{t`Save theme`}</Button>}</div>}>
    <Space orientation="vertical" size="large" className="w-full">
      <Alert type="info" showIcon title={t`Workspace appearance`} description={t`Changes preview immediately. Save shares the theme with this workspace; closing restores the saved appearance. Message templates and customer pages are unchanged.`} />
      {!canManage && <Alert type="info" title={t`Only workspace owners can change the shared theme.`} />}
      {error && <Alert type="error" showIcon title={error} />}
      <Form layout="vertical" disabled={disabled}>
        <Form.Item label={t`Color scheme`}><Radio.Group aria-label={t`Color scheme`} value={draft.color_scheme} onChange={event=>patch({color_scheme:event.target.value})} options={[{value:'light',label:t`Light`},{value:'dark',label:t`Dark`},{value:'system',label:t`Follow system`}]} optionType="button" /></Form.Item>
        <Form.Item label={t`Accent color source`}><Select aria-label={t`Accent color source`} value={draft.accent_mode} onChange={accent_mode=>patch({accent_mode})} options={[{value:'preset',label:t`Color presets`},{value:'solid',label:t`Custom solid color`},{value:'gradient',label:t`Custom gradient`}]} /></Form.Item>
        {draft.accent_mode==='preset' && <div className="console-theme-presets">{Object.keys(ACCENT_PRESETS).map(key=>{
          const id=key as ConsoleTheme['accent_preset'];const p=consoleAccent({...draft,accent_preset:id})
          return <button type="button" key={id} disabled={disabled} aria-label={labels[id]} aria-pressed={draft.accent_preset===id} className="console-theme-swatch" onClick={()=>patch({accent_preset:id})}><span aria-hidden style={{background:p.background}}/><span>{labels[id]}</span></button>
        })}</div>}
        {draft.accent_mode==='solid' && colorInput(t`Custom accent color`,draft.primary_color,primary_color=>patch({primary_color}))}
        {draft.accent_mode==='gradient' && <Space orientation="vertical" className="w-full">
          <Form.Item label={t`Gradient colors`}><Radio.Group value={draft.gradient_colors.length} onChange={event=>patch({gradient_colors:event.target.value===3?[...draft.gradient_colors,'#E14ECA']:draft.gradient_colors.slice(0,2)})} options={[{value:2,label:t`Two colors`},{value:3,label:t`Three colors`}]} /></Form.Item>
          {draft.gradient_colors.map((color,index)=>colorInput(t`Gradient color ${index+1}`,color,next=>patch({gradient_colors:draft.gradient_colors.map((old,i)=>i===index?next:old)})))}
          <Select aria-label={t`Gradient type`} value={draft.gradient_type} onChange={gradient_type=>patch({gradient_type})} options={[{value:'linear',label:t`Linear gradient`},{value:'radial',label:t`Radial gradient`}]} />
          {draft.gradient_type==='linear' && <InputNumber aria-label={t`Gradient angle`} value={draft.gradient_angle} min={0} max={360} precision={0} onChange={angle=>{if(angle!==null)patch({gradient_angle:angle})}} />}
        </Space>}
        <div className="console-theme-preview" style={{background:accent.background}}><span style={{color:accent.controlForeground,background:accent.controlBackground}}>{t`Live theme preview`}</span></div>
        <Form.Item label={t`Navigation appearance`}><Radio.Group value={draft.sidebar_style} onChange={event=>patch({sidebar_style:event.target.value})} options={[{value:'light',label:t`Light navigation`},{value:'dark',label:t`Dark navigation`}]} /></Form.Item>
        <Form.Item label={t`Content width`}><Radio.Group value={draft.content_width} onChange={event=>patch({content_width:event.target.value})} options={[{value:'fluid',label:t`Fluid width`},{value:'fixed',label:t`Centered width`}]} /></Form.Item>
        <Form.Item label={t`Interface density`}><Radio.Group value={draft.density} onChange={event=>patch({density:event.target.value})} options={[{value:'comfortable',label:t`Comfortable`},{value:'compact',label:t`Compact`}]} /></Form.Item>
        <Form.Item label={t`Page title style`}><Select aria-label={t`Page title style`} value={draft.title_style} onChange={title_style=>patch({title_style})} options={[{value:'accent',label:t`Accent title`},{value:'plain',label:t`Plain title`},{value:'compact',label:t`Compact title`}]} /></Form.Item>
        <div className="console-theme-title-example"><h2 className="workspace-page-title">{t`Marketing Campaigns`}</h2><Typography.Text type="secondary">{t`Title, description and actions follow the same hierarchy.`}</Typography.Text></div>
        <Typography.Paragraph type="secondary">{t`Console fonts remain managed in Workspace settings → General.`}</Typography.Paragraph>
        <Button block disabled={disabled} onClick={()=>patch(normalizeConsoleTheme(DEFAULT_CONSOLE_THEME))}>{t`Restore default theme`}</Button>
      </Form>
    </Space>
  </Drawer>
}
