<script lang="ts">
  import type { Snippet } from 'svelte';
  import AcceptanceList from '$lib/components/AcceptanceList.svelte';
  import Markdown from '$lib/components/Markdown.svelte';
  import MentionList from '$lib/components/MentionList.svelte';
  import RefinePanel from '$lib/components/RefinePanel.svelte';
  import { acceptanceChecks, fromLines, mentions } from '$lib/requirements';
  import type { MCPServerMention, ModelChoice, RefineField, RefineStream } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import { labelClass } from '$lib/ui/control';
  import Field from '$lib/ui/Field.svelte';
  import FieldError from '$lib/ui/FieldError.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import MentionTextarea from '$lib/ui/MentionTextarea.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';

  let {
    id = $bindable(''),
    statement = $bindable(''),
    acceptance = $bindable(''),
    idLocked = false,
    fields = {},
    refine,
    models = [],
    implementModel = '',
    servers = []
  }: {
    id?: string;
    statement?: string;
    acceptance?: string;
    idLocked?: boolean;
    fields?: Record<string, string>;
    /** Streams the implement model's new text for one field (RFC-087). Without it the form has no refine buttons. */
    refine?: RefineStream;
    /** Models a refine may call, and the implement route's model it starts on (RFC-124). */
    models?: ModelChoice[];
    implementModel?: string;
    /** The tribe's MCP servers that a statement or acceptance line may mention (RFC-101). */
    servers?: MCPServerMention[];
  } = $props();

  let view = $state('write');
  const checks = $derived(acceptanceChecks(acceptance));
  const statementMentions = $derived(mentions(statement));
  const acceptanceMentions = $derived(mentions(acceptance));
  let open = $state({ statement: false, acceptance: false });
  let model = $derived(implementModel);
  let thinking = $state('');
  let effort = $state('');
  let autostart = $state(false);
  let offeredFor = $state<string | null>(null);
  const offering = $derived(offeredFor !== null && offeredFor === statement && !open.acceptance);

  function toggle(field: RefineField) {
    open[field] = !open[field];
    if (field === 'acceptance') {
      autostart = false;
      offeredFor = null;
    }
  }

  function apply(field: RefineField, text: string) {
    if (field === 'statement') {
      statement = text;
      offeredFor = text;
    } else acceptance = fromLines(text === '' ? [] : text.split('\n'));
    open[field] = false;
  }

  function refineAcceptance() {
    offeredFor = null;
    autostart = true;
    open.acceptance = true;
  }

  const views = [
    { value: 'write', label: 'Write' },
    { value: 'preview', label: 'Preview' }
  ];
</script>

<Field
  label="Id"
  id="requirement-id"
  width="md"
  mono
  disabled={idLocked}
  placeholder={idLocked ? '' : 'REQ-012'}
  hint={idLocked ? 'Cards and pull requests refer to this id, so it cannot change.' : 'Leave blank to use the next free REQ- number.'}
  error={fields.id}
  bind:value={id}
