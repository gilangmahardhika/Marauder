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

export function filterUsers(users: SessionUser[], filter: UserFilter) {
  return users.filter(
    (user) =>
      (filter.role === '' || user.role === filter.role) &&
      hasAccess(user, filter.access) &&
      matchText([user.email], filter.q)
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
