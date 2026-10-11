import { get } from 'svelte/store';
import { goto } from '$app/navigation';
import { page } from '$app/stores';
import { withParams } from './filter';

export function setParams(updates: Record<string, string>) {
  void goto(withParams(get(page).url, updates), { replaceState: true, keepFocus: true, noScroll: true });
}
