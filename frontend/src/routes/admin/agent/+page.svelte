<script lang="ts">
  import { getAgent, getLLM, getRouter, putAgent, testAgent } from '$lib/api';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { AgentKind, ClaudeCodeOptions, Family } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import ChoiceCards, { type Choice } from '$lib/ui/ChoiceCards.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import Select from '$lib/ui/Select.svelte';

  const shown = [
    'kind',
    'claudeCode.family',
    'claudeCode.model',
    'claudeCode.smallModel',
    'claudeCode.maxTurns',
    'claudeCode.auth',
    'claudeCode.instructions',
    'claudeCode.binary'
  ];
  const kindOptions: Choice[] = [
    { value: 'builtin', label: 'Built-in', description: "Runs the orchestrator's own tool loop." },
    { value: 'claude_code', label: 'Claude Code', description: "Runs the claude CLI in the card's worktree." }
  ];
  const subhead = 'text-[11px] font-semibold uppercase tracking-wider text-muted';

  let kind = $state<AgentKind>('builtin');
  let family = $state('');
  let model = $state('');
  let smallModel = $state('');
  let maxTurns = $state<string | number>('');
  let auth = $state<ClaudeCodeOptions['auth']>('auto');
  let allowWeb = $state(false);
  let instructions = $state('');
  let binary = $state('');
  let families = $state<Family[]>([]);
  let routerTurns = $state(40);
  let loading = $state('Loading agent');
  let loadError = $state('');
  let busy = $state(false);
  let checking = $state(false);
  let check = $state<{ ok: boolean; text: string } | null>(null);
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  const anthropic = $derived(families.filter((item) => item.protocol === 'anthropic'));
  const models = $derived(anthropic.find((item) => item.id === family)?.models ?? []);
  const noKey = $derived(anthropic.length > 0 && anthropic.every((item) => !item.keySet));

  showShell(() => ({ title: 'Agent' }));

  function load() {
    loading = 'Loading agent';
    loadError = '';
    void Promise.all([getAgent(), getLLM(), getRouter()])
      .then(([agent, llm, router]) => {
        kind = agent.kind;
        family = agent.claudeCode.family;
        model = agent.claudeCode.model;
        smallModel = agent.claudeCode.smallModel;
        maxTurns = agent.claudeCode.maxTurns || '';
        auth = agent.claudeCode.auth || 'auto';
        allowWeb = agent.claudeCode.allowWeb;
        instructions = agent.claudeCode.instructions;
        binary = agent.claudeCode.binary;
        families = llm.families;
        routerTurns = router.budgets.maxModelTurnsPerAttempt || 40;
      })
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(() => {
    load();
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = noFailure;
    try {
      const saved = await putAgent({
        kind,
        claudeCode: {
          family,
          model,
          smallModel,
          maxTurns: Number(maxTurns) || 0,
          auth,
          allowWeb,
          instructions,
          binary: binary.trim()
        }
      });
      kind = saved.kind;
      toast.success(saved.kind === 'claude_code' ? 'Implement jobs now use Claude Code.' : 'Implement jobs now use the built-in agent.');
    } catch (error) {
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }

  async function runCheck() {
    checking = true;
    check = null;
    try {
      const result = await testAgent();
      check = result.ok ? { ok: true, text: result.version ?? 'Claude Code answered.' } : { ok: false, text: result.message ?? 'Claude Code did not answer.' };
    } catch (error) {
      check = { ok: false, text: failureText(error) };
    } finally {
      checking = false;
    }
  }
</script>

<Page
  title="Agent"
  description="Which agent writes the code of an implement job. Applies to every project, on this server and on runners."
  {loading}
  error={loadError}
  onretry={load}
>
  <form onsubmit={save} bind:this={form} novalidate>
    <div class="flex flex-col gap-6">
      <FormAlert message={failure.message} extra={failure.extra} />
      <Panel title="Implement agent">
        <div class="flex flex-col gap-3">
          <ChoiceCards
            label="Implement agent"
            id="agent-kind"
            columns={2}
            options={kindOptions}
            value={kind}
            disabled={busy}
            onchange={(value) => (kind = value as AgentKind)}
          />
          {#if failure.fields.kind}<p class="text-sm text-red-800">{failure.fields.kind}</p>{/if}
        </div>
      </Panel>

      {#if kind === 'claude_code'}
        <Panel title="Claude Code" hint="The family's key is sent to the runner with each job and never written to disk.">
          {#if anthropic.length === 0}
            <Notice tone="warning">
              No family uses the anthropic protocol. Add one on <a class="text-accent hover:underline" href="/admin/llm">LLM APIs</a>, for example
              DeepSeek at https://api.deepseek.com/anthropic.
            </Notice>
          {:else if noKey}
            <Notice tone="warning">
              No anthropic family has an API key. Set one on <a class="text-accent hover:underline" href="/admin/llm">LLM APIs</a>.
            </Notice>
          {/if}

          <div class="flex flex-col gap-4">
            <h3 class={subhead}>Model</h3>
            <div class="grid items-start gap-5 sm:grid-cols-2">
              <Select label="Model family" id="family" width="full" error={failure.fields['claudeCode.family']} bind:value={family}>
                <option value="">Choose a family</option>
                {#each anthropic as item (item.id)}
                  <option value={item.id} disabled={!item.keySet}>{item.keySet ? item.id : `${item.id} (No API key)`}</option>
                {/each}
              </Select>
              <Select
                label="Model"
                id="model"
                width="full"
                hint="Passed to claude --model. Use a Claude Code alias or Anthropic model id from the family (for example sonnet or claude-sonnet-4-5), not a Cursor chat model slug."
                error={failure.fields['claudeCode.model']}
                bind:value={model}
              >
                <option value="">Choose a model</option>
                {#each models as item (item)}<option value={item}>{item}</option>{/each}
              </Select>
              <Select
                label="Small model (optional)"
                id="small-model"
                width="full"
                error={failure.fields['claudeCode.smallModel']}
                bind:value={smallModel}
              >
                <option value="">Same as model</option>
                {#each models as item (item)}<option value={item}>{item}</option>{/each}
              </Select>
              <Select label="Auth header" id="auth" width="full" error={failure.fields['claudeCode.auth']} bind:value={auth}>
                <option value="auto">Auto</option>
                <option value="apiKey">x-api-key</option>
                <option value="bearer">Bearer</option>
              </Select>
            </div>
          </div>

          <div class="flex flex-col gap-4 border-t border-line pt-5">
            <h3 class={subhead}>Limits</h3>
            <Field
              label="Max turns"
              id="max-turns"
              type="number"
              width="sm"
              hint="Empty uses the router's turn budget"
              placeholder={String(routerTurns)}
              error={failure.fields['claudeCode.maxTurns']}
              bind:value={maxTurns}
            />
            <Checkbox
              id="allow-web"
              label="Allow web fetch and search"
              hint="Off keeps Claude Code to the worktree and the project's allowed commands."
              bind:checked={allowWeb}
            />
            <Field
              label="Extra instructions"
              id="instructions"
              type="textarea"
              width="full"
              rows={4}
              hint="Added to Claude Code's system prompt. At most 8 KiB."
              error={failure.fields['claudeCode.instructions']}
              bind:value={instructions}
            />
          </div>

          <div class="flex flex-col gap-4 border-t border-line pt-5">
            <h3 class={subhead}>This server</h3>
            <Field
              label="Claude binary on this server"
              id="binary"
              width="full"
              mono
              hint="Empty uses claude on PATH. Runners use their own."
              placeholder="/usr/local/bin/claude"
              error={failure.fields['claudeCode.binary']}
              bind:value={binary}
            />
            <div class="flex flex-wrap items-center gap-3">
              <Button type="button" icon="terminal" loading={checking} onclick={() => void runCheck()}>Check</Button>
              {#if check}
                <span
                  class="inline-flex items-center rounded-control border px-2 py-1 text-xs {check.ok
                    ? 'border-emerald-200 bg-emerald-50 text-emerald-800'
                    : 'border-red-200 bg-red-50 text-red-800'}"
                  role="status">{check.text}</span
                >
              {/if}
            </div>
          </div>
        </Panel>
      {/if}
    </div>
    <SaveBar {busy} />
  </form>
</Page>
