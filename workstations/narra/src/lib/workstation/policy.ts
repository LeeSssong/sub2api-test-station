export const SESSION_SECONDS = 6 * 60 * 60;
export function isSessionLive(created: number, now: number) { return now >= created && now < created + SESSION_SECONDS; }
export function fixedSiteProvider(baseUrl: string, apiKey: string, model: string) {
  return { baseUrl, apiKey, model, models: [model], remember: false, label: '本站' };
}
export function allowedWorkstationPath(path: string) {
  return path === '/' || path === '/create' || path === '/admin/generations' || path === '/api/generate' || path === '/api/provider-models/probe' || path.startsWith('/api/workstation/') || path.startsWith('/api/me/generations/') || path.startsWith('/_next/') || path === '/favicon.ico' || path === '/api/healthz' || path === '/api/readyz';
}
