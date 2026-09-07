import { describe, it, expect } from 'vitest'
import { buildFunnelGraph } from './funnel-graph'
describe('funnel graph', () => { it('expands OR and parallel branches with an explicit merge per step', () => { const graph = buildFunnelGraph({ subject_type: 'contract', window_seconds: 3600, steps: [{ id: 'a', events: ['start'] }, { id: 'b', events: ['signed', 'approved'] }, { id: 'c', all_events: ['front', 'back'] }] }); expect(graph.nodes).toHaveLength(7); expect(graph.edges.some(e => e.target === 'b')).toBe(true); expect(graph.nodes.find(n => n.id === 'c')?.data.mode).toBe('all') }) })
