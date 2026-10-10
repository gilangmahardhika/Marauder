<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getTrace, unreachable } from '$lib/api';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { TraceEvent } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';

  let events = $state<TraceEvent[]>([]);
  let loaded = $state(false);
  let message = $state('Loading trace');

  const cardId = $derived($page.params.id ?? '');

  showShell(() => ({ title: 'Trace', message }));

  $effect(() => {
    const id = $page.params.id;
    if (!id || shell.projectId === '') return;
    void getTrace(id)
      .then((body) => {
        events = body.events;
        loaded = true;
        message = 'Trace';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });
</script>

<Page title="Trace" description={`Events for ${cardId}, oldest first.`} width="form">
  {#snippet actions()}
    <Button href={`/cards/${cardId}`}>Back to card</Button>
  {/snippet}
  {#if loaded && events.length === 0}
    <EmptyState title="No events yet" />
  {:else}
    <ol class="flex flex-col gap-4 border-l border-line pl-6">
      {#each events as event (event.id)}
        <li class="relative">
          <span class="absolute -left-[29px] top-4 size-2.5 rounded-full border-2 border-ground bg-accent" aria-hidden="true"
          ></span>
          <div class="rounded-panel border border-line bg-surface">
            <div class="flex flex-wrap items-center gap-2 border-b border-line px-4 py-2.5">
              <Badge>{event.kind}</Badge>
              {#if event.attemptId}<span class="font-mono text-xs text-muted">{event.attemptId}</span>{/if}
              <span class="ml-auto font-mono text-xs text-muted">{event.ts}</span>
            </div>
            <pre class="overflow-x-auto px-4 py-3 font-mono text-xs leading-relaxed text-ink">{JSON.stringify(
                event.payload,
                null,
                2
              )}</pre>
          </div>
        </li>
      {/each}
    </ol>
  {/if}
</Page>
