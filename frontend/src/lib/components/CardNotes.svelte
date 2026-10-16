<script lang="ts">
  import { stamp } from '$lib/format';
  import type { CardNote } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Markdown from './Markdown.svelte';
  import TraceText from './TraceText.svelte';

  let { notes }: { notes: CardNote[] } = $props();

  let earlier = $state(false);

  const labels: Record<string, string> = {
    review: 'AI review',
    approver: 'Approver',
    local_check: 'Local check',
    ci: 'CI checks',
    dev_check: 'Dev check',
    no_commit: 'No commit',
    instructions: 'Instructions'
  };
  /** Only people and the reviewer model write prose. Check output stays plain text. */
  const prose = new Set(['review', 'approver', 'instructions']);

  /** Notes the next attempt reads stay open: newest AI review, newest other note, and current instructions (RFC-131). */
  const read = $derived(
    notes.filter(
      (note) =>
        note === notes.find((n) => n.source === 'review') ||
        note === notes.find((n) => n.source !== 'instructions' && n.source !== 'review') ||
        (note === notes.find((n) => n.source === 'instructions') && note.note !== '')
    )
  );
  const open = $derived(read.length > 0 ? read : notes.slice(0, 1));
  const rest = $derived(notes.filter((note) => !open.includes(note)));
</script>

{#snippet entry(note: CardNote)}
  <article class="flex min-w-0 flex-col gap-2">
    <div class="flex flex-wrap items-center gap-2 text-xs text-muted">
      <Badge tone={prose.has(note.source) ? 'amber' : 'default'}>
        {labels[note.source] ?? note.source}
      </Badge>
      <span>{stamp(note.createdAt)}</span>
      {#if note.attemptN > 0}<span>· attempt {note.attemptN}</span>{/if}
    </div>
    {#if note.source === 'instructions' && note.note === ''}
      <p class="text-sm text-muted">Instructions cleared</p>
    {:else if prose.has(note.source)}
      <Markdown text={note.note} headingBase={4} />
    {:else}
      <TraceText text={note.note} />
    {/if}
  </article>
{/snippet}

{#if open.length > 0}
  <Panel title="Notes" hint="Newest first. The next attempt gets the newest AI review note and the newest other note, plus instructions.">
    {#each open as note, index (index)}
      <div class={index > 0 ? 'border-t border-line pt-5' : ''}>{@render entry(note)}</div>
    {/each}
    {#if rest.length > 0}
      {#if earlier}
        {#each rest as note, index (index)}
          <div class="border-t border-line pt-5">{@render entry(note)}</div>
        {/each}
      {:else}
        <div>
          <Button type="button" variant="ghost" onclick={() => (earlier = true)}>
            Show {rest.length} earlier {rest.length === 1 ? 'note' : 'notes'}
          </Button>
        </div>
      {/if}
    {/if}
  </Panel>
{/if}
