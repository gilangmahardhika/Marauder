import { untrack } from 'svelte';
import { pageSize } from './filter';

// Call during component setup. The limit goes back to one page whenever key() changes.
export function pager(key: () => string) {
  let limit = $state(pageSize);
  let seen = -1;
  $effect(() => {
    key();
    untrack(() => {
      limit = pageSize;
      seen = -1;
    });
  });
  return {
    get limit() {
      return limit;
    },
    more() {
      limit += pageSize;
    },
    /** When every entry was shown and more arrive, the limit grows so the new ones show too. */
    follow(total: number) {
      if (seen !== -1 && seen <= limit && total > limit) limit = total;
      seen = total;
    }
  };
}