/>
<div class="flex flex-col gap-2">
  <Segmented label="Requirement view" options={views} value={view} onchange={(next) => (view = next)} />
  <div class="flex flex-col gap-5">
    {#if view === 'write'}
      <div class="flex flex-col gap-2">
        <MentionTextarea
          label="Statement"
          id="requirement-statement"
          rows={8}
          grow
          hint="Markdown works: headings, lists, code, bold, italic, and links."
          error={fields.statement}
          cornerAction={refine ? statementAction : undefined}
          {servers}
          bind:value={statement}
        />
        <MentionList mentions={statementMentions} {servers} />
        {#if refine && open.statement}
          <div id="refine-statement-panel">
            <RefinePanel
              field="statement"
              {statement}
              {acceptance}
              stream={refine}
              {models}
              {implementModel}
              bind:model
              bind:thinking
              bind:effort
              onapply={(text) => apply('statement', text)}
              onclose={() => toggle('statement')}
            />
          </div>
        {:else if refine && offering}
          {@render chainOffer()}
        {/if}
      </div>
      <div class="flex flex-col gap-2">
        <MentionTextarea
          label="Acceptance"
          id="requirement-acceptance"
          rows={6}
          grow
          hint="One check per line. If you paste a markdown list, only its items are saved. Markdown works inside a line: code, bold, italic, and links."
          error={fields.acceptance}
          cornerAction={refine ? acceptanceAction : undefined}
          {servers}
          bind:value={acceptance}
        />
        <MentionList mentions={acceptanceMentions} {servers} />
        {#if refine && open.acceptance}
          <div id="refine-acceptance-panel">
            <RefinePanel
              field="acceptance"
              {statement}
              {acceptance}
              stream={refine}
              {autostart}
              {models}
              {implementModel}
              bind:model
              bind:thinking
              bind:effort
              onapply={(text) => apply('acceptance', text)}
              onclose={() => toggle('acceptance')}
            />
          </div>
        {/if}
      </div>
    {:else}
      {@render preview('Statement', 'min-h-48', statement.trim() === '', fields.statement, statementPreview)}
      {@render preview('Acceptance', 'min-h-36', checks.lines.length === 0, fields.acceptance, acceptancePreview)}
    {/if}
  </div>
</div>
{#snippet statementAction()}
  {@render refineButton('statement')}
{/snippet}

{#snippet acceptanceAction()}
  {@render refineButton('acceptance')}
{/snippet}

{#snippet chainOffer()}
  <div class="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-control border border-accent/20 bg-accent/5 py-0.5 pr-0.5 pl-3">
    <Icon name="check" class="size-4 shrink-0 text-accent" />
    <p class="text-sm font-medium text-ink" role="status">Statement updated.</p>
    <Button variant="ghost" icon="sparkles" onclick={refineAcceptance}>
      {checks.lines.length > 0 ? 'Refine acceptance to match' : 'Draft acceptance from it'}
    </Button>
    <button
      type="button"
      class="ml-auto inline-flex size-11 items-center justify-center rounded-control text-muted hover:bg-ground hover:text-ink"
      aria-label="Dismiss"
      onclick={() => (offeredFor = null)}
    >
      <Icon name="close" class="size-4" />
    </button>
  </div>
{/snippet}

{#snippet refineButton(field: RefineField)}
  <button
    type="button"
    class="group inline-flex min-h-11 items-center rounded-full focus-visible:outline-hidden"
    aria-label={`Refine ${field} with AI`}
    aria-expanded={open[field]}
    aria-controls={`refine-${field}-panel`}
    onclick={() => toggle(field)}
  >
    <span
      class="inline-flex h-8 items-center gap-1.5 rounded-control px-2 text-xs font-medium text-accent underline-offset-4 transition-colors duration-150 group-hover:underline group-focus-visible:underline"
    >
      <Icon name="sparkles" class="size-3.5" />
      Refine with AI
      <Icon name="chevron" class="size-3 group-aria-expanded:rotate-180 motion-safe:transition-transform" />
    </span>
  </button>
{/snippet}

{#snippet statementPreview()}
  <Markdown text={statement} />
{/snippet}

{#snippet acceptancePreview()}
  <AcceptanceList lines={checks.lines} label="Acceptance lines" />
  {#if checks.skipped > 0}
    <p class="mt-3 border-t border-line pt-2 text-xs text-muted">
      {checks.skipped === 1
        ? '1 line is not a list item, so it is not saved.'
        : `${checks.skipped} lines are not list items, so they are not saved.`}
    </p>
  {/if}
{/snippet}

{#snippet preview(label: string, height: string, empty: boolean, error: string | undefined, body: Snippet)}
  <div class="flex flex-col gap-1.5">
    <span class={labelClass}>{label}</span>
    <div class="{height} rounded-control border border-line bg-surface px-3 py-2.5" aria-label={`${label} preview`} role="region">
      {#if empty}
        <p class="text-sm text-muted">Nothing to preview</p>
      {:else}
        {@render body()}
      {/if}
    </div>
    <FieldError id={`requirement-${label.toLowerCase()}-preview`} {error} />
  </div>
{/snippet}
