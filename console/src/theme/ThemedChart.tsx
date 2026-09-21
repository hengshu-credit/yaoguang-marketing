import ReactECharts from 'echarts-for-react'
import ReactEChartsCore from 'echarts-for-react/lib/core'
import type { EChartsReactProps } from 'echarts-for-react/lib/types'
import { useConsoleTheme } from './ConsoleThemeProvider'
import { chartThemeOptions } from './chartTheme'

function useChartProps(props:EChartsReactProps) {
  const appearance=useConsoleTheme()
  if(!appearance || props.theme)return props
  return {...props,theme:appearance.dark?'dark':undefined,option:chartThemeOptions(props.option,appearance.value,appearance.dark)}
}
export function ThemedChart(props:EChartsReactProps){return <ReactECharts {...useChartProps(props)} />}
export function ThemedChartCore(props:EChartsReactProps){return <ReactEChartsCore {...useChartProps(props)} />}
