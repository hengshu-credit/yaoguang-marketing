import { createContext, useContext } from 'react'
export const ArchivedMedia = createContext<Record<string, string>>({})
export function useArchivedMedia() { return useContext(ArchivedMedia) }
