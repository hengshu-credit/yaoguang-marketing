import { Select, Tag } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { templateCategoriesApi } from '../../services/api/templateCategories'
import { templateCategoryDisplayName } from './templateCategoryLabels'

interface TemplateCategorySelectProps {
  workspaceId: string
  value?: string
  onChange?: (value: string) => void
  disabled?: boolean
}

const TemplateCategorySelect: React.FC<TemplateCategorySelectProps> = ({ workspaceId, value, onChange, disabled }) => {
  const { t } = useLingui()
  const { data, isLoading } = useQuery({
    queryKey: ['template-categories', workspaceId, true],
    queryFn: () => templateCategoriesApi.list(workspaceId, true),
    enabled: Boolean(workspaceId)
  })
  const categories = (data?.categories || []).filter((category) => category.is_active || category.id === value)
  const systemNames: Record<string, string> = {
    marketing: t`Marketing`, transactional: t`Transactional`, welcome: t`Welcome`,
    opt_in: t`Opt-in`, unsubscribe: t`Unsubscribe`, bounce: t`Bounce`,
    blocklist: t`Blocklist`, blog: t`Blog`, other: t`Other`
  }
  return <Select
    value={value}
    onChange={onChange}
    disabled={disabled}
    loading={isLoading}
    placeholder={t`Select category`}
    options={categories.map((category) => ({
      value: category.id,
      label: <span>{templateCategoryDisplayName(category, systemNames)}{!category.is_active && <Tag className="ml-2">{t`Inactive`}</Tag>}</span>
    }))}
  />
}

export default TemplateCategorySelect
