import { api } from '../services/api/client'
import type { ConsoleTheme } from './consoleTheme'

export const saveConsoleTheme = (workspaceId: string, value: ConsoleTheme) => api.post<{console_theme: ConsoleTheme}>('/api/workspaces.setConsoleTheme', {workspace_id:workspaceId,console_theme:value})
