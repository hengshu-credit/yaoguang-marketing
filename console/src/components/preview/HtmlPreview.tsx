import DeviceFrame, { type PreviewDevice } from './DeviceFrame'
import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'

export default function HtmlPreview({ html, title, device, browser = false, profile, header, frameTitle }: {
  html: string
  title: string
  device: PreviewDevice
  browser?: boolean
  profile?: string
  header?: ReactNode
  frameTitle?: string
}) {
  const viewport = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState({ width: 0, height: 0 })
  // A desktop template must hit desktop media queries even inside a narrow drawer.
  const layoutWidth = device === 'desktop' ? 1024 : device === 'android' ? 393 : 375
  const scale = size.width > 0 ? size.width / layoutWidth : 1
  useLayoutEffect(() => {
    const element = viewport.current
    if (!element) return
    const measure = () => {
      const bounds = element.getBoundingClientRect()
      if (bounds.width > 0 && bounds.height > 0) setSize({ width: bounds.width, height: bounds.height })
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  return <DeviceFrame device={device} title={frameTitle || title} appearance={browser ? 'browser' : 'app'}>
    <div className="material-preview-html-layout">
      {header}
      <div ref={viewport} className="material-preview-html-viewport">
      <iframe
        className="material-preview-html" srcDoc={html} title={title} sandbox="" data-client-profile={profile}
        data-preview-viewport-width={layoutWidth}
        style={{ width: layoutWidth, height: size.height > 0 ? size.height / scale : 600, minHeight: 0, transform: `scale(${scale})`, transformOrigin: 'top left' }}
      />
      </div>
    </div>
  </DeviceFrame>
}
