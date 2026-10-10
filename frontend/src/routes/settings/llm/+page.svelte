<script lang="ts">
  import { ApiError, getLLM, putLLM, testLLM, unreachable } from '$lib/api';
  import Live from '$lib/components/Live.svelte';
  import SecretField, { omitEmptySecret } from '$lib/components/SecretField.svelte';

  type Draft = {
    id: string;
    protocol: string;
    baseUrl: string;
    models: string[];
    key: string;
    clear: boolean;
    keySet: boolean;
  };

  let families = $state<Draft[]>([]);
  let message = $state('Loading LLM settings');
  let busy = $state(false);

  $effect(() => {
    void getLLM()
      .then((body) => {
        families = body.families.map((family) => ({
          id: family.id,
          protocol: family.protocol || 'anthropic',
          baseUrl: family.baseUrl,
          models: family.models.length ? family.models : [''],
          key: '',
          clear: false,
          keySet: family.keySet
        }));
        message = 'LLM APIs';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  function addFamily() {
    families = [...families, { id: '', protocol: 'anthropic', baseUrl: '', models: [''], key: '', clear: false, keySet: false }];
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Saving LLM settings';
    try {
      const saved = await putLLM({
        families: families.map((family) => ({
          id: family.id,
          protocol: family.protocol,
          baseUrl: family.baseUrl,
          models: family.models.map((model) => model.trim()).filter(Boolean),
          ...omitEmptySecret('apiKey', family.key, family.clear)
        }))
      });
      families = saved.families.map((family) => ({
        id: family.id,
        protocol: family.protocol,
        baseUrl: family.baseUrl,
        models: family.models.length ? family.models : [''],
        key: '',
        clear: false,
        keySet: family.keySet
      }));
      message = 'LLM APIs';
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function test(id: string) {
    busy = true;
    try {
      const result = await testLLM(id);
      message = result.ok ? 'ok' : (result.message ?? 'failed');
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<h1>LLM APIs</h1>
<Live {message} />
<form class="form" onsubmit={save}>
  {#each families as family, index}
    <fieldset>
      <label for={`id-${index}`}>Id <input id={`id-${index}`} bind:value={family.id} /></label>
      <label for={`protocol-${index}`}>
        Protocol
        <select id={`protocol-${index}`} bind:value={family.protocol}>
          <option value="anthropic">anthropic</option>
          <option value="openai">openai</option>
        </select>
      </label>
      <label for={`url-${index}`}>Base URL <input id={`url-${index}`} bind:value={family.baseUrl} /></label>
      {#each family.models as model, modelIndex}
        <label for={`model-${index}-${modelIndex}`}>
          Model
          <input id={`model-${index}-${modelIndex}`} bind:value={family.models[modelIndex]} />
        </label>
        <button type="button" onclick={() => (family.models = family.models.filter((_, item) => item !== modelIndex))}>Remove</button>
      {/each}
      <button type="button" onclick={() => (family.models = [...family.models, ''])}>Add model</button>
      <SecretField label="API key" name={`key-${index}`} saved={family.keySet} bind:value={family.key} bind:clear={family.clear} />
      <button type="button" disabled={busy || family.id === ''} onclick={() => void test(family.id)}>Test</button>
    </fieldset>
  {/each}
  <button type="button" onclick={addFamily}>Add family</button>
  <button class="primary" type="submit" disabled={busy}>Save</button>
</form>
