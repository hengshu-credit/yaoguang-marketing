import Papa from 'papaparse'
import { i18n } from '@lingui/core'
import { msg } from '@lingui/core/macro'

export const CONTACT_IMPORT_ACCEPT = '.csv,.xlsx,.xls,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,application/vnd.ms-excel'
const CUSTOMER_IDENTITY_HEADERS = ['external_id', 'external_user_id', 'email', 'phone']

export function isContactImportFile(file: File): boolean {
  return /\.(csv|xlsx|xls)$/i.test(file.name)
}

function readFileBuffer(file: File): Promise<ArrayBuffer> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as ArrayBuffer)
    reader.onerror = () => reject(reader.error ?? new Error(i18n._(msg`Unable to read the import file.`)))
    reader.readAsArrayBuffer(file)
  })
}

export async function readContactImportFile(file: File): Promise<string[][]> {
  if (!isContactImportFile(file)) throw new Error(i18n._(msg`Please upload a CSV, XLSX or XLS file.`))
  let data: string[][]
  if (/\.csv$/i.test(file.name)) {
    data = await new Promise((resolve, reject) => {
      Papa.parse<string[]>(file, {
        skipEmptyLines: 'greedy',
        complete: (result) => {
          const error = result.errors.find((item) => item.code !== 'UndetectableDelimiter')
          if (error) reject(new Error(i18n._(msg`The CSV file is malformed. Check quotes and delimiters.`)))
          else resolve(result.data)
        },
        error: reject
      })
    })
  } else {
    try {
      const buffer = await readFileBuffer(file)
      const bytes = new Uint8Array(buffer)
      // SheetJS also reads plain text; reject text files renamed to Excel.
      if (!(bytes[0] === 0x50 && bytes[1] === 0x4b) && !(bytes[0] === 0xd0 && bytes[1] === 0xcf)) throw new Error('Invalid workbook')
      const { read, utils } = await import('xlsx')
      const workbook = read(buffer, { type: 'array' })
      const sheet = workbook.Sheets[workbook.SheetNames[0]]
      data = sheet ? utils.sheet_to_json<string[]>(sheet, { header: 1, raw: false, defval: '', blankrows: false }) : []
    } catch {
      throw new Error(i18n._(msg`Unable to read the Excel file. Use an unencrypted XLSX or XLS workbook.`))
    }
  }
  data = data.filter((row) => row.some((cell) => cell.trim() !== ''))
  if (data.length < 2) throw new Error(i18n._(msg`The import file must contain a header row and at least one customer.`))
  const headers = data[0].map((cell) => cell.replace(/^\uFEFF/, '').trim())
  if (headers.some((header) => !header) || new Set(headers.map((header) => header.toLowerCase())).size !== headers.length) {
    throw new Error(i18n._(msg`Column headers must be non-empty and unique.`))
  }
  return [headers, ...data.slice(1)]
}

export async function customerImportCSV(file: File): Promise<Blob> {
  if (!isContactImportFile(file)) throw new Error(i18n._(msg`Please upload a CSV, XLSX or XLS file.`))
  // Keep CSV streaming and malformed-row accounting on the server.
  if (/\.csv$/i.test(file.name)) return file
  const [headers, ...rows] = await readContactImportFile(file)
  if (!CUSTOMER_IDENTITY_HEADERS.some((header) => headers.some(value => value.toLowerCase() === header))) {
    throw new Error(i18n._(msg`Include external_id, email or phone; at least one customer identifier is required.`))
  }
  return new Blob([Papa.unparse([headers, ...rows])], { type: 'text/csv;charset=utf-8' })
}

export async function createContactImportTemplate(headers: string[], format: 'csv' | 'xlsx'): Promise<Blob> {
  if (format === 'csv') return new Blob(['\uFEFF', Papa.unparse([headers]), '\r\n'], { type: 'text/csv;charset=utf-8' })
  const { utils, write } = await import('xlsx')
  const sheet = utils.aoa_to_sheet([headers, headers.map(() => '')])
  sheet['!cols'] = headers.map((header) => ({ wch: Math.max(22, header.length + 2) }))
  headers.forEach((_, column) => {
    sheet[utils.encode_cell({ r: 1, c: column })] = { t: 's', v: '', z: '@' }
  })
  const workbook = utils.book_new()
  utils.book_append_sheet(workbook, sheet, 'Customers')
  return new Blob([write(workbook, { type: 'array', bookType: 'xlsx' })], {
    type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  })
}
