import './resizeObserverMock'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nProvider } from '@lingui/react'
import { setupI18n } from '@lingui/core'
import { CustomerBusinessActivity } from '../components/customers/CustomerBusinessActivity'
vi.mock('../services/api/businessActivity',()=>({businessActivityApi:{events:vi.fn().mockResolvedValue({events:[1,2].map(i=>({event_id:`event-${i}`,report_id:`report-${i}`,reported_at:'2026-09-07T00:00:00Z',processing_status:'accepted',event:{event_name:'Repeated login',event_code:'login',triggered_at:'2026-09-07T00:00:00Z',data:{},customer_attributes:{}}}))}),objects:vi.fn().mockResolvedValue({objects:[]})}}))
describe('CustomerBusinessActivity',()=>{it('retains two occurrences with the same event name',async()=>{const i18n=setupI18n({locale:'en',messages:{en:{}}});const client=new QueryClient({defaultOptions:{queries:{retry:false}}});render(<I18nProvider i18n={i18n}><QueryClientProvider client={client}><CustomerBusinessActivity workspaceId="w" customerId="customer"/></QueryClientProvider></I18nProvider>);expect(await screen.findAllByText('Repeated login')).toHaveLength(2)})})
