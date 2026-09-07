export function record(value: unknown): Record<string, unknown> {
 return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}
export function materialTemplates(materials: unknown): Record<string, unknown>[] {
 const list = Array.isArray(materials) ? materials : record(materials).templates
 return Array.isArray(list) ? list.map(record) : []
}
export function executionSections(configuration: Record<string, unknown>) {
 const settings = record(configuration.settings)
 return { filters: configuration.filters ?? settings.audience ?? configuration, tracking: record(settings.metadata).tracking_plan ?? settings.tracking_plan ?? record(configuration.trigger_config).tracking_plan ?? configuration.tracking_plan }
}
export function safeContentPreview(html: string) {
 return `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; media-src data:; style-src 'unsafe-inline' data:; font-src data:">${html}`
}
