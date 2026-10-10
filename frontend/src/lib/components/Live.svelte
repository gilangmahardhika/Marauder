<script lang="ts">
  let { message = '', class: extra = '' }: { message?: string; class?: string } = $props();
</script>

<p class="text-muted {extra || 'text-sm'}" role="status">{message}</p>
