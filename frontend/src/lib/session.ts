const key = 'orchestrator.bearer';
const projectKey = 'orchestrator.project';
const tribeKey = 'orchestrator.tribe';

export const session = {
  bearer(): string {
    if (typeof sessionStorage === 'undefined') return '';
    return sessionStorage.getItem(key) ?? '';
  },
  set(value: string) {
    sessionStorage.setItem(key, value);
  },
  clear() {
    sessionStorage.removeItem(key);
    sessionStorage.removeItem(projectKey);
    sessionStorage.removeItem(tribeKey);
  },
  project(): string {
    if (typeof sessionStorage === 'undefined') return '';
    return sessionStorage.getItem(projectKey) ?? '';
  },
  setProject(value: string) {
    sessionStorage.setItem(projectKey, value);
  },
  tribe(): string {
    if (typeof sessionStorage === 'undefined') return '';
    return sessionStorage.getItem(tribeKey) ?? '';
  },
  setTribe(value: string) {
    sessionStorage.setItem(tribeKey, value);
  }
};

const projectRoutes = ['/intake', '/cards', '/settings', '/overview'];

/** Board, intake, card, overview, and project settings pages work inside one tribe's project. */
export function needsTribe(path: string): boolean {
  return path === '/' || projectRoutes.some((route) => path === route || path.startsWith(`${route}/`));
}
