export const shell = $state({
  title: 'Marauder',
  message: '',
  admin: false,
  email: '',
  role: '',
  projectId: ''
});

export function showShell(get: () => { title: string; message?: string }) {
  $effect(() => {
    const next = get();
    shell.title = next.title;
    shell.message = next.message ?? '';
  });
}
