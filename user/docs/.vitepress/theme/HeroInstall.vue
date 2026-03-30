<script setup>
import { ref } from 'vue'

const copied = ref(false)
const cmd = 'curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh'

function copy() {
  navigator.clipboard.writeText(cmd).then(() => {
    copied.value = true
    setTimeout(() => copied.value = false, 1500)
  })
}
</script>

<template>
  <div class="hero-version">v1.48.0</div>
  <div class="hero-install">
    <button class="install-btn" @click="copy">
      <span class="cmd">{{ cmd }}</span>
      <span class="copy-label">{{ copied ? '✓' : '📋' }}</span>
    </button>
    <a href="https://github.com/PlayIdea-Lab/cq" target="_blank" class="github-btn" aria-label="GitHub">
      <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
        <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
      </svg>
    </a>
  </div>
  <div class="hero-links">
    <a href="/cq/guide/quickstart" class="hero-link-btn">Quick Start</a>
    <a href="https://github.com/PlayIdea-Lab/cq/discussions" target="_blank" class="hero-link-btn secondary">Discussion</a>
  </div>
</template>

<style scoped>
.hero-install {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  margin-top: 24px;
  margin-bottom: 48px;
}

.install-btn {
  display: flex;
  align-items: center;
  gap: 10px;
  font-family: 'SF Mono', 'Fira Code', Menlo, Consolas, monospace;
  font-size: 13px;
  background: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  padding: 10px 16px;
  color: var(--vp-c-text-1);
  cursor: pointer;
  transition: border-color 0.2s;
}

.install-btn:hover {
  border-color: var(--vp-c-brand-1);
}

.cmd {
  user-select: all;
}

.copy-label {
  font-size: 16px;
  flex-shrink: 0;
}

.github-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: 8px;
  border: 1px solid var(--vp-c-divider);
  background: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
  transition: border-color 0.2s;
}

.github-btn:hover {
  border-color: var(--vp-c-brand-1);
}

.hero-links {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  margin-top: 16px;
  margin-bottom: 48px;
}

.hero-link-btn {
  font-size: 14px;
  font-weight: 600;
  padding: 8px 20px;
  border-radius: 8px;
  text-decoration: none;
  transition: all 0.2s;
}

.hero-link-btn:not(.secondary) {
  background: var(--vp-c-brand-1);
  color: var(--vp-c-white);
}

.hero-link-btn:not(.secondary):hover {
  background: var(--vp-c-brand-2);
}

.hero-link-btn.secondary {
  background: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
  border: 1px solid var(--vp-c-divider);
}

.hero-link-btn.secondary:hover {
  border-color: var(--vp-c-brand-1);
}

.hero-version {
  text-align: center;
  font-size: 13px;
  font-weight: 600;
  color: var(--vp-c-brand-1);
  margin-bottom: 8px;
  letter-spacing: 0.02em;
}
</style>
