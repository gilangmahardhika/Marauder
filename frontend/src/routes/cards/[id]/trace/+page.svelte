<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getTrace, unreachable } from '$lib/api';
  import Live from '$lib/components/Live.svelte';
  import type { TraceEvent } from '$lib/types';

  let events = $state<TraceEvent[]>([]);
  let message = $state('Loading trace');

  $effect(() => {
    const id = $page.params.id;
    if (!id) return;
    void getTrace(id)
      .then((body) => {
        events = body.events;
        message = 'Trace';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });
</script>

<h1>Trace</h1>
<Live {message} />
<ul>
  {#each events as event}
    <li>
      <p>{event.ts}</p>
      <p>{event.kind}</p>
      {#if event.attemptId}<p>{event.attemptId}</p>{/if}
      <pre>{JSON.stringify(event.payload, null, 2)}</pre>
    </li>
  {/each}
</ul>
