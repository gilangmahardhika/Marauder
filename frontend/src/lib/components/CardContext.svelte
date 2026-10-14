<script lang="ts">
  import { getConventions, setCardContext } from '$lib/api';
  import { categoryLabel } from '$lib/conventions';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { toast } from '$lib/toast.svelte';
  import type { Card, CardContext, CardContextChange, ConventionCatalog } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SearchSelect from '$lib/ui/SearchSelect.svelte';

  let { card = $bindable() }: { card: Card } = $props();

  type Kind = CardContextChange['kind'];

  let catalog = $state<ConventionCatalog | null>(null);
  let adding = $state<Record<Kind, string>>({ skill: '', rule: '', mcp: '' });
  /** The change in flight as "kind:id"; its button shows the spinner. */
  let pending = $state('');
  let failure = $state(noFailure);
  /** The attach control whose change failed, which shows `fields.itemId`. */
  let failedAttach = $state<Kind | ''>('');
  let body = $state<HTMLElement>();

  const busy = $derived(pending !== '');

  const context = $derived<CardContext>(card.context ?? { rules: [], skills: [], mcp: [], mcpScope: 'all' });
  const canChange = $derived((card.actions ?? []).includes('context'));
  const canAttachServer = $derived((card.actions ?? []).includes('attach-mcp'));

  const ruleOptions = $derived(
    (catalog?.rules ?? [])
      .filter((rule) => !rule.disabled && !context.rules.some((item) => item.id === rule.id))
      .map((rule) => ({ value: rule.id, label: rule.title, detail: categoryLabel(rule.category) }))
  );
  const skillOptions = $derived(
    (catalog?.skills ?? [])
      .filter((skill) => !skill.disabled && !context.skills.some((item) => item.id === skill.id))
      .map((skill) => ({ value: skill.id, label: skill.title, detail: skill.slug }))
  );
  const serverOptions = $derived(
    context.mcp
      .filter((server) => server.enabled && server.state === 'off' && server.origin !== 'card')
      .map((server) => ({ value: server.id, label: server.slug, detail: `${server.toolCount} tools` }))
  );

  const originLabel: Record<string, string> = { project: 'Project', planner: 'Planner', scope: 'Scope', card: 'Card' };

  $effect(() => {
    if (!canChange || catalog !== null) return;
    getConventions()
      .then((body) => (catalog = body))
      .catch(() => (catalog = { rules: [], stacks: [], skills: [] }));
  });

  /** A failure with field errors shows in the panel's alert; any other failure is an error toast. */
  async function change(kind: Kind, itemId: string, mode: CardContextChange['mode']) {
    pending = `${kind}:${itemId}`;
    failure = noFailure;
    failedAttach = '';
    try {
      card = await setCardContext(card.id, { kind, itemId, mode });
      toast.success('Card context saved');
    } catch (error) {
      const next = formFailure(error, mode === 'attach' ? ['itemId'] : []);
      if (Object.keys(next.fields).length === 0) {
        toast.error(failureText(error));
      } else {
        failure = next;
        failedAttach = mode === 'attach' ? kind : '';
        void focusInvalid(body);
      }
    } finally {
      pending = '';
    }
  }

  function attachError(kind: Kind) {
    return failedAttach === kind ? (failure.fields.itemId ?? '') : '';
  }

  function attach(kind: Kind, id: string) {
    adding = { ...adding, [kind]: '' };
    if (id) void change(kind, id, 'attach');
  }
</script>

