export const shell = $state({
  title: 'Marauder',
  admin: false,
  name: '',
  email: '',
  role: '',
  projectId: '',
  tribeAdminOf: [] as string[]
});

/** A company admin, or a tribe admin of this tribe (RFC-057). */
export function canManageTribe(tribeId: string) {
  return shell.admin || shell.tribeAdminOf.includes(tribeId);
}

export function managesAnyTribe() {
  return shell.admin || shell.tribeAdminOf.length > 0;
}

export function showShell(get: () => { title: string }) {
  $effect(() => {
    shell.title = get().title;
  });
}
