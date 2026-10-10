import { redirect } from '@sveltejs/kit';
import { session } from '$lib/session';

export const ssr = false;
export const prerender = false;

export function load({ url }) {
  if (url.pathname !== '/login' && session.bearer() === '') {
    redirect(307, '/login');
  }
}