{#snippet control(kind: Kind, id: string, label: string, state: 'on' | 'off', origin: string)}
  {#if canChange}
    {#if origin === 'card'}
      <Button
        type="button"
        variant="ghost"
        icon={state === 'on' ? 'trash' : undefined}
        disabled={busy}
        loading={pending === `${kind}:${id}`}
        onclick={() => change(kind, id, 'inherit')}
      >
        {state === 'on' ? 'Remove' : 'Restore'}<span class="sr-only"> {label}</span>
      </Button>
    {:else if state === 'on'}
      <Button
        type="button"
        variant="ghost"
        disabled={busy}
        loading={pending === `${kind}:${id}`}
        onclick={() => change(kind, id, 'detach')}
      >
        Detach<span class="sr-only"> {label}</span>
      </Button>
    {/if}
  {/if}
{/snippet}

{#snippet badges(state: 'on' | 'off', origin: string)}
  <Badge tone={origin === 'card' ? 'sky' : 'default'}>{originLabel[origin] ?? origin}</Badge>
  {#if state === 'off'}<Badge tone="amber">Detached</Badge>{/if}
{/snippet}

<Panel title="Context" hint="The rules, skills, and MCP servers this card's attempts carry. Changes apply from the next attempt.">
  <div class="flex flex-col gap-6" bind:this={body}>
    <FormAlert message={failure.message} extra={failure.extra} />
    <section aria-labelledby="context-rules">
      <h3 id="context-rules" class="text-xs font-semibold uppercase tracking-wider text-muted">Rules</h3>
      {#if context.rules.length === 0}
        <p class="mt-2 text-sm text-muted">No rules apply to this card.</p>
      {:else}
        <ul class="mt-2 flex flex-col divide-y divide-line">
          {#each context.rules as rule (rule.id)}
            <li class="flex flex-wrap items-center justify-between gap-3 py-2">
              <span class="flex min-w-0 flex-wrap items-center gap-2">
                <span class="text-sm {rule.state === 'on' ? 'text-ink' : 'text-muted line-through'}">{rule.title}</span>
                <span class="text-xs text-muted">{rule.scope === 'global' ? 'Global' : rule.stackName}</span>
                <Badge>{categoryLabel(rule.category)}</Badge>
                {@render badges(rule.state, rule.origin)}
              </span>
              {@render control('rule', rule.id, rule.title, rule.state, rule.origin)}
            </li>
          {/each}
        </ul>
      {/if}
      {#if canChange}
        <div class="mt-3">
          <SearchSelect
            label="Attach rule"
            id="attach-rule"
            width="lg"
            placeholder={ruleOptions.length === 0 ? 'No other enabled rules' : 'Attach a rule'}
            options={ruleOptions}
            error={attachError('rule')}
            bind:value={adding.rule}
            onchange={(id) => attach('rule', id)}
          />
        </div>
      {/if}
    </section>

    <section aria-labelledby="context-skills">
      <h3 id="context-skills" class="text-xs font-semibold uppercase tracking-wider text-muted">Skills</h3>
      {#if context.skills.length === 0}
        <p class="mt-2 text-sm text-muted">No skills attached.</p>
      {:else}
        <ul class="mt-2 flex flex-col divide-y divide-line">
          {#each context.skills as skill (skill.id)}
            <li class="flex flex-wrap items-center justify-between gap-3 py-2">
              <span class="min-w-0">
                <span class="flex flex-wrap items-center gap-2">
                  <span class="text-sm font-medium {skill.state === 'on' ? 'text-ink' : 'text-muted line-through'}">{skill.title}</span>
                  {@render badges(skill.state, skill.origin)}
                </span>
                {#if skill.useWhen}<span class="mt-0.5 line-clamp-2 text-sm text-muted">{skill.useWhen}</span>{/if}
              </span>
              {@render control('skill', skill.id, skill.title, skill.state, skill.origin)}
            </li>
          {/each}
        </ul>
      {/if}
      {#if canChange}
        <div class="mt-3">
          <SearchSelect
            label="Attach skill"
            id="attach-skill"
            width="lg"
            placeholder={skillOptions.length === 0 ? 'No other enabled skills' : 'Attach a skill'}
            options={skillOptions}
            error={attachError('skill')}
            bind:value={adding.skill}
            onchange={(id) => attach('skill', id)}
          />
        </div>
      {/if}
    </section>

    <section aria-labelledby="context-mcp">
      <h3 id="context-mcp" class="text-xs font-semibold uppercase tracking-wider text-muted">MCP servers</h3>
      {#if context.mcp.length === 0}
        <p class="mt-2 text-sm text-muted">This tribe has no MCP servers.</p>
      {:else}
        {#if context.mcpScope === 'none'}
          <p class="mt-2 text-sm text-muted">The next attempt opens no MCP server.</p>
        {/if}
        <ul class="mt-2 flex flex-col divide-y divide-line">
          {#each context.mcp as server (server.id)}
            <li class="flex flex-wrap items-center justify-between gap-3 py-2">
              <span class="flex min-w-0 flex-wrap items-center gap-2">
                <span class="font-mono text-[13px] {server.state === 'on' ? 'text-ink' : 'text-muted'}">{server.slug}</span>
                <span class="text-xs text-muted">{server.toolCount} tools</span>
                {#if !server.enabled}
                  <Badge>Disabled</Badge>
                {:else}
                  <Badge tone={server.origin === 'card' ? 'sky' : 'default'}>{originLabel[server.origin]}</Badge>
                  <Badge tone={server.state === 'on' ? 'green' : 'default'}>{server.state === 'on' ? 'Opens' : 'Off'}</Badge>
                {/if}
              </span>
              {#if server.enabled}
                {@render control('mcp', server.id, server.slug, server.state, server.origin)}
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
      {#if canAttachServer && serverOptions.length > 0}
        <div class="mt-3">
          <SearchSelect
            label="Attach server"
            id="attach-mcp"
            width="lg"
            placeholder="Attach an MCP server"
            options={serverOptions}
            error={attachError('mcp')}
            bind:value={adding.mcp}
            onchange={(id) => attach('mcp', id)}
          />
        </div>
      {/if}
    </section>
  </div>
  {#snippet footer()}
    <Button href="/settings/conventions">Project conventions</Button>
  {/snippet}
</Panel>
