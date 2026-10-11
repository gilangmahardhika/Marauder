<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { setParams } from '$lib/params';
  import { control } from './control';

  let {
    label,
    placeholder = '',
    value = '',
    param = 'q'
  }: { label: string; placeholder?: string; value?: string; param?: string } = $props();

  let query = $state(untrack(() => value));
  let timer: ReturnType<typeof setTimeout> | undefined;

  $effect(() => {
    const next = value;
    untrack(() => {
      if (timer === undefined && next !== query.trim()) query = next;
    });
  });

  function input(next: string) {
    query = next;
    clearTimeout(timer);
    timer = setTimeout(() => {
      timer = undefined;
      setParams({ [param]: next.trim() });
    }, 200);
  }

  onDestroy(() => clearTimeout(timer));
</script>

<label class="min-w-64 flex-1">
  <span class="sr-only">{label}</span>
  <input
    type="search"
    class={control}
    {placeholder}
    value={query}
    oninput={(event) => input(event.currentTarget.value)}
    onkeydown={(event) => {
      if (event.key === 'Enter') event.preventDefault();
    }}
  />
</label>
