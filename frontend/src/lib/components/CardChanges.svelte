<script lang="ts">
  import { tick } from "svelte";
  import {
    changedLines,
    groupByFolder,
    hunkLabel,
    numberLines,
    parseHunkHeader,
    splitPath,
  } from "$lib/diff";
  import { pageSize } from "$lib/filter";
  import type { CardDiff, DiffFile, DiffLine } from "$lib/types";
  import Badge from "$lib/ui/Badge.svelte";
  import Button from "$lib/ui/Button.svelte";
  import Checkbox from "$lib/ui/Checkbox.svelte";
  import Icon from "$lib/ui/Icon.svelte";
  import Notice from "$lib/ui/Notice.svelte";
  import Panel from "$lib/ui/Panel.svelte";
  import ShowMore from "$lib/ui/ShowMore.svelte";

  let { diff, error = "" }: { diff: CardDiff | null; error?: string } =
    $props();

  /** Files open on load only while the whole diff stays this small. */
  const openFiles = 10;
  const openLines = 400;

  let open = $state<Record<string, boolean>>({});
  let limit = $state(50);
  let wrap = $state(false);

  const files = $derived(diff?.files ?? []);
  const added = $derived(
    files.reduce((sum, file) => sum + Math.max(0, file.added), 0),
  );
  const deleted = $derived(
    files.reduce((sum, file) => sum + Math.max(0, file.deleted), 0),
  );
  const short = $derived(diff?.sha ? diff.sha.slice(0, 7) : "");
  const groups = $derived(groupByFolder(files));
  const openByDefault = $derived(
    files.length <= openFiles &&
      files.reduce((sum, file) => sum + changedLines(file), 0) <= openLines,
  );

  /** Open state is keyed by commit, so a new head starts from the default again. */
  function key(file: DiffFile) {
    return `${diff?.sha ?? ""}:${file.path}`;
  }

  function isOpen(file: DiffFile) {
    return open[key(file)] ?? openByDefault;
  }

  function toggle(file: DiffFile) {
    open = { ...open, [key(file)]: !isOpen(file) };
  }

  function setAll(value: boolean) {
    open = Object.fromEntries(files.map((file) => [key(file), value]));
  }

  function fileId(index: number) {
    return `diff-file-${index}`;
  }

  async function jump(event: MouseEvent, file: DiffFile, index: number) {
    event.preventDefault();
    limit = Math.max(limit, index + 1);
    open = { ...open, [key(file)]: true };
    await tick();
    const target = document.getElementById(fileId(index));
    target?.scrollIntoView?.({ block: "start" });
    target
      ?.querySelector<HTMLButtonElement>("button")
      ?.focus({ preventScroll: true });
  }

  const rowClass: Record<DiffLine["kind"], string> = {
    add: "bg-green-50",
    del: "bg-red-50",
    ctx: "",
    meta: "",
  };
  const gutterClass: Record<DiffLine["kind"], string> = {
    add: "bg-green-100 text-green-900",
    del: "bg-red-100 text-red-900",
    ctx: "bg-ground/60 text-muted",
    meta: "bg-ground/60 text-muted",
  };
  const codeClass: Record<DiffLine["kind"], string> = {
    add: "text-green-950",
    del: "text-red-950",
    ctx: "text-ink",
    meta: "italic text-muted",
  };
  const sign: Record<DiffLine["kind"], string> = {
    add: "+",
    del: "-",
    ctx: "",
    meta: "",
  };
  const spoken: Record<DiffLine["kind"], string> = {
    add: "Added line",
    del: "Removed line",
    ctx: "",
    meta: "",
  };
</script>

