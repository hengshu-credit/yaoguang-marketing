import type { FunnelDefinition } from '../../services/api/funnels'
export function buildFunnelGraph(definition: FunnelDefinition) {
 const nodes: Array<{ id: string; position: { x: number; y: number }; data: { label: string; mode?: 'any' | 'all'; optional?: boolean } }> = []
 const edges: Array<{ id: string; source: string; target: string }> = []
 definition.steps.forEach((step, index) => {
  const codes = step.all_events ?? step.events ?? []
  const previous = definition.steps[index - 1]?.id
  nodes.push({ id: step.id, position: { x: index * 330, y: 150 }, data: { label: step.name || codes.join(' / '), mode: step.all_events ? 'all' : 'any', optional: step.optional } })
  if (codes.length > 1) codes.forEach((code, branch) => { const id = `${step.id}:branch:${branch}`; nodes.push({ id, position: { x: index * 330 - 150, y: branch * 100 }, data: { label: code } }); if (previous) edges.push({ id: `${previous}:${id}`, source: previous, target: id }); edges.push({ id: `${id}:${step.id}`, source: id, target: step.id }) })
  else if (previous) edges.push({ id: `${previous}:${step.id}`, source: previous, target: step.id })
  if (step.optional && previous && definition.steps[index + 1]) edges.push({ id: `${step.id}:skip`, source: previous, target: definition.steps[index + 1].id })
 })
 return { nodes, edges }
}
