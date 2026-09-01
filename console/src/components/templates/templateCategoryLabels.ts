import type { TemplateCategoryDefinition } from '../../services/api/templateCategories'

export function templateCategoryDisplayName(category: TemplateCategoryDefinition, systemNames: Record<string, string>): string {
  if (!category.is_system) return category.name
  return systemNames[category.id] || category.name
}
