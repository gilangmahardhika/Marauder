<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import { mentionAt, mentionChoices } from '$lib/requirements';
  import type { MCPServerMention } from '$lib/types';
  import { autogrow } from './autogrow';
  import { controlBase, labelClass, widths, type Width } from './control';
  import FieldError from './FieldError.svelte';

  let {
    label,
    id,
    hint = '',
    error = '',
    value = $bindable(''),
    width = 'full',
    rows,
    grow = false,
    labelAction,
    cornerAction,
    servers = []
  }: {
    label: string;
    id: string;
    hint?: string;
    error?: string;
    value?: string;
    width?: Width;
    rows?: number;
    grow?: boolean;
    labelAction?: Snippet;
    /** A control at the bottom right inside the textarea; the text keeps clear of it. */
    cornerAction?: Snippet;
    /** The tribe's MCP servers that typing @ lists (RFC-101). */
    servers?: MCPServerMention[];
  } = $props();

  let area = $state<HTMLTextAreaElement | undefined>();
  let at = $state<{ start: number; query: string } | null>(null);
  let dismissed = $state(-1);
  let active = $state(0);

  const listId = $derived(`${id}-mentions`);
  const hintId = $derived(`${id}-hint`);
  const fullHint = $derived(`${hint ? `${hint} ` : ''}Type @ to name an MCP server.`);
  const choices = $derived(at ? mentionChoices(servers, at.query) : []);
  const open = $derived(at !== null);

  function follow() {
    if (!area) return;
    const found = area.selectionStart === area.selectionEnd ? mentionAt(value, area.selectionStart) : null;
    if (!found || found.start === dismissed) {
      if (!found) dismissed = -1;
      at = null;
      return;
    }
    if (at?.start !== found.start || at.query !== found.query) active = 0;
    at = found;
  }

  function close() {
    at = null;
  }

  async function insert(slug: string) {
    if (!at || !area) return;
    const caret = area.selectionStart;
    const text = `@${slug} `;
    value = value.slice(0, at.start) + text + value.slice(caret);
    const position = at.start + text.length;
    at = null;
    dismissed = -1;
    await tick();
    area.focus();
    area.setSelectionRange(position, position);
  }

  function onkeydown(event: KeyboardEvent) {
    if (event.isComposing || !open) return;
    if (event.key === 'ArrowDown' && choices.length > 0) {
      event.preventDefault();
      active = Math.min(active + 1, choices.length - 1);
    } else if (event.key === 'ArrowUp' && choices.length > 0) {
      event.preventDefault();
      active = Math.max(active - 1, 0);
    } else if ((event.key === 'Enter' || event.key === 'Tab') && choices[active]) {
      event.preventDefault();
      void insert(choices[active].value);
    } else if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      dismissed = at?.start ?? -1;
      close();
    }
  }

  $effect(() => {
    if (!open) return;
    document.getElementById(`${id}-mention-${active}`)?.scrollIntoView?.({ block: 'nearest' });
  });
</script>

<div class="flex w-full flex-col gap-1.5 {widths[width]}">
  {#if labelAction}
    <div class="flex items-center justify-between gap-2">
      <label for={id} class={labelClass}>{label}</label>
      {@render labelAction()}
    </div>
  {:else}
    <label for={id} class={labelClass}>{label}</label>
  {/if}
  <p id={hintId} class="text-xs text-muted">{fullHint}</p>
  <div class="relative">
    <!-- svelte-ignore a11y_role_supports_aria_props_implicit -->
    <textarea
      {id}
      {rows}
      bind:this={area}
      aria-describedby={error ? `${hintId} ${id}-error` : hintId}
      aria-invalid={error ? true : undefined}
      aria-autocomplete="list"
      aria-controls={listId}
      aria-expanded={open}
      aria-activedescendant={open && choices.length > 0 ? `${id}-mention-${active}` : undefined}
      class="{controlBase} w-full {cornerAction ? 'pt-2.5 pb-14' : 'py-2.5'} {grow ? 'resize-none overflow-hidden' : ''}"
      use:autogrow={{ on: grow, value }}
      bind:value
      oninput={follow}
      onclick={follow}
      onkeyup={(event) => {
        if (event.key === 'ArrowLeft' || event.key === 'ArrowRight' || event.key === 'Home' || event.key === 'End') follow();
      }}
      {onkeydown}
      onblur={close}
    ></textarea>
    {#if cornerAction}
      <div class="absolute right-1.5 bottom-1">{@render cornerAction()}</div>
    {/if}
    {#if open}
      <ul
        id={listId}
        role="listbox"
        aria-label="MCP servers"
        class="absolute top-full z-30 mt-1 max-h-64 w-full max-w-sm overflow-auto rounded-panel border border-line bg-surface p-1 shadow-lg"
      >
        {#each choices as option, index (option.value)}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <li
            id={`${id}-mention-${index}`}
            role="option"
            tabindex="-1"
            aria-selected={index === active}
            class="flex min-h-11 cursor-pointer items-center gap-2 rounded-control px-3 py-1.5 {index === active ? 'bg-ground' : ''}"
            onpointerdown={(event) => event.preventDefault()}
            onpointerenter={() => (active = index)}
            onclick={() => void insert(option.value)}
          >
            <span class="min-w-0 flex-1 truncate font-mono text-[13px] text-ink">@{option.label}</span>
            <span class="shrink-0 text-xs text-muted">{option.detail}</span>
          </li>
        {:else}
          <li class="px-3 py-2 text-sm text-muted">{servers.length === 0 ? 'No MCP servers in this tribe.' : 'No matches'}</li>
        {/each}
      </ul>
    {/if}
  </div>
  <FieldError {id} {error} />
</div>
