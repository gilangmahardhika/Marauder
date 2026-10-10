const key = 'orchestrator.bearer';
const projectKey = 'orchestrator.project';

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
  },
  project(): string {
    if (typeof sessionStorage === 'undefined') return '';
    return sessionStorage.getItem(projectKey) ?? '';
  },
  setProject(value: string) {
    sessionStorage.setItem(projectKey, value);
  }
};
