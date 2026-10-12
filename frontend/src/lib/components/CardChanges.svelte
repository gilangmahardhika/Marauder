<script lang="ts">
  import { pageSize } from '$lib/filter';
  import type { CardDiff, DiffFile, DiffLine } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import ShowMore from '$lib/ui/ShowMore.svelte';

  let { diff, error = '' }: { diff: CardDiff | null; error?: string } = $props();

  let open = $state<Record<string, boolean>>({});
  let limit = $state(50);

  const files = $derived(diff?.files ?? []);
  const added = $derived(files.reduce((sum, file) => sum + file.added, 0));
  const deleted = $derived(files.reduce((sum, file) => sum + file.deleted, 0));
  const short = $derived(diff?.sha ? diff.sha.slice(0, 7) : '');

  function toggle(path: string) {
    open = { ...open, [path]: !open[path] };
  }

  function rowId(index: number) {
    return `diff-file-${index}`;
  }

  const lineClass: Record<DiffLine['kind'], string> = {
    add: 'bg-green-50 text-green-950',
    del: 'bg-red-50 text-red-950',
    ctx: 'text-ink',
    meta: 'text-muted italic'
  };
  const sign: Record<DiffLine['kind'], string> = { add: '+', del: '-', ctx: ' ', meta: ' ' };

  function counts(file: DiffFile) {
    return file.binary ? 'binary' : `+${file.added} -${file.deleted}`;
  }
</script>

<Panel title="Changes" hint={short ? `Commit ${short} against ${diff?.base}.` : 'The code this card changes.'} flush>
  {#snippet actions()}
    {#if files.length > 0}
      <Badge>{files.length} {files.length === 1 ? 'file' : 'files'}</Badge>
      <Badge tone="green">+{added}</Badge>
      <Badge tone="red">-{deleted}</Badge>
    {/if}
  {/snippet}
  <div class="flex flex-col gap-4 p-5">
    {#if error}
      <Notice tone="danger">{error}</Notice>
    {:else if diff === null}
      <p class="text-sm text-muted">Loading changes.</p>
    {:else if !diff.sha}
      <p class="text-sm text-muted">No commit yet. Changes show here after the implementer commits.</p>
    {:else if diff.missing}
      <p class="text-sm text-muted">Commit {short} is not in the project clone yet.</p>
    {:else if files.length === 0}
      <p class="text-sm text-muted">This commit changes no files against {diff.base}.</p>
    {:else}
      {#if diff.truncated}
        <Notice tone="warning">The diff is large, so it is cut. The file list is complete. Open it on GitHub for the rest.</Notice>
      {/if}
      <ul class="flex flex-col divide-y divide-line rounded-control border border-line">
        {#each files.slice(0, limit) as file, index (file.path)}
          <li>
            <button
              type="button"
              class="flex min-h-11 w-full items-center gap-3 px-3 text-left hover:bg-ground"
              aria-expanded={!!open[file.path]}
              aria-controls={rowId(index)}
              onclick={() => toggle(file.path)}
            >
              <Icon name="chevron" class="size-4 shrink-0 text-muted transition-transform {open[file.path] ? 'rotate-180' : ''}" />
              <span class="min-w-0 flex-1 truncate font-mono text-[13px] text-ink">{file.path}</span>
              <span class="shrink-0 font-mono text-xs text-muted">{counts(file)}</span>
            </button>
            {#if open[file.path]}
              <div id={rowId(index)} class="border-t border-line">
                {#if file.binary}
                  <p class="px-4 py-3 text-sm text-muted">Binary file. Its contents are not shown.</p>
                {:else if file.hunks.length === 0}
                  <p class="px-4 py-3 text-sm text-muted">
                    {file.truncated ? 'This file is past the cut. Open it on GitHub.' : 'No text changes.'}
                  </p>
                {:else}
                  <div class="overflow-x-auto">
                    {#each file.hunks as hunk, h (h)}
                      <pre class="bg-ground px-4 py-1 font-mono text-xs text-muted">{hunk.header}</pre>
                      <pre class="font-mono text-xs leading-5">{#each hunk.lines as line, l (l)}<span class="block px-4 {lineClass[line.kind]}">{sign[line.kind]}{line.text}</span>{/each}</pre>
                    {/each}
                  </div>
                  {#if file.truncated}
                    <p class="border-t border-line px-4 py-2 text-xs text-muted">This file is cut. Open it on GitHub for the rest.</p>
                  {/if}
                {/if}
              </div>
            {/if}
          </li>
        {/each}
      </ul>
      {#if files.length > 50}
        <ShowMore total={files.length} {limit} noun={['file', 'files']} onmore={() => (limit += pageSize)} />
      {/if}
    {/if}
    {#if diff?.compareUrl}
      <a class="inline-flex min-h-11 w-fit items-center text-sm font-medium text-accent hover:underline" href={diff.compareUrl}
        >View these changes on GitHub</a
      >
    {/if}
  </div>
</Panel>
