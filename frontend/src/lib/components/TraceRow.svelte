<script lang="ts">
  import { clock } from '$lib/format';
  import { describe, kindLabel, type Tone } from '$lib/trace';
  import type { TraceEvent } from '$lib/types';
  import Icon from '$lib/ui/Icon.svelte';
  import type { IconName } from '$lib/ui/icons';
  import TraceText from './TraceText.svelte';

  let { event }: { event: TraceEvent } = $props();

  let open = $state(false);
  let raw = $state(false);

  const words = $derived(describe(event));
  const detailId = $derived(`trace-${event.id}`);
  const rawId = $derived(`trace-raw-${event.id}`);

  const toolIcons: Record<string, IconName> = {
    conventions_context: 'rules',
    linked_context: 'link',
    list_files: 'eye',
    search_files: 'eye',
    read_file: 'eye',
    read_linked_file: 'eye',
    read_diff: 'eye',
    edit_file: 'skill',
    replace_in_file: 'skill',
    delete_file: 'skill',
    move_file: 'skill',
    todo_write: 'check',
    repo_context: 'rules',
    run_local_check: 'terminal',
    run_command: 'terminal',
    commit: 'git',
    comment_pr: 'link',
    verdict: 'check'
  };

  const kindIcons: Record<string, IconName> = {
    transition: 'board',
    model_call: 'spark',
    reviewers_requested: 'users',
    deploy: 'git',
    health: 'shield',
    verify: 'shield',
    route: 'router',
    config_changed: 'router'
  };

  const icon = $derived<IconName>(
    words.tone === 'red' || event.kind === 'transition_rejected'
      ? 'alert'
      : event.kind === 'tool_call'
        ? (toolIcons[String((event.payload as { name?: unknown } | null)?.name ?? '')] ?? 'terminal')
        : (kindIcons[event.kind] ?? 'check')
  );

  const labelTone: Record<Tone, string> = {
    default: 'text-ink',
    green: 'text-emerald-800',
    amber: 'text-amber-800',
    sky: 'text-sky-800',
    red: 'text-red-800'
  };
  const iconTone: Record<Tone, string> = {
    default: 'text-muted',
    green: 'text-emerald-700',
    amber: 'text-amber-700',
    sky: 'text-sky-700',
    red: 'text-red-700'
  };

  const toggle = 'inline-flex min-h-11 w-fit items-center gap-2 rounded-control px-2 text-xs font-medium text-muted hover:bg-ground hover:text-ink';
</script>

<li class={words.tone === 'red' ? 'bg-red-50/70' : ''}>
  <button
    type="button"
    class="grid min-h-11 w-full grid-cols-[4.25rem_1rem_minmax(0,1fr)] items-center gap-x-3 px-3 py-1.5 text-left hover:bg-ground/70 sm:grid-cols-[4.25rem_1rem_minmax(0,1fr)_auto]"
    aria-expanded={open}
    aria-controls={detailId}
    onclick={() => (open = !open)}
  >
    <time class="font-mono text-xs text-muted" datetime={event.ts} title={event.ts}>{clock(event.ts)}</time>
    <Icon name={icon} class="size-4 {iconTone[words.tone]}" />
    <span class="flex min-w-0 items-baseline gap-2">
      <span class="max-w-full shrink-0 truncate text-sm font-medium {labelTone[words.tone]}">{words.label}</span>
      {#if words.detail}<span class="min-w-0 truncate text-sm text-muted">{words.detail}</span>{/if}
    </span>
    {#if words.meta}<span class="hidden shrink-0 font-mono text-xs text-muted sm:block">{words.meta}</span>{/if}
  </button>
  {#if open}
    <div id={detailId} class="flex flex-col gap-3 border-t border-line px-3 py-3 sm:pl-[6.5rem]">
      {#if words.detail}<p class="text-sm break-words text-ink">{words.detail}</p>{/if}
      {#if words.text}<TraceText text={words.text} />{/if}
      <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 text-xs">
        <dt class="text-muted">Kind</dt>
        <dd class="text-ink">{kindLabel(event.kind)} <span class="font-mono text-muted">{event.kind}</span></dd>
        <dt class="text-muted">Time</dt>
        <dd class="font-mono text-ink">{event.ts}</dd>
        {#if words.meta}
          <dt class="text-muted">Usage</dt>
          <dd class="font-mono text-ink">{words.meta}</dd>
        {/if}
        {#if event.attemptId}
          <dt class="text-muted">Attempt</dt>
          <dd class="font-mono break-all text-ink">{event.attemptId}</dd>
        {/if}
        <dt class="text-muted">Event</dt>
        <dd class="font-mono break-all text-ink">{event.id}</dd>
      </dl>
      <button type="button" class={toggle} aria-expanded={raw} aria-controls={rawId} onclick={() => (raw = !raw)}>
        <Icon name="chevron" class="size-4 transition-transform {raw ? 'rotate-180' : ''}" />
        Raw event
      </button>
      {#if raw}
        <div id={rawId}><TraceText text={JSON.stringify(event.payload, null, 2)} /></div>
      {/if}
    </div>
  {/if}
</li>
