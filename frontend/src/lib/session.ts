const key = 'orchestrator.bearer';

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
  }
};
