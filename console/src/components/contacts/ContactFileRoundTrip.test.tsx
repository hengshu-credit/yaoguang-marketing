import '../../__tests__/resizeObserverMock'
import { App, message } from 'antd'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Papa from 'papaparse'
import { read, utils } from 'xlsx'
import { ImportTemplateButtons } from './ImportTemplateButtons'
import { ExportContactsModal } from './ExportContactsModal'
import { ContactsCsvUploadDrawer } from './ContactsCsvUploadDrawer'
import { exportContactField, importContactField, matchContactFileField, type ContactFileField } from '../../utils/contactFileSchema'
import type { Contact } from '../../services/api/contacts'

const { loadFields, listContacts, batchImport } = vi.hoisted(() => ({ loadFields: vi.fn(), listContacts: vi.fn(), batchImport: vi.fn() }))
vi.mock('../../services/api/client', () => ({ api: { get: vi.fn() } }))
vi.mock('../../utils/contactFileSchema', async () => ({ ...await vi.importActual('../../utils/contactFileSchema'), contactFileSchema: { load: loadFields } }))
vi.mock('../../services/api/contacts', () => ({ contactsApi: { list: listContacts, batchImport } }))

const field = (key: string, type: ContactFileField['type'] = 'string', header = key, read_only = false): ContactFileField => ({ key, type, header, read_only })
const fields = [field('email'), field('full_name'), field('custom_string_1', 'string', '会员编号 (custom_string_1)'), field('custom_number_1', 'number', '积分 (custom_number_1)'), field('profile.attributes.VIP', 'boolean', '会员 (profile.attributes.VIP)'), field('profile.attributes.detail', 'json', '资料 (profile.attributes.detail)'), field('created_at', 'string', 'created_at', true)]
const contact: Contact = { email: 'test@example.com', full_name: '小明\n客户', custom_string_1: '000123456789012345678', custom_number_1: 0, profile: { attributes: { VIP: false, detail: { name: '中文', active: false } } }, created_at: '2020-01-01T00:00:00Z', updated_at: '', contact_lists: [] }
let downloads: Blob[]

function bufferOf(blob: Blob): Promise<ArrayBuffer> {
  return new Promise(resolve => { const reader = new FileReader(); reader.onload = () => resolve(reader.result as ArrayBuffer); reader.readAsArrayBuffer(blob) })
}

function wrapper({ children }: { children: React.ReactNode }) { return <App>{children}</App> }

describe('shared contact export and import schema', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    downloads = []
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn((blob: Blob) => { downloads.push(blob); return 'blob:test' }) })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    loadFields.mockImplementation(async () => [...fields])
    listContacts.mockResolvedValue({ contacts: [contact] })
    batchImport.mockResolvedValue({ operations: [{ action: 'create', email: contact.email }] })
    localStorage.clear()
  })

  it('downloads template and export with identical columns and includes newly created fields on the next download', async () => {
    render(<><ImportTemplateButtons workspaceId="ws" /><ExportContactsModal visible onCancel={vi.fn()} workspaceId="ws" workspace={undefined} filters={{}} segmentsData={[]} listsData={[]} /></>, { wrapper })
    fireEvent.click(screen.getByRole('button', { name: 'Download CSV template' }))
    await waitFor(() => expect(downloads).toHaveLength(1))
    fireEvent.click(screen.getByRole('button', { name: 'Export' }))
    await waitFor(() => expect(listContacts).toHaveBeenCalled())
    await waitFor(() => expect(downloads).toHaveLength(2))
    const templateRows = Papa.parse<string[]>(new TextDecoder().decode(await bufferOf(downloads[0]))).data
    const exportRows = Papa.parse<string[]>(new TextDecoder().decode(await bufferOf(downloads[1]))).data
    expect(exportRows[0]).toEqual(templateRows[0])
    expect(exportRows[1]).toEqual(['test@example.com', '小明\n客户', '000123456789012345678', '0', 'false', '{"name":"中文","active":false}', '2020-01-01T00:00:00Z'])
    expect(listContacts).toHaveBeenCalledWith(expect.objectContaining({ with_profiles: true }))
    loadFields.mockResolvedValue([...fields, field('profile.attributes.newField', 'integer', '新增字段 (profile.attributes.newField)')])
    fireEvent.click(screen.getByRole('button', { name: 'Download Excel template' }))
    await waitFor(() => expect(downloads).toHaveLength(3))
    const workbook = read(await bufferOf(downloads[2]), { type: 'array' })
    const headers = utils.sheet_to_json<string[]>(workbook.Sheets[workbook.SheetNames[0]], { header: 1 })[0]
    expect(headers).toEqual([...templateRows[0], '新增字段 (profile.attributes.newField)'])
    expect(loadFields).toHaveBeenCalledTimes(3)
  })

  it('imports exported columns into typed fields and ignores system timestamps', async () => {
    render(<ContactsCsvUploadDrawer workspaceId="ws" isVisible onClose={vi.fn()} />, { wrapper })
    const values = [contact.email, contact.full_name, contact.custom_string_1, '0', 'false', JSON.stringify(contact.profile?.attributes?.detail), contact.created_at]
    const csv = Papa.unparse([fields.map(field => field.header), values])
    fireEvent.change(document.querySelector('input[type="file"]')!, { target: { files: [new File([csv], 'export.csv')] } })
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Start Upload' }) || vi.mocked(message.error).mock.calls.length).toBeTruthy())
    expect(message.error).not.toHaveBeenCalled()
    fireEvent.click(await screen.findByRole('button', { name: 'Start Upload' }))
    await waitFor(() => expect(batchImport).toHaveBeenCalled())
    const uploaded = batchImport.mock.calls[0][0].contacts[0]
    expect(uploaded).toEqual({ email: contact.email, full_name: contact.full_name, custom_string_1: contact.custom_string_1, custom_number_1: 0, profile: contact.profile })
    expect(uploaded).not.toHaveProperty('created_at')
  })

  it('keeps base exports available without requesting restricted customer profile data', async () => {
    loadFields.mockResolvedValue(fields.filter(field => !field.key.startsWith('profile.')))
    render(<ExportContactsModal visible onCancel={vi.fn()} workspaceId="ws" workspace={undefined} filters={{}} segmentsData={[]} listsData={[]} />, { wrapper })
    fireEvent.click(screen.getByRole('button', { name: 'Export' }))
    await waitFor(() => expect(downloads).toHaveLength(1))
    expect(listContacts).toHaveBeenCalledWith(expect.objectContaining({ with_profiles: false }))
  })

  it('does not silently replace malformed typed data with null', () => {
    const output: Partial<Contact> = {}
    expect(() => importContactField(output, fields[3], 'not-a-number')).toThrow()
    expect(() => importContactField(output, fields[5], 'not-json')).toThrow()
    expect(output).toEqual({})
  })

  it('matches renamed labels by stable keys and preserves case-sensitive attribute names', () => {
    expect(matchContactFileField(fields, '旧名称 (custom_string_1)')?.key).toBe('custom_string_1')
    const variants = [field('profile.attributes.Score', 'number', '分数 (profile.attributes.Score)'), field('profile.attributes.score', 'number', '分数 (profile.attributes.score)')]
    expect(matchContactFileField(variants, 'profile.attributes.Score')?.key).toBe('profile.attributes.Score')
    expect(matchContactFileField(variants, '分数')).toBeUndefined()
    expect(matchContactFileField(fields, 'emai')).toBeUndefined()
    expect(exportContactField(contact, field('profile.attributes.constructor'))).toBe('')
  })
})
