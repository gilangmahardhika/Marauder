import { flushSync } from 'svelte';
import { expect, test } from 'vitest';
import { pageSize } from './filter';
import { pager } from './pager.svelte';

function withPager(run: (paging: ReturnType<typeof pager>) => void) {
  const stop = $effect.root(() => {
    const paging = pager(() => 'key');
    flushSync();
    run(paging);
  });
  stop();
}

test('follow grows the limit when every entry was shown', () => {
  withPager((paging) => {
    paging.follow(10);
    paging.follow(pageSize + 5);
    expect(paging.limit).toBe(pageSize + 5);
    paging.follow(pageSize + 9);
    expect(paging.limit).toBe(pageSize + 9);
  });
});

test('follow keeps the limit when some entries were hidden', () => {
  withPager((paging) => {
    paging.follow(pageSize + 20);
    paging.follow(pageSize + 30);
    expect(paging.limit).toBe(pageSize);
    paging.more();
    expect(paging.limit).toBe(pageSize * 2);
  });
});
