export const PUBLIC_ROUTES = ['/signin', '/accept-invitation', '/logout'] as const
export type PublicRoute = (typeof PUBLIC_ROUTES)[number]

export const isPublicRoute = (path: string): boolean => {
  return PUBLIC_ROUTES.includes(path as PublicRoute)
}
