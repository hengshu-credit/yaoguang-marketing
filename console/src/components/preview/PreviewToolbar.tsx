import type { ReactNode } from 'react'
import { useLingui } from '@lingui/react/macro'
import { MonitorSmartphone } from 'lucide-react'
import './preview.css'

export default function PreviewToolbar({ children }: { children: ReactNode }) {
  const { t } = useLingui()
  return <div className="material-preview-toolbar">
    <div className="material-preview-toolbar__controls">{children}</div>
    <span className="material-preview-toolbar__simulation"><MonitorSmartphone size={14} aria-hidden="true" />{t`Simulated preview`}</span>
  </div>
}
