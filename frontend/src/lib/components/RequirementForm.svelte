<script lang="ts">
  import Field from '$lib/ui/Field.svelte';

  let {
    id = $bindable(''),
    statement = $bindable(''),
    acceptance = $bindable(''),
    idLocked = false,
    fields = {}
  }: {
    id?: string;
    statement?: string;
    acceptance?: string;
    idLocked?: boolean;
    fields?: Record<string, string>;
  } = $props();
</script>

<Field
  label="Id"
  id="requirement-id"
  width="md"
  mono
  disabled={idLocked}
  placeholder={idLocked ? '' : 'REQ-012'}
  hint={idLocked ? 'Cards and pull requests refer to this id, so it cannot change.' : 'Leave blank to use the next free REQ- number.'}
  error={fields.id}
  bind:value={id}
/>
<Field label="Statement" id="requirement-statement" type="textarea" width="full" error={fields.statement} bind:value={statement} />
<Field
  label="Acceptance"
  id="requirement-acceptance"
  type="textarea"
  width="full"
  hint="One check per line. The reviewer reads them as a checklist."
  error={fields.acceptance}
  bind:value={acceptance}
/>
