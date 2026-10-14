<script lang="ts">
  import Icon from './Icon.svelte';

  let {
    message = '',
    extra = [],
    title = ''
  }: {
    message?: string;
    /** Field errors no input on the form shows. */
    extra?: string[];
    /** A bold first line, such as "Not saved". */
    title?: string;
  } = $props();

  const lines = $derived(extra.filter((line) => line !== message));
</script>

{#if message || lines.length > 0}
  <div
    class="flex items-start gap-3 rounded-control border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-900 focus-visible:outline-danger"
    role="alert"
    tabindex="-1"
  >
    <Icon name="alert" class="mt-0.5 size-4 shrink-0 text-danger" />
    <div class="min-w-0 flex-1">
      {#if title}<p class="font-medium">{title}</p>{/if}
      {#if message}<p class="wrap-break-word">{message}</p>{/if}
      {#if lines.length > 0}
        <ul class="mt-1 list-disc pl-4">
          {#each lines as line, index (index)}<li class="wrap-break-word">{line}</li>{/each}
        </ul>
      {/if}
    </div>
  </div>
{/if}
