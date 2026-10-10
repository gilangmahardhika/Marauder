<script lang="ts">
  let { message = '' }: { message?: string } = $props();
</script>

<p role="status">{message}</p>
