import type { StateRule } from '../../services/api/funnels'

export function normalizeStateRule(rule: StateRule): StateRule {
 return { ...rule, ...(rule.kind === 'attribute' ? { operator: rule.operator || 'eq' } : {}), ...(rule.kind === 'state' ? { value: rule.value || 'true' } : {}), ...(rule.children ? { children: rule.children.map(normalizeStateRule) } : {}) }
}