{#snippet counts(file: DiffFile)}
  {#if file.binary}
    <span class="text-muted">binary</span>
  {:else}
    <span class="text-green-800">+{file.added}</span>
    <span class="text-red-800">-{file.deleted}</span>
  {/if}
{/snippet}

<Panel
  title="Changes"
  hint={short
    ? `Commit ${short} against ${diff?.base}.`
    : "The code this card changes."}
  flush
>
  {#snippet actions()}
    {#if files.length > 0}
      <Badge>{files.length} {files.length === 1 ? "file" : "files"}</Badge>
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
      <p class="text-sm text-muted">
        No commit yet. Changes show here after the implementer commits.
      </p>
    {:else if diff.missing}
      <p class="text-sm text-muted">
        Commit {short} is not in the project clone yet.
      </p>
    {:else if files.length === 0}
      <p class="text-sm text-muted">
        This commit changes no files against {diff.base}.
      </p>
    {:else}
      {#if diff.truncated}
        <Notice tone="warning"
          >The diff is large, so it is cut. The file list is complete. Open it
          on GitHub for the rest.</Notice
        >
      {/if}
      <div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <div class="flex flex-wrap items-center gap-2">
          <Button onclick={() => setAll(true)}>Expand all</Button>
          <Button variant="ghost" onclick={() => setAll(false)}
            >Collapse all</Button
          >
        </div>
        <Checkbox id="diff-wrap" label="Wrap lines" bind:checked={wrap} />
      </div>
      {#if files.length > 1}
        <nav
          aria-label="Changed files"
          class="max-h-72 overflow-y-auto rounded-control border border-line bg-ground/40 px-3 py-2"
        >
          {#each groups as group (group.folder)}
            <div class="py-1">
              <p
                class="truncate font-mono text-xs text-muted"
                title={group.folder || "Repository root"}
              >
                {group.folder ? `${group.folder}/` : "Repository root"}
              </p>
              <ul class="mt-0.5 flex flex-col">
                {#each group.files as entry (entry.file.path)}
                  <li>
                    <a
                      href={`#${fileId(entry.index)}`}
                      class="flex min-h-11 items-center gap-3 rounded-control px-2 text-sm hover:bg-surface"
                      onclick={(event) => jump(event, entry.file, entry.index)}
                    >
                      <span
                        class="min-w-0 flex-1 truncate font-mono text-[13px] text-ink"
                        >{splitPath(entry.file.path).name}</span
                      >
                      {#if entry.file.truncated}<Badge tone="amber">cut</Badge
                        >{/if}
                      <span class="flex shrink-0 gap-2 font-mono text-xs"
                        >{@render counts(entry.file)}</span
                      >
                    </a>
                  </li>
                {/each}
              </ul>
            </div>
          {/each}
        </nav>
      {/if}
      <ul class="flex flex-col gap-3">
        {#each files.slice(0, limit) as file, index (file.path)}
          {@const parts = splitPath(file.path)}
          {@const expanded = isOpen(file)}
          <li
            id={fileId(index)}
            class="scroll-mt-14 rounded-control border border-line"
          >
            <button
              type="button"
              class="sticky top-12 z-5 flex min-h-11 w-full items-center gap-3 border-line bg-surface px-3 text-left hover:bg-ground {expanded
                ? 'rounded-t-control border-b'
                : 'rounded-control'}"
              aria-expanded={expanded}
              aria-controls={`${fileId(index)}-body`}
              onclick={() => toggle(file)}
            >
              <Icon
                name="chevron"
                class="size-4 shrink-0 text-muted transition-transform {expanded
                  ? 'rotate-180'
                  : ''}"
              />
              <span
                class="min-w-0 flex-1 truncate font-mono text-[13px]"
                title={file.path}
              >
                {#if parts.folder}<span class="text-muted">{parts.folder}/</span
                  >{/if}<span class="font-semibold text-ink">{parts.name}</span>
              </span>
              {#if file.truncated}<Badge tone="amber">cut</Badge>{/if}
              <span class="flex shrink-0 gap-2 font-mono text-xs"
                >{@render counts(file)}</span
              >
            </button>
            {#if expanded}
              <div id={`${fileId(index)}-body`}>
                {#if file.binary}
                  <p class="px-4 py-3 text-sm text-muted">
                    Binary file. Its contents are not shown.
                  </p>
                {:else if file.hunks.length === 0}
                  <p class="px-4 py-3 text-sm text-muted">
                    {file.truncated
                      ? "This file is past the cut. Open it on GitHub."
                      : "No text changes."}
                  </p>
                {:else}
                  {#each file.hunks as hunk, h (h)}
                    {@const context =
                      parseHunkHeader(hunk.header)?.context ?? ""}
                    <div
                      class="flex flex-wrap items-baseline gap-x-3 border-line bg-sky-50 px-4 py-1.5 text-xs text-sky-950 {h >
                      0
                        ? 'border-t'
                        : ''}"
                      title={hunk.header}
                    >
                      <span class="font-medium">{hunkLabel(hunk.header)}</span>
                      {#if context}<span class="min-w-0 truncate font-mono"
                          >{context}</span
                        >{/if}
                    </div>
                    <div class="overflow-x-auto">
                      <div
                        class="font-mono text-xs leading-5 {wrap
                          ? 'w-full'
                          : 'w-max min-w-full'}"
                      >
                        {#each numberLines(hunk) as line, l (l)}
                          <div
                            class="grid grid-cols-[3.25rem_3.25rem_1.5rem_minmax(0,1fr)] {rowClass[
                              line.kind
                            ]}"
                          >
                            <span
                              class="select-none px-2 text-right {gutterClass[
                                line.kind
                              ]}"
                              aria-hidden="true">{line.old ?? ""}</span
                            >
                            <span
                              class="select-none px-2 text-right {gutterClass[
                                line.kind
                              ]}"
                              aria-hidden="true">{line.new ?? ""}</span
                            >
                            <span
                              class="select-none text-center font-semibold {codeClass[
                                line.kind
                              ]}"
                            >
                              <span aria-hidden="true">{sign[line.kind]}</span>
                              {#if spoken[line.kind]}<span class="sr-only"
                                  >{spoken[line.kind]}</span
                                >{/if}
                            </span>
                            <span
                              class="pr-4 {wrap
                                ? 'whitespace-pre-wrap break-all'
                                : 'whitespace-pre'} {codeClass[line.kind]}"
                              >{line.text}</span
                            >
                          </div>
                        {/each}
                      </div>
                    </div>
                  {/each}
                  {#if file.truncated}
                    <p
                      class="border-t border-line px-4 py-2 text-xs text-muted"
                    >
                      This file is cut. Open it on GitHub for the rest.
                    </p>
                  {/if}
                {/if}
              </div>
            {/if}
          </li>
        {/each}
      </ul>
      {#if files.length > 50}
        <ShowMore
          total={files.length}
          {limit}
          noun={["file", "files"]}
          onmore={() => (limit += pageSize)}
        />
      {/if}
    {/if}
    {#if diff?.compareUrl}
      <a
        class="inline-flex min-h-11 w-fit items-center text-sm font-medium text-accent hover:underline"
        href={diff.compareUrl}>View these changes on GitHub</a
      >
    {/if}
  </div>
</Panel>
