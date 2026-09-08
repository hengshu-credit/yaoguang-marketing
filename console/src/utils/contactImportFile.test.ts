import { describe, expect, it } from 'vitest'
import { read, utils, write } from 'xlsx'
import Papa from 'papaparse'
import { createContactImportTemplate, customerImportCSV, readContactImportFile } from './contactImportFile'

const CUSTOMER_IMPORT_HEADERS = ['external_user_id', 'email', 'phone'] // Legacy files remain readable.

function bufferOf(blob: Blob): Promise<ArrayBuffer> {
  return new Promise((resolve) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as ArrayBuffer)
    reader.readAsArrayBuffer(blob)
  })
}

function workbookFile(rows: string[][], format: 'xlsx' | 'xls') {
  const workbook = utils.book_new()
  utils.book_append_sheet(workbook, utils.aoa_to_sheet(rows), 'Customers')
  utils.book_append_sheet(workbook, utils.aoa_to_sheet([['email'], ['ignored@example.com']]), 'Other')
  return new File([write(workbook, { type: 'array', bookType: format === 'xls' ? 'biff8' : 'xlsx' })], `customers.${format.toUpperCase()}`)
}

describe('contact import files', () => {
  const rows = [CUSTOMER_IMPORT_HEADERS, ['000123456789012345678', '', '+8613800000000'], ['', 'customer@example.com', '']]

  it.each(['xlsx', 'xls'] as const)('reads the first %s worksheet without losing identifiers or empty fields', async (format) => {
    const file = workbookFile(rows, format)
    expect(await readContactImportFile(file)).toEqual(rows)
    const csv = new TextDecoder().decode(await bufferOf(await customerImportCSV(file)))
    expect(Papa.parse(csv).data).toEqual(rows)
  })

  it('reads BOM, semicolon CSV, quoted text and skips empty lines', async () => {
    const file = new File(['\uFEFFemail;first_name;phone\r\ncustomer@example.com;"小明,\n测试";00123\r\n;;\r\n'], 'customers.CSV')
    expect(await readContactImportFile(file)).toEqual([
      ['email', 'first_name', 'phone'], ['customer@example.com', '小明,\n测试', '00123']
    ])
  })

  it.each(['csv', 'xlsx'] as const)('can fill and reimport the downloaded %s template', async (format) => {
    const blob = await createContactImportTemplate(CUSTOMER_IMPORT_HEADERS, format)
    const buffer = await bufferOf(blob)
    let file: File
    if (format === 'csv') {
      const text = new TextDecoder().decode(buffer)
      expect(Array.from(new Uint8Array(buffer).slice(0, 3))).toEqual([239, 187, 191])
      expect(text.trim()).toBe(CUSTOMER_IMPORT_HEADERS.join(','))
      file = new File([blob, '000123,,\r\n'], 'template.csv')
    } else {
      const workbook = read(buffer, { type: 'array', cellNF: true })
      const sheet = workbook.Sheets[workbook.SheetNames[0]]
      expect(sheet.A2.z).toBe('@')
      utils.sheet_add_aoa(sheet, [['000123', '', '']], { origin: 'A2' })
      file = new File([write(workbook, { type: 'array', bookType: 'xlsx' })], 'template.xlsx')
    }
    expect(await readContactImportFile(file)).toEqual([CUSTOMER_IMPORT_HEADERS, ['000123', '', '']])
  })

  it.each([
    ['customers.txt', 'email\na@example.com', 'Please upload a CSV, XLSX or XLS file.'],
    ['customers.xlsx', 'email\na@example.com', 'Unable to read the Excel file.'],
    ['customers.csv', 'email\n', 'at least one customer'],
    ['customers.csv', 'email,EMAIL\na@example.com,b@example.com', 'unique'],
    ['customers.csv', 'email,\na@example.com,b', 'non-empty'],
    ['customers.csv', 'email\n"unclosed', 'malformed']
  ])('rejects invalid input %s: %s', async (filename, contents, error) => {
    await expect(readContactImportFile(new File([contents], filename))).rejects.toThrow(error)
  })

  it('requires a customer identity column before submitting a background import', async () => {
    await expect(customerImportCSV(workbookFile([['first_name'], ['小明']], 'xlsx'))).rejects.toThrow('at least one customer identifier')
  })

  it('preserves CSV bytes so the server can retain malformed rows as failures', async () => {
    const file = new File(['\uFEFFexternal_user_id,email\na,a@example.com\nbad-row\nc,c@example.com'], 'customers.csv')
    expect(await customerImportCSV(file)).toBe(file)
  })
})
