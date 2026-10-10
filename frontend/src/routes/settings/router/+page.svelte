<script lang="ts">
  import { ApiError, getLLM, getRouter, putRouter, unreachable } from '$lib/api';
  import Live from '$lib/components/Live.svelte';
  import type { RouteChoice } from '$lib/types';

  const names = ['plan', 'implement', 'implement_hard', 'review', 'triage'];
  let routes = $state<Record<string, RouteChoice>>({
    plan: { model: '', fallback: '' },
    implement: { model: '', fallback: '' },
    implement_hard: { model: '', fallback: '' },
    review: { model: 'different-family-from-author', fallback: '' },
    triage: { model: '', fallback: '' }
  });
  let models = $state<string[]>([]);
  let maxAttempts = $state(3);
  let maxTurns = $state(12);
  let maxTokens = $state(200000);
  let maxMinutes = $state(30);
  let independent = $state(false);
  let message = $state('Loading router');
  let result = $state('');
  let busy = $state(false);

  $effect(() => {
    void Promise.all([getRouter(), getLLM()])
      .then(([router, llm]) => {
        routes = router.routes;
        maxAttempts = router.budgets.maxAttempts || 3;
        maxTurns = router.budgets.maxModelTurnsPerAttempt || 12;
        maxTokens = router.budgets.maxTokensPerCard || 200000;
        maxMinutes = router.budgets.maxWallMinutesPerAttempt || 30;
        independent = router.reviewIndependentPossible;
        models = llm.families.flatMap((family) => family.models);
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

<h1>Router</h1>
<Live {message} />
<p>
  {independent
    ? 'Review can use a second family.'
    : 'Only one family is configured, so review will record review.independent = false.'}
</p>
<form class="form" onsubmit={save}>
  <table>
    <thead>
      <tr><th>Class</th><th>Model</th><th>Fallback</th></tr>
    </thead>
    <tbody>
      {#each names as name}
        <tr>
          <th scope="row">{name}</th>
          <td>
            <label for={`${name}-model`}>
              Model
              <select id={`${name}-model`} bind:value={routes[name].model}>
                <option value=""></option>
                {#if name === 'review'}<option value="different-family-from-author">different-family-from-author</option>{/if}
                {#each models as model}<option value={model}>{model}</option>{/each}
              </select>
            </label>
          </td>
          <td>
            <label for={`${name}-fallback`}>
              Fallback
              <select id={`${name}-fallback`} bind:value={routes[name].fallback}>
                <option value=""></option>
                {#each models as model}<option value={model}>{model}</option>{/each}
              </select>
            </label>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
  <label for="attempts">Max attempts <input id="attempts" type="number" bind:value={maxAttempts} /></label>
  <label for="turns">Max model turns <input id="turns" type="number" bind:value={maxTurns} /></label>
  <label for="tokens">Max tokens <input id="tokens" type="number" bind:value={maxTokens} /></label>
  <label for="minutes">Max wall minutes <input id="minutes" type="number" bind:value={maxMinutes} /></label>
  <button class="primary" type="submit" disabled={busy}>Save</button>
</form>
{#if result}<p>{result}</p>{/if}
