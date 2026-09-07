import type { StateRule } from './funnels'

export interface BusinessConditionNodeConfig {
  mode: 'wait' | 'exit_if' | 'continue_if'
  subject_type: string
  events: string[]
  condition?: StateRule
  timeout_seconds: number
  on_timeout: 'exit' | 'continue'
}

export interface MarketingTrackingPlan {
  subject_type: string
  events: string[]
  states: string[]
  goal_events?: string[]
  window_seconds: number
}

export interface MarketingEventRule {
  subject_type: string
  events: string[]
  condition?: StateRule
  once_per_object?: boolean
  max_causation_depth?: number
  excluded_sources?: string[]
}
