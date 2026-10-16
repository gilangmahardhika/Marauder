<script lang="ts">
  import { page } from '$app/stores';
  import { getTribeSummary, getTribes } from '$lib/api';
  import SummaryDashboard from '$lib/components/SummaryDashboard.svelte';
  import Button from '$lib/ui/Button.svelte';

  let tribeName = $state('');

  const id = $derived($page.params.id ?? '');
  const load = $derived.by(() => (from: string, to: string) => getTribeSummary(id, from, to));
  const usageHref = $derived(id ? `/tribes/${id}/usage` : '');

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

<SummaryDashboard
  title={tribeName ? `Overview: ${tribeName}` : 'Tribe overview'}
  description="WIP across this tribe's projects, range insights, and MCP health. Days are in UTC."
  {load}
  {usageHref}
  showProjects
  showMcp
>
  {#snippet actions()}
    <Button href={`/tribes/${id}`} icon="arrow-left">Back to tribe</Button>
  {/snippet}
</SummaryDashboard>
