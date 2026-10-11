import { matchText, type Option } from './filter';
import { jobRoles } from './roles';
import type { SessionUser } from './types';

export type Access = 'all' | 'admins' | 'active' | 'disabled';

export const accessValues = ['all', 'admins', 'active', 'disabled'] as const;

export const roleOptions: Option[] = [{ value: '', label: 'All roles' }, ...jobRoles];

export type UserFilter = {
  q: string;
  role: string;
  access: Access;
};

function hasAccess(user: SessionUser, access: Access) {
  switch (access) {
    case 'admins':
      return user.admin;
    case 'active':
      return !user.disabled;
    case 'disabled':
      return Boolean(user.disabled);
    default:
      return true;
  }
}

export type Person = { name?: string; email: string; githubLogin?: string | null };

/** The label for a person: their name, else their GitHub login, else their email (RFC-058). */
export function displayName(person: Person) {
  return person.name || person.githubLogin || person.email;
}

/** The label plus what tells two people apart: "Ada Lovelace (ada)", or "ada (ada@example.com)" without a name. */
export function describePerson(person: Person) {
  const label = displayName(person);
  const detail = person.name ? person.githubLogin || person.email : person.githubLogin ? person.email : '';
  return detail ? `${label} (${detail})` : label;
}

export function filterUsers<T extends SessionUser>(users: T[], filter: UserFilter): T[] {
  return users.filter(
    (user) =>
      (filter.role === '' || user.role === filter.role) &&
      hasAccess(user, filter.access) &&
      matchText([user.name, user.email], filter.q)
  );
}

export function accessOptions(users: SessionUser[]): Option<Access>[] {
  const count = (access: Access) => users.filter((user) => hasAccess(user, access)).length;
  return [
    { value: 'all', label: 'All', count: count('all') },
    { value: 'admins', label: 'Admins', count: count('admins') },
    { value: 'active', label: 'Active', count: count('active') },
    { value: 'disabled', label: 'Disabled', count: count('disabled') }
  ];
}
