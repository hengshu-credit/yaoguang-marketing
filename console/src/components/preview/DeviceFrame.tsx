import type { ReactNode } from 'react'
import { BatteryFull, ChevronLeft, Ellipsis, LockKeyhole, Signal, Wifi } from 'lucide-react'
import './preview.css'

export type PreviewDevice = 'ios' | 'android' | 'desktop' | 'h5'

export interface DeviceFrameProps {
  device: PreviewDevice
  children: ReactNode
  title?: string
  appearance?: 'app' | 'notification' | 'browser'
  compact?: boolean
  currentTime?: string
}

/** Shared device chrome for material editors, saved templates and delivery history. */
export default function DeviceFrame({
  device, children, title, appearance = 'app', compact = false, currentTime = '9:41'
}: DeviceFrameProps) {
  const desktop = device === 'desktop'
  const browser = device === 'h5' || appearance === 'browser'
  return (
    <div className={`material-preview-stage${compact ? ' material-preview-stage--compact' : ''}`}>
      <div
        className={`material-device material-device--${device} material-device--${appearance}${compact ? ' material-device--compact' : ''}`}
        data-preview-device={device}
      >
        {!desktop && <div className="material-device__buttons" aria-hidden="true" />}
        <div className="material-device__screen">
          {desktop ? (
            <div className="material-device__window-bar">
              <span className="material-device__window-dots" aria-hidden="true"><i /><i /><i /></span>
              <span className="material-device__window-title">{title}</span>
              <Ellipsis size={16} aria-hidden="true" />
            </div>
          ) : (
            <div className="material-device__status" aria-hidden="true">
              <span>{currentTime}</span>
              <span className="material-device__camera" />
              <span className="material-device__status-icons"><Signal size={14} /><Wifi size={14} /><BatteryFull size={18} /></span>
            </div>
          )}
          {browser && <div className="material-device__address" aria-hidden="true"><ChevronLeft size={16} /><span><LockKeyhole size={12} />{title || 'H5'}</span><Ellipsis size={16} /></div>}
          {!desktop && !browser && appearance === 'app' && title && (
            <div className="material-device__app-bar"><ChevronLeft size={20} aria-hidden="true" /><span>{title}</span><Ellipsis size={20} aria-hidden="true" /></div>
          )}
          <div className="material-device__viewport" tabIndex={0}>
            {appearance === 'notification' && !desktop && <div className="material-device__lock-clock" aria-hidden="true"><LockKeyhole size={18} /><span>{currentTime}</span></div>}
            {children}
          </div>
          {!desktop && <div className="material-device__home" aria-hidden="true"><span /></div>}
        </div>
      </div>
    </div>
  )
}
