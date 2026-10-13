import { redirect } from '@sveltejs/kit';
import { needsTribe, session } from '$lib/session';

export const ssr = false;
export const prerender = false;

export function load({ url }) {
  if (url.pathname !== '/login' && session.bearer() === '') {
    redirect(307, '/login');
  }
  if (needsTribe(url.pathname) && session.tribe() === '') {
    redirect(307, '/choose-tribe');
  }
}
