<script lang="ts">
  import { page } from '$app/stores';
  import { getTribeUsage, getTribes } from '$lib/api';
  import UsageDashboard from '$lib/components/UsageDashboard.svelte';
  import Button from '$lib/ui/Button.svelte';

  let tribeName = $state('');

  const id = $derived($page.params.id ?? '');
  const load = $derived((from: string, to: string) => getTribeUsage(id, from, to));

  $effect(() => {
    const current = id;
    void getTribes()
      .then((body) => {
        if (current === id) tribeName = body.tribes.find((tribe) => tribe.id === current)?.name ?? '';
      })
      .catch(() => {
        tribeName = '';
      });
  });
</script>

<UsageDashboard
  title={tribeName ? `Token usage: ${tribeName}` : 'Token usage'}
  description="Tokens used by model calls in this tribe's projects. Days are in UTC. Total is input plus output; cached input is counted apart."
  {load}
>
  {#snippet actions()}
    <Button href={`/tribes/${id}`} icon="arrow-left">Back to tribe</Button>
  {/snippet}
</UsageDashboard>
