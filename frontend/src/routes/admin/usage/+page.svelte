<script lang="ts">
  import { getUsage } from '$lib/api';
  import UsageDashboard from '$lib/components/UsageDashboard.svelte';
</script>

<UsageDashboard
  title="Token usage"
  description="Tokens used by model calls in every project. Days are in UTC. Total is input plus output; cached input is counted apart."
  load={getUsage}
/>
