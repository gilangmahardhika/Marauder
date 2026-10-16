<script lang="ts">
  import { getProjectSummary, getProjects } from '$lib/api';
  import SummaryDashboard from '$lib/components/SummaryDashboard.svelte';
  import { canManageTribe, shell } from '$lib/shell.svelte';
  import { session } from '$lib/session';
  import Button from '$lib/ui/Button.svelte';

  let projectName = $state('');
  let tribeId = $state('');

  const load = $derived.by(() => (from: string, to: string) => getProjectSummary(from, to));
  const usageHref = $derived(
    tribeId && canManageTribe(tribeId) ? `/tribes/${tribeId}/usage?project=${session.project()}` : ''
  );

  $effect(() => {
    const id = shell.projectId || session.project();
    if (!id) {
      projectName = '';
      tribeId = '';
      return;
    }
    void getProjects()
      .then((body) => {
        const project = body.projects.find((item) => item.id === id);
        projectName = project?.name ?? '';
        tribeId = project?.tribeId ?? '';
      })
      .catch(() => {
        projectName = '';
        tribeId = '';
      });
  });
</script>

<SummaryDashboard
  title={projectName ? `Overview: ${projectName}` : 'Overview'}
  description="Board WIP, attention queues, and range insights for this project. Days are in UTC."
  {load}
  {usageHref}
>
  {#snippet actions()}
    <Button href="/" icon="board">Board</Button>
  {/snippet}
</SummaryDashboard>
