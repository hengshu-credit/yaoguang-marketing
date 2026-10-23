import { useLingui } from '@lingui/react/macro'
import { Monitor, Tablet, Smartphone } from 'lucide-react'
import type { PreviewDevice } from './DeviceFrame'

export default function PreviewDeviceButtons({ value, onChange, label }: {
  value: PreviewDevice
  onChange: (device: PreviewDevice) => void
  label?: string
}) {
  const { t } = useLingui()
  const devices = [
    { value: 'desktop' as const, label: t`Computer`, Icon: Monitor },
    { value: 'tablet' as const, label: 'Pad', Icon: Tablet },
    { value: 'android' as const, label: t`Android`, Icon: Smartphone },
    { value: 'ios' as const, label: t`iOS`, Icon: Smartphone },
  ]
  return <div className="material-preview-platforms" role="group" aria-label={label || t`Client preview`}>
    {devices.map(({ value: device, label: name, Icon }) => <button key={device} type="button" aria-pressed={(value === 'h5' ? 'ios' : value) === device} onClick={() => onChange(device)}><Icon size={14} aria-hidden="true" />{name}</button>)}
  </div>
}
