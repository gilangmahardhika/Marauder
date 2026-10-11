import { untrack } from 'svelte';
import { pageSize } from './filter';

// Call during component setup. The limit goes back to one page whenever key() changes.
export function pager(key: () => string) {
  let limit = $state(pageSize);
  $effect(() => {
    key();
    untrack(() => (limit = pageSize));
  });
  return {
    get limit() {
      return limit;
    },
    more() {
      limit += pageSize;
    }
  };
}
