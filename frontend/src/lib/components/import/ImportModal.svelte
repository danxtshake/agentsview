<script lang="ts">
  import { importClaudeAI, importChatGPT } from "../../api/client.js";

  interface Props {
    open: boolean;
    onclose: () => void;
    onimported: () => void;
  }

  let { open = $bindable(), onclose, onimported }: Props = $props();

  let fileInput: HTMLInputElement | undefined = $state();
  let selectedFile: File | null = $state(null);
  let provider: "claude-ai" | "chatgpt" = $state("claude-ai");
  let importing = $state(false);
  let result: {
    imported: number;
    updated: number;
    skipped?: number;
    errors: number;
  } | null = $state(null);
  let error: string | null = $state(null);

  // Reset file selection when provider changes.
  $effect(() => {
    // eslint-disable-next-line @typescript-eslint/no-unused-expressions
    provider;
    selectedFile = null;
    result = null;
    error = null;
    if (fileInput) fileInput.value = "";
  });

  function handleFileChange(e: Event) {
    const input = e.target as HTMLInputElement;
    selectedFile = input.files?.[0] ?? null;
    result = null;
    error = null;
  }

  async function handleImport() {
    if (importing || !selectedFile) return;
    importing = true;
    error = null;
    result = null;

    try {
      if (provider === "chatgpt") {
        result = await importChatGPT(selectedFile);
      } else {
        const r = await importClaudeAI(selectedFile);
        result = { ...r, skipped: 0 };
      }
      onimported();
    } catch (e) {
      error = e instanceof Error ? e.message : "Import failed";
    } finally {
      importing = false;
    }
  }

  function handleClose() {
    if (importing) return;
    selectedFile = null;
    result = null;
    error = null;
    open = false;
    onclose();
  }
</script>

{#if open}
  <div class="modal-backdrop" role="presentation" onkeydown={(e) => e.key === "Escape" && handleClose()} onclick={handleClose}>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <!-- svelte-ignore a11y_interactive_supports_focus -->
    <div class="modal" role="dialog" aria-modal="true" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.key === "Escape" && handleClose()}>
      <h3>Import Conversations</h3>

      <div class="provider-select">
        <label>
          <input type="radio" bind:group={provider} value="claude-ai" disabled={importing} />
          Claude.ai
        </label>
        <label>
          <input type="radio" bind:group={provider} value="chatgpt" disabled={importing} />
          ChatGPT
        </label>
      </div>

      <p class="instructions">
        {#if provider === "claude-ai"}
          Upload <code>conversations.json</code> or a <code>.zip</code> from a Claude.ai data export.
        {:else}
          Upload a <code>.zip</code> from a ChatGPT data export.
        {/if}
      </p>

      <input
        bind:this={fileInput}
        type="file"
        accept={provider === "claude-ai" ? ".json,.zip" : ".zip"}
        onchange={handleFileChange}
        disabled={importing}
      />

      {#if error}
        <p class="error">{error}</p>
      {/if}

      {#if result}
        <p class="success">
          {result.imported + result.updated + (result.skipped ?? 0)} conversations processed
          ({result.imported} new{#if result.updated > 0}, {result.updated} updated{/if}{#if (result.skipped ?? 0) > 0}, {result.skipped} skipped{/if})
          {#if result.errors > 0}
            , {result.errors} errors
          {/if}
        </p>
      {/if}

      <div class="actions">
        <button onclick={handleClose} disabled={importing}>
          {result ? "Close" : "Cancel"}
        </button>
        {#if !result}
          <button
            class="primary"
            onclick={handleImport}
            disabled={!selectedFile || importing}
          >
            {importing ? "Importing..." : "Import"}
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal {
    background: var(--bg-primary);
    border: 1px solid var(--border-primary);
    border-radius: 8px;
    padding: 1.5rem;
    max-width: 28rem;
    width: 90%;
  }

  h3 {
    margin: 0 0 0.5rem;
    font-size: 1rem;
  }

  .provider-select {
    display: flex;
    gap: 1rem;
    margin-bottom: 1rem;
  }

  .provider-select label {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.85rem;
    cursor: pointer;
  }

  .instructions {
    color: var(--text-secondary);
    font-size: 0.85rem;
    margin: 0 0 1rem;
  }

  code {
    background: var(--bg-secondary);
    padding: 0.1em 0.3em;
    border-radius: 3px;
    font-size: 0.85em;
  }

  input[type="file"] {
    width: 100%;
    margin-bottom: 1rem;
  }

  .error {
    color: var(--accent-red, #e53e3e);
    font-size: 0.85rem;
    margin: 0 0 1rem;
  }

  .success {
    color: var(--accent-green, #38a169);
    font-size: 0.85rem;
    margin: 0 0 1rem;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    justify-content: flex-end;
  }

  button {
    padding: 0.4rem 1rem;
    border: 1px solid var(--border-primary);
    border-radius: 4px;
    background: var(--bg-secondary);
    color: var(--text-primary);
    cursor: pointer;
    font-size: 0.85rem;
  }

  button.primary {
    background: var(--accent-blue);
    color: white;
    border-color: var(--accent-blue);
  }

  button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
