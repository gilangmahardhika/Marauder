<script lang="ts" module>
  export function omitEmptySecret(name: string, value: string, clear: boolean): Record<string, string | boolean> {
    if (clear) return { clear: true };
    if (value === '') return {};
    return { [name]: value };
  }
</script>

<script lang="ts">
  let {
    label,
    name,
    saved = false,
    value = $bindable(''),
    clear = $bindable(false)
  }: {
    label: string;
    name: string;
    saved?: boolean;
    value?: string;
    clear?: boolean;
  } = $props();
  const inputId = $derived(`secret-${name}`);
</script>

<label for={inputId}>
  {label}
  <input id={inputId} name={name} type="password" autocomplete="off" placeholder={saved ? 'A value is saved' : ''} bind:value />
</label>
<button
  type="button"
  onclick={() => {
    clear = true;
    value = '';
  }}>Remove</button
>
