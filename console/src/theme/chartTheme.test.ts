import { describe,expect,it } from 'vitest'
import { chartThemeOptions } from './chartTheme'
import { normalizeConsoleTheme } from './consoleTheme'

describe('chart theme preserves data and adapts readable chrome',()=>{
  it('keeps light options intact and changes only dark-mode labels',()=>{
    const series=[{type:'line',data:[1,2],lineStyle:{color:'#ff0000'}}]
    const option={series,xAxis:{axisLabel:{color:'#6b7280',formatter:'{value}%'}},legend:{show:true}}
    const value=normalizeConsoleTheme()
    expect(chartThemeOptions(option,value,false)).toBe(option)
    const dark=chartThemeOptions(option,value,true)
    expect(dark.series).toBe(series);expect(dark.backgroundColor).toBe('transparent')
    expect(dark.xAxis).toMatchObject({axisLabel:{formatter:'{value}%',color:'#B6C2D5'}})
    expect(option.xAxis.axisLabel.color).toBe('#6b7280')
  })
})
