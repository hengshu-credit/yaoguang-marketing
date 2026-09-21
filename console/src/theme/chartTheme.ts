import type { ConsoleTheme } from './consoleTheme'
import { consolePalette } from './consoleTheme'

const record = (value: unknown): Record<string,unknown> => value && typeof value==='object' && !Array.isArray(value) ? value as Record<string,unknown> : {}
export function chartThemeOptions(option: Record<string,unknown>,value:ConsoleTheme,dark:boolean) {
  if(!dark)return option
  const p=consolePalette(value,true)
  const axis=(input:unknown):unknown => Array.isArray(input) ? input.map(axis) : {...record(input),axisLabel:{...record(record(input).axisLabel),color:p.secondary},axisLine:{...record(record(input).axisLine),lineStyle:{...record(record(record(input).axisLine).lineStyle),color:p.border}},splitLine:{...record(record(input).splitLine),lineStyle:{...record(record(record(input).splitLine).lineStyle),color:p.border}}}
  return {...option,backgroundColor:'transparent',textStyle:{...record(option.textStyle),color:p.text},
    ...(option.legend ? {legend:Array.isArray(option.legend)?option.legend.map(item=>({...item,textStyle:{...item.textStyle,color:p.secondary}})):{...record(option.legend),textStyle:{...record(record(option.legend).textStyle),color:p.secondary}}} : {}),
    ...(option.xAxis?{xAxis:axis(option.xAxis)}:{}),...(option.yAxis?{yAxis:axis(option.yAxis)}:{})}
}
