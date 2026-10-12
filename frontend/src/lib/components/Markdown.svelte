<script lang="ts">
  import { cleanMarkdown, parse, parseInline, type Inline, type List, type MdBlock } from '$lib/markdown';
  import { count } from '$lib/format';

  let {
    text,
    inline = false,
    headingBase = 3,
    cap = 20_000
  }: { text: string; inline?: boolean; headingBase?: number; cap?: number } = $props();

  const doc = $derived(inline ? null : parse(text, cap));
  const spans = $derived(inline ? parseInline(cleanMarkdown(text, cap).text) : []);

  const tag = (level: number) => `h${Math.min(6, headingBase + level - 1)}`;
  const size = (level: number) => (level === 1 ? 'text-base font-semibold' : level === 2 ? 'text-sm font-semibold' : 'text-sm font-medium');
</script>

<!-- prettier-ignore -->
{#snippet inlines(nodes: Inline[])}{#each nodes as node, index (index)}{#if node.type === 'text'}{node.text}{:else if node.type === 'code'}<code class="rounded bg-ground px-1 font-mono text-[13px]">{node.text}</code>{:else if node.type === 'strong'}<strong class="font-semibold">{@render inlines(node.children)}</strong>{:else if node.type === 'em'}<em>{@render inlines(node.children)}</em>{:else if node.type === 'link'}<a class="break-words text-accent underline-offset-2 hover:underline" href={node.href} target="_blank" rel="noreferrer noopener">{@render inlines(node.children)}</a>{:else}<br />{/if}{/each}{/snippet}

{#snippet list(node: List)}
  <svelte:element
    this={node.ordered ? 'ol' : 'ul'}
    start={node.ordered && node.start !== 1 ? node.start : undefined}
    class="flex flex-col gap-1 pl-5 {node.ordered ? 'list-decimal' : 'list-disc'}"
  >
    {#each node.items as item, index (index)}
      <li>{@render inlines(item.children)}{#each item.sublists as sublist, subIndex (subIndex)}{@render list(sublist)}{/each}</li>
    {/each}
  </svelte:element>
{/snippet}

{#snippet blocks(nodes: MdBlock[])}
  {#each nodes as node, index (index)}
    {#if node.type === 'paragraph'}
      <p>{@render inlines(node.children)}</p>
    {:else if node.type === 'heading'}
      <svelte:element this={tag(node.level)} class="{size(node.level)} text-ink">{@render inlines(node.children)}</svelte:element>
    {:else if node.type === 'list'}
      {@render list(node)}
    {:else if node.type === 'code'}
      <pre
        class="overflow-x-auto rounded-control border border-line bg-ground/60 px-3 py-2 font-mono text-xs leading-relaxed text-ink"><code>{node.text}</code></pre>
    {:else if node.type === 'quote'}
      <blockquote class="flex flex-col gap-2 border-l-2 border-line-strong pl-3 text-muted">{@render blocks(node.children)}</blockquote>
    {:else}
      <hr class="border-line" />
    {/if}
  {/each}
{/snippet}

{#if doc}
  <div class="flex min-w-0 flex-col gap-2 text-sm leading-relaxed break-words text-ink">
    {@render blocks(doc.blocks)}
    {#if doc.cut}<p class="text-xs text-muted">Showing the first {count(cap)} characters.</p>{/if}
  </div>
{:else}
  <span class="break-words">{@render inlines(spans)}</span>
{/if}
