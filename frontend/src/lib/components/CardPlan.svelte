<script lang="ts">
  import { tick } from "svelte";
  import {
    acceptCardPlan,
    skipCardPlan,
    streamPlanChat,
    ApiError,
  } from "$lib/api";
  import { failureText } from "$lib/forms";
  import { toast } from "$lib/toast.svelte";
  import type { Card, PlanStep } from "$lib/types";
  import Button from "$lib/ui/Button.svelte";
  import Icon from "$lib/ui/Icon.svelte";
  import { autogrow } from "$lib/ui/autogrow";
  import { controlBase, labelClass } from "$lib/ui/control";

  let { card = $bindable() }: { card: Card } = $props();

  const canAccept = $derived((card.actions ?? []).includes("accept_plan"));
  const canSkip = $derived((card.actions ?? []).includes("skip_plan"));

  const open = card.plan?.status === "open" ? card.plan : null;
  let title = $state(open?.title ?? "");
  let steps = $state<PlanStep[]>(
    (open?.steps ?? []).map((s) => ({ id: s.id, text: s.text, detail: s.detail ?? "" })),
  );
  let saving = $state(false);
  let fieldError = $state("");
  let chatMessage = $state("");
  let chatReply = $state("");
  let chatBusy = $state(false);
  let revision: {
    title: string;
    steps: { text: string; detail: string }[];
  } | null = $state(null);
  let abort: AbortController | null = null;
  let dragFrom = $state<number | null>(null);
  let dropOver = $state<number | null>(null);
  let editingId = $state<string | null>(null);
  let editingTitle = $state(false);
  let titleEditRoot = $state<HTMLElement | null>(null);

  const iconBtn =
    "inline-flex size-11 shrink-0 items-center justify-center rounded-control text-muted transition-colors duration-150 hover:bg-ground hover:text-ink disabled:pointer-events-none disabled:opacity-40";
  const hoverReveal =
    "opacity-0 transition-opacity duration-150 group-hover:opacity-100 focus-visible:opacity-100";

  function move(i: number, dir: -1 | 1) {
    const j = i + dir;
    if (j < 0 || j >= steps.length) return;
    const next = [...steps];
    [next[i], next[j]] = [next[j], next[i]];
    steps = next;
  }

  function reorder(from: number, to: number) {
    if (from === to || from < 0 || to < 0 || from >= steps.length || to >= steps.length) return;
    const next = [...steps];
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item);
    steps = next;
  }

  function onDragStart(i: number, event: DragEvent) {
    if (!canAccept || saving) {
      event.preventDefault();
      return;
    }
    dragFrom = i;
    dropOver = i;
    event.dataTransfer?.setData("text/plain", String(i));
    if (event.dataTransfer) event.dataTransfer.effectAllowed = "move";
  }

  function onDragOver(i: number, event: DragEvent) {
    if (dragFrom === null) return;
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
    dropOver = i;
  }

  function onDrop(i: number, event: DragEvent) {
    event.preventDefault();
    const from = dragFrom;
    dragFrom = null;
    dropOver = null;
    if (from === null) return;
    reorder(from, i);
  }

  function onDragEnd() {
    dragFrom = null;
    dropOver = null;
  }

  function onHandleKeydown(i: number, event: KeyboardEvent) {
    if (event.key === "ArrowUp") {
      event.preventDefault();
      move(i, -1);
    } else if (event.key === "ArrowDown") {
      event.preventDefault();
      move(i, 1);
    }
  }

  async function startEdit(id: string) {
    if (!canAccept || saving) return;
    editingTitle = false;
    editingId = id;
    await tick();
    const el = document.getElementById(`plan-step-${id}-text`) as HTMLTextAreaElement | null;
    el?.focus();
    el?.setSelectionRange(el.value.length, el.value.length);
  }

  function stopEdit() {
    editingId = null;
  }

  async function startTitleEdit() {
    if (!canAccept || saving) return;
    editingId = null;
    editingTitle = true;
    await tick();
    const el = document.getElementById("plan-title") as HTMLInputElement | null;
    el?.focus();
    el?.setSelectionRange(el.value.length, el.value.length);
  }

  function stopTitleEdit() {
    editingTitle = false;
  }

  function onStepKeydown(event: KeyboardEvent) {
    if (event.key === "Escape") {
      event.preventDefault();
      stopEdit();
    }
  }

  function onTitleKeydown(event: KeyboardEvent) {
    if (event.key === "Escape") {
      event.preventDefault();
      stopTitleEdit();
    }
  }

  $effect(() => {
    if (editingId === null && !editingTitle) return;
    function onPointer(event: PointerEvent) {
      const target = event.target as Node | null;
      if (!target) return;
      if (editingTitle) {
        if (titleEditRoot && !titleEditRoot.contains(target)) stopTitleEdit();
      }
      if (editingId !== null) {
        const row = document.getElementById(`plan-step-row-${editingId}`);
        if (row && !row.contains(target)) stopEdit();
      }
    }
    document.addEventListener("pointerdown", onPointer);
    return () => document.removeEventListener("pointerdown", onPointer);
  });

  async function addStep() {
    const id = `new_${steps.length + 1}`;
    steps = [...steps, { id, text: "", detail: "" }];
    await startEdit(id);
  }

  function removeStep(i: number) {
    if (steps.length <= 1) return;
    const id = steps[i]?.id;
    steps = steps.filter((_, idx) => idx !== i);
    if (editingId === id) editingId = null;
  }

  async function accept() {
    if (saving || !canAccept) return;
    fieldError = "";
    if (!title.trim() || steps.some((s) => !s.text.trim())) {
      fieldError = "Give the plan a title and text for every step.";
      return;
    }
    saving = true;
    editingId = null;
    editingTitle = false;
    try {
      card = await acceptCardPlan(card.id, {
        title: title.trim(),
        steps: steps.map((s) => ({
          id: s.id,
          text: s.text.trim(),
          detail: s.detail.trim(),
        })),
      });
      toast.success("Plan accepted. The next attempt is queued.");
    } catch (error) {
      if (error instanceof ApiError && error.fields?.title)
        fieldError = error.fields.title;
      else if (error instanceof ApiError && error.fields?.steps)
        fieldError = error.fields.steps;
      else toast.error(failureText(error));
    } finally {
      saving = false;
    }
  }

  async function skip() {
    if (saving || !canSkip) return;
    saving = true;
    try {
      card = await skipCardPlan(card.id);
      toast.success("Plan skipped. The next attempt is queued.");
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      saving = false;
    }
  }

  async function sendChat() {
    if (chatBusy || !canAccept) return;
    abort?.abort();
    abort = new AbortController();
    chatBusy = true;
    chatReply = "";
    revision = null;
    try {
      await streamPlanChat(
        card.id,
        {
          message: chatMessage.trim(),
          title: title.trim(),
          steps: steps.map((s) => ({
            id: s.id,
            text: s.text.trim(),
            detail: s.detail.trim(),
          })),
        },
        (event) => {
          if (event.kind === "delta") chatReply += event.text;
          if (event.kind === "done" && event.revision)
            revision = event.revision;
          if (event.kind === "error") toast.error(event.message);
        },
        abort.signal,
      );
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        toast.error(failureText(error));
      }
    } finally {
      chatBusy = false;
    }
  }

  function applyRevision() {
    if (!revision) return;
    title = revision.title;
    steps = revision.steps.map((s, i) => ({
      id: steps[i]?.id ?? `rev_${i + 1}`,
      text: s.text,
      detail: s.detail ?? "",
    }));
    revision = null;
    editingId = null;
    editingTitle = false;
    toast.success("Revised steps applied. Accept when ready.");
  }
