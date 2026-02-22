<script setup>
import { ref } from 'vue'

const cmd = 'curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh'
const copied = ref(false)

function copy() {
  navigator.clipboard.writeText(cmd)
  copied.value = true
  setTimeout(() => { copied.value = false }, 2000)
}
</script>

<template>
  <div class="install-cmd">
    <span class="install-cmd__prefix">$</span>
    <code class="install-cmd__text">{{ cmd }}</code>
    <button class="install-cmd__btn" @click="copy" :aria-label="copied ? 'Copied' : 'Copy to clipboard'">
      <span v-if="copied">✓ Copied</span>
      <span v-else>Copy</span>
    </button>
  </div>
</template>

<style scoped>
.install-cmd {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  margin-top: 20px;
  padding: 10px 16px;
  background: var(--vp-code-block-bg);
  border: 1px solid var(--vp-c-divider);
  border-radius: 10px;
  max-width: 100%;
  overflow: hidden;
}

.install-cmd__prefix {
  color: var(--vp-c-brand-1);
  font-family: var(--vp-font-family-mono);
  font-size: 14px;
  font-weight: 600;
  flex-shrink: 0;
}

.install-cmd__text {
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  color: var(--vp-c-text-1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  background: transparent;
  padding: 0;
}

.install-cmd__btn {
  flex-shrink: 0;
  padding: 3px 10px;
  font-size: 12px;
  font-weight: 500;
  border-radius: 5px;
  border: 1px solid var(--vp-c-brand-1);
  color: var(--vp-c-brand-1);
  background: transparent;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
  line-height: 1.6;
}

.install-cmd__btn:hover {
  background: var(--vp-c-brand-1);
  color: #fff;
}
</style>
