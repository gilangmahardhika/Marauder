<script lang="ts">
  import type { Snippet } from "svelte";
  import Button from "./Button.svelte";
  import Icon from "./Icon.svelte";
  import Spinner from "./Spinner.svelte";

  let {
    title,
    description = "",
    meta,
    summary,
    actions,
    loading = "",
    error = "",
    onretry,
    children,
  }: {
    title: string;
    description?: string;
    meta?: Snippet;
    /** A line under the title, after `description`. */
    summary?: Snippet;
    actions?: Snippet;
    /** While set, such as "Loading board", a spinner row replaces the body. */
    loading?: string;
    /** A load failure; replaces the body with the message and "Try again" when `onretry` is set. */
    error?: string;
    onretry?: () => void;
    children: Snippet;
  } = $props();
</script>

<div class="w-full min-w-0">
  <div
    class="mb-8 flex flex-col gap-4 border-b border-line pb-6 sm:flex-row sm:items-end sm:justify-between sm:gap-8"
  >
    <div class="min-w-0 flex-1">
      <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
        <h1
          class="wrap-break-word text-2xl font-semibold tracking-tight text-ink"
        >
          {title}
        </h1>
        {#if meta}{@render meta()}{/if}
      </div>
      {#if description}<p class="mt-1.5 max-w-2xl text-sm text-muted">
          {description}
        </p>{/if}
      {#if summary}<div class="mt-2">{@render summary()}</div>{/if}
    </div>
    {#if actions}<div class="flex shrink-0 flex-wrap items-center gap-2">
        {@render actions()}
      </div>{/if}
  </div>
  {#if error}
    <div
      class="flex max-w-2xl items-start gap-3 rounded-panel border border-red-200 bg-red-50 p-4"
      role="alert"
    >
      <Icon name="alert" class="mt-0.5 size-4 shrink-0 text-danger" />
      <div class="min-w-0 flex-1">
        <p class="wrap-break-word text-sm text-red-900">{error}</p>
        {#if onretry}
          <div class="mt-3">
            <Button icon="refresh" onclick={onretry}>Try again</Button>
          </div>
        {/if}
      </div>
    </div>
  {:else if loading}
    <p class="flex items-center gap-2 py-6 text-sm text-muted" role="status">
      <Spinner />
      {loading}
    </p>
  {:else}
    {@render children()}
  {/if}
</div>
