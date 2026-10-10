<script lang="ts">
  import { ApiError, getLLM, getRouter, putRouter, unreachable } from '$lib/api';
  import { showShell } from '$lib/shell.svelte';
  import type { Family, RouteChoice } from '$lib/types';
  import Field from '$lib/ui/Field.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import SearchSelect, { type SearchOption } from '$lib/ui/SearchSelect.svelte';

  const names = ['plan', 'implement', 'implement_hard', 'review', 'triage'];
  let routes = $state<Record<string, RouteChoice>>({
    plan: { model: '', fallback: '' },
    implement: { model: '', fallback: '' },
    implement_hard: { model: '', fallback: '' },
    review: { model: 'different-family-from-author', fallback: '' },
    triage: { model: '', fallback: '' }
  });
  let families = $state<Family[]>([]);
  const modelOptions = $derived(
    families.flatMap((family) => family.models.map((model) => ({ value: model, label: model, detail: family.id })))
  );
  const reviewRule = 'different-family-from-author';

  function optionsFor(name: string, kind: 'model' | 'fallback'): SearchOption[] {
    const options = [...modelOptions];
    if (kind === 'model' && name === 'review') options.unshift({ value: reviewRule, label: reviewRule, detail: 'Rule' });
    const current = routes[name]?.[kind] ?? '';
    if (current !== '' && !options.some((option) => option.value === current)) {
      options.push({ value: current, label: current, detail: 'Not in LLM APIs' });
    }
    return options;
  }
  let maxAttempts = $state(3);
  let maxTurns = $state(12);
  let maxTokens = $state(200000);
  let maxMinutes = $state(30);
  let independent = $state(false);
  let message = $state('Loading router');
  let result = $state('');
  let busy = $state(false);

  showShell(() => ({ title: 'Router', message }));

  $effect(() => {
    void Promise.all([getRouter(), getLLM()])
      .then(([router, llm]) => {
        routes = router.routes;
        maxAttempts = router.budgets.maxAttempts || 3;
        maxTurns = router.budgets.maxModelTurnsPerAttempt || 12;
        maxTokens = router.budgets.maxTokensPerCard || 200000;
        maxMinutes = router.budgets.maxWallMinutesPerAttempt || 30;
        independent = router.reviewIndependentPossible;
        families = llm.families;
        message = 'Router';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Saving router';
    try {
      const saved = await putRouter({
        routes,
        budgets: {
          maxAttempts,
          maxModelTurnsPerAttempt: maxTurns,
          maxTokensPerCard: maxTokens,
          maxWallMinutesPerAttempt: maxMinutes
        }
      });
      independent = saved.reviewIndependentPossible;
      result = 'Applies to the next route decision.';
      message = 'Router';
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page title="Router" description="Which model handles each class of work, and how much each card may spend. Applies to every project." width="form">
  <form onsubmit={save}>
    <div class="flex flex-col gap-6">
      <Notice>
        {independent
          ? 'Review can use a second family.'
          : 'Only one family is configured, so review will record review.independent = false.'}
      </Notice>
      <Panel title="Routes" flush>
        <div
          class="hidden grid-cols-[9rem_1fr_1fr] gap-3 border-b border-line bg-ground/50 px-5 text-[11px] font-semibold uppercase tracking-wider text-muted sm:grid"
          aria-hidden="true"
        >
          <span class="flex h-10 items-center">Class</span>
          <span class="flex h-10 items-center">Model</span>
          <span class="flex h-10 items-center">Fallback</span>
        </div>
        <ul aria-label="Routes">
          {#each names as name (name)}
            <li
              class="grid gap-3 border-b border-line px-5 py-3 last:border-b-0 sm:grid-cols-[9rem_1fr_1fr] sm:items-center"
            >
              <span class="font-mono text-[13px] font-medium text-ink">{name}</span>
              <SearchSelect
                label="Model for {name}"
                id={`${name}-model`}
                width="full"
                hideLabel="sm"
                emptyLabel="None"
                placeholder="Search models"
                options={optionsFor(name, 'model')}
                bind:value={routes[name].model}
              />
              <SearchSelect
                label="Fallback for {name}"
                id={`${name}-fallback`}
                width="full"
                hideLabel="sm"
                emptyLabel="None"
                placeholder="Search models"
                options={optionsFor(name, 'fallback')}
                bind:value={routes[name].fallback}
              />
            </li>
          {/each}
        </ul>
      </Panel>
      <Panel title="Budgets" hint="Limits per card and per attempt.">
        <div class="grid gap-5 sm:grid-cols-2">
          <Field label="Max attempts" id="attempts" type="number" width="sm" bind:value={maxAttempts} />
          <Field label="Max model turns" id="turns" type="number" width="sm" hint="Per attempt" bind:value={maxTurns} />
          <Field label="Max tokens" id="tokens" type="number" width="sm" hint="Per card" bind:value={maxTokens} />
          <Field label="Max wall minutes" id="minutes" type="number" width="sm" hint="Per attempt" bind:value={maxMinutes} />
        </div>
      </Panel>
    </div>
    <SaveBar {busy} note={result} />
  </form>
</Page>
