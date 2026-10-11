<script lang="ts">
  import type { Option } from '$lib/filter';
  import { control } from './control';

  let {
    label,
    options,
    value,
    onchange
  }: { label: string; options: Option[]; value: string; onchange: (value: string) => void } = $props();
</script>

<label class="w-44">
  <span class="sr-only">{label}</span>
  <select class={control} {value} onchange={(event) => onchange(event.currentTarget.value)}>
    {#each options as option (option.value)}
      <option value={option.value}>{option.label}</option>
    {/each}
  </select>
</label>