</script>

<section id="plan" class="flex flex-col gap-4">
  <div>
    <p class="text-sm font-medium text-ink">The agent has a plan</p>
    <p class="mt-0.5 text-sm text-muted">
      Click a step to edit it, ask about the plan, then accept to queue the next attempt.
    </p>
  </div>

  <div class="flex flex-col gap-1" bind:this={titleEditRoot}>
    <span class={labelClass}>Title</span>
    {#if editingTitle && canAccept}
      <input
        id="plan-title"
        class="{controlBase} h-11 text-sm"
        bind:value={title}
        disabled={saving}
        aria-label="Plan title"
        onkeydown={onTitleKeydown}
      />
    {:else if canAccept}
      <button
        type="button"
        class="rounded-control px-2 py-1 -mx-2 text-left text-base font-medium text-ink transition-colors duration-150 hover:bg-ground/60 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ink/20 disabled:hover:bg-transparent"
        disabled={saving}
        aria-label="Edit title"
        onclick={() => void startTitleEdit()}
      >
        {title.trim() || "Untitled plan — click to edit"}
      </button>
    {:else}
      <p class="text-base font-medium text-ink">{title}</p>
    {/if}
  </div>

  <div class="flex flex-col gap-1.5">
    <span class={labelClass}>Steps</span>
    <ol class="flex flex-col divide-y divide-line/60">
      {#each steps as step, i (step.id)}
        {@const editing = editingId === step.id && canAccept}
        <li
          id="plan-step-row-{step.id}"
          class="group flex items-start gap-2 py-3 transition-colors duration-150 {dropOver ===
            i && dragFrom !== null && dragFrom !== i
            ? 'border-t-2 border-accent'
            : ''} {editing ? 'bg-ground/40 rounded-panel p-3 -mx-2' : ''} {dragFrom ===
          i
            ? 'opacity-50'
            : ''}"
          ondragover={(event) => onDragOver(i, event)}
          ondrop={(event) => onDrop(i, event)}
        >
          {#if canAccept}
            <button
              type="button"
              class="{iconBtn} {hoverReveal} cursor-grab active:cursor-grabbing"
              draggable="true"
              disabled={saving}
              aria-label="Drag to reorder"
              title="Drag to reorder, or use arrow keys"
              ondragstart={(event) => onDragStart(i, event)}
              ondragend={onDragEnd}
              onkeydown={(event) => onHandleKeydown(i, event)}
            >
              <Icon name="menu" class="size-4" />
            </button>
          {/if}
          <span
            class="w-6 shrink-0 pt-2.5 text-sm tabular-nums text-muted"
            aria-hidden="true">{i + 1}.</span
          >
          <div class="flex min-w-0 flex-1 flex-col gap-1.5">
            {#if editing}
              <label for="plan-step-{step.id}-text" class="sr-only">Step {i + 1}</label>
              <textarea
                id="plan-step-{step.id}-text"
                class="{controlBase} max-h-28 min-h-11 resize-none overflow-y-auto py-2 text-sm"
                rows="1"
                bind:value={step.text}
                disabled={saving}
                aria-label="Step {i + 1}"
                use:autogrow={{ on: true, value: step.text }}
                onkeydown={onStepKeydown}
              ></textarea>
              <label class="flex flex-col gap-1">
                <span class="text-xs text-muted"
                  >Detail <span class="font-normal">(optional)</span></span
                >
                <textarea
                  class="{controlBase} max-h-40 min-h-11 resize-none overflow-y-auto py-2 text-sm"
                  rows="2"
                  placeholder="How to verify, files to touch…"
                  bind:value={step.detail}
                  disabled={saving}
                  aria-label="Detail for step {i + 1}"
                  use:autogrow={{ on: true, value: step.detail }}
                  onkeydown={onStepKeydown}
                ></textarea>
              </label>
              <div class="flex justify-end">
                <Button variant="ghost" type="button" onclick={stopEdit}>Done</Button>
              </div>
            {:else}
              <button
                type="button"
                class="min-h-11 w-full rounded-control px-1 py-1.5 text-left transition-colors duration-150 hover:bg-ground/60 focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ink/20 disabled:hover:bg-transparent"
                disabled={!canAccept || saving}
                aria-label="Edit step {i + 1}"
                onclick={() => void startEdit(step.id)}
              >
                <p class="text-sm wrap-break-word text-ink">
                  {step.text.trim() || "Empty step — click to edit"}
                </p>
                {#if step.detail.trim()}
                  <p class="mt-1 text-xs wrap-break-word whitespace-pre-wrap text-muted">
                    {step.detail}
                  </p>
                {/if}
              </button>
            {/if}
          </div>
          {#if canAccept}
            <button
              type="button"
              class="{iconBtn} {hoverReveal}"
              disabled={steps.length <= 1 || saving}
              onclick={() => removeStep(i)}
              aria-label="Remove step"
            >
              <Icon name="trash" class="size-4" />
            </button>
          {/if}
        </li>
      {/each}
    </ol>
  </div>

  {#if fieldError}
    <p class="text-sm text-danger" role="alert">{fieldError}</p>
  {/if}

  {#if canAccept}
    <div class="flex flex-wrap items-center justify-end gap-2">
      <Button
        variant="ghost"
        icon="plus"
        onclick={() => void addStep()}
        disabled={saving || steps.length >= 30}>Add step</Button
      >
      {#if canSkip}
        <Button
          variant="secondary"
          onclick={() => void skip()}
          disabled={saving}>Skip plan</Button
        >
      {/if}
      <Button variant="primary" icon="check" onclick={() => void accept()} loading={saving}>
        Accept plan
      </Button>
    </div>
  {/if}

  {#if canAccept}
    <div class="flex flex-col gap-3 border-t border-line pt-4">
      <div>
        <h3 class="text-[13px] font-medium text-ink">Ask about this plan</h3>
        <p class="mt-0.5 text-sm text-muted">
          Suggest a change; apply the revision when it looks right.
        </p>
      </div>

      {#if chatReply}
        <div
          class="rounded-2xl rounded-tl-md bg-ground/70 px-3.5 py-2.5 text-sm whitespace-pre-wrap text-ink"
          aria-live="polite"
          aria-busy={chatBusy}
          role="region"
          aria-label="Plan chat reply"
        >
          {chatReply}{#if chatBusy}<span
              class="ml-0.5 inline-block h-4 w-1.5 translate-y-0.5 bg-ink motion-safe:animate-pulse"
              aria-hidden="true"
            ></span>{/if}
        </div>
      {/if}

      {#if revision}
        <Button variant="secondary" onclick={applyRevision}>Apply revision</Button>
      {/if}

      <div
        class="rounded-2xl border border-line bg-surface shadow-xs transition-colors duration-150 focus-within:border-muted"
      >
        <label for="plan-chat-message" class="sr-only">What should change?</label>
        <textarea
          id="plan-chat-message"
          class="block max-h-72 min-h-24 w-full resize-none overflow-y-auto rounded-t-2xl bg-transparent px-4 pt-3.5 pb-2 text-sm leading-6 text-ink placeholder:text-muted/70 focus-visible:outline-hidden"
          rows="3"
          bind:value={chatMessage}
          placeholder="What should change?"
          disabled={chatBusy}
          use:autogrow={{ on: true, value: chatMessage }}
        ></textarea>
        <div class="flex items-center justify-end gap-1 px-1.5 pb-1.5">
          {#if chatBusy}
            <button
              type="button"
              class="inline-flex size-11 shrink-0 items-center justify-center rounded-full bg-ink text-surface transition-colors duration-150 hover:bg-ink/85"
              aria-label="Stop"
              onclick={() => abort?.abort()}
            >
              <Icon name="stop" class="size-4" />
            </button>
          {:else}
            <button
              type="button"
              class="inline-flex size-11 shrink-0 items-center justify-center rounded-full bg-ink text-surface transition-colors duration-150 hover:bg-ink/85 disabled:opacity-40"
              aria-label="Send"
              disabled={!chatMessage.trim()}
              onclick={() => void sendChat()}
            >
              <Icon name="send" class="size-4" />
            </button>
          {/if}
        </div>
      </div>
    </div>
  {/if}
</section>
