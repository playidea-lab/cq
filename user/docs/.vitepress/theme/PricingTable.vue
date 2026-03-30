<script setup>
import { ref } from 'vue'

const showModal = ref(false)

function handlePro() {
  showModal.value = true
  setTimeout(() => showModal.value = false, 3000)
}
</script>

<template>
  <section class="pricing-section">
    <h2 class="pricing-title">Pricing</h2>
    <p class="pricing-subtitle">Your GPUs. Our orchestration + knowledge layer.</p>

    <div class="pricing-grid">
      <!-- Free -->
      <div class="pricing-card">
        <div class="pricing-badge">Free</div>
        <div class="pricing-price">$0</div>
        <div class="pricing-period">forever</div>
        <ul class="pricing-features">
          <li>1 device at a time</li>
          <li>Local knowledge (SQLite)</li>
          <li>All AI tools supported</li>
          <li>70+ skills</li>
          <li>Unlimited local usage</li>
        </ul>
        <a href="/cq/guide/install" class="pricing-btn secondary">Get Started</a>
      </div>

      <!-- Pro -->
      <div class="pricing-card featured">
        <div class="pricing-popular">Early Bird</div>
        <div class="pricing-badge">Pro</div>
        <div class="pricing-price">$5<span class="pricing-original">$10</span></div>
        <div class="pricing-period">/month</div>
        <ul class="pricing-features">
          <li><strong>Unlimited GPUs</strong> (yours)</li>
          <li>Cloud knowledge (pgvector)</li>
          <li>Relay P2P encrypted</li>
          <li>Hub orchestration</li>
          <li>Research Loop</li>
        </ul>
        <button class="pricing-btn primary" @click="handlePro">Get Pro</button>
      </div>

      <!-- Team -->
      <div class="pricing-card">
        <div class="pricing-badge">Team</div>
        <div class="pricing-price">Custom</div>
        <div class="pricing-period">contact us</div>
        <ul class="pricing-features">
          <li>Everything in Pro</li>
          <li>Shared knowledge base</li>
          <li>Team GPU pool</li>
          <li>Priority support</li>
          <li>Custom onboarding</li>
        </ul>
        <a href="mailto:hello@pilab.kr" class="pricing-btn secondary">Contact</a>
      </div>
    </div>

    <!-- Modal -->
    <Transition name="fade">
      <div v-if="showModal" class="pricing-modal-overlay" @click="showModal = false">
        <div class="pricing-modal" @click.stop>
          <div class="pricing-modal-icon">🚀</div>
          <h3>Coming Soon</h3>
          <p>Pro plan is launching soon.<br/>Stay tuned!</p>
          <a href="https://github.com/PlayIdea-Lab/cq" target="_blank" class="pricing-btn primary">
            Star on GitHub
          </a>
        </div>
      </div>
    </Transition>
  </section>
</template>

<style scoped>
.pricing-section {
  max-width: 1100px;
  margin: 0 auto;
  padding: 64px 24px 80px;
}

.pricing-title {
  text-align: center;
  font-size: 32px;
  font-weight: 700;
  color: var(--vp-c-text-1);
  margin-bottom: 8px;
}

.pricing-subtitle {
  text-align: center;
  font-size: 16px;
  color: var(--vp-c-text-2);
  margin-bottom: 48px;
}

.pricing-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 24px;
}

@media (max-width: 768px) {
  .pricing-grid {
    grid-template-columns: 1fr;
    max-width: 400px;
    margin: 0 auto;
  }
}

.pricing-card {
  position: relative;
  background: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 12px;
  padding: 32px 24px;
  display: flex;
  flex-direction: column;
  align-items: center;
  transition: border-color 0.2s;
}

.pricing-card:hover {
  border-color: var(--vp-c-brand-1);
}

.pricing-card.featured {
  border: 2px solid var(--vp-c-brand-1);
  transform: scale(1.04);
}

.pricing-popular {
  position: absolute;
  top: -12px;
  background: var(--vp-c-brand-1);
  color: var(--vp-c-white);
  font-size: 12px;
  font-weight: 700;
  padding: 4px 16px;
  border-radius: 20px;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.pricing-badge {
  font-size: 18px;
  font-weight: 700;
  color: var(--vp-c-text-1);
  margin-bottom: 16px;
}

.pricing-price {
  font-size: 48px;
  font-weight: 800;
  color: var(--vp-c-text-1);
  line-height: 1;
}

.pricing-original {
  font-size: 20px;
  font-weight: 400;
  color: var(--vp-c-text-3);
  text-decoration: line-through;
  margin-left: 8px;
}

.pricing-period {
  font-size: 14px;
  color: var(--vp-c-text-2);
  margin-bottom: 24px;
}

.pricing-features {
  list-style: none;
  padding: 0;
  margin: 0 0 32px;
  width: 100%;
}

.pricing-features li {
  font-size: 14px;
  color: var(--vp-c-text-2);
  padding: 8px 0;
  border-bottom: 1px solid var(--vp-c-divider);
}

.pricing-features li:last-child {
  border-bottom: none;
}

.pricing-btn {
  display: inline-block;
  font-size: 14px;
  font-weight: 600;
  padding: 10px 28px;
  border-radius: 8px;
  text-decoration: none;
  cursor: pointer;
  border: none;
  transition: all 0.2s;
  margin-top: auto;
}

.pricing-btn.primary {
  background: var(--vp-c-brand-1);
  color: var(--vp-c-white);
}

.pricing-btn.primary:hover {
  background: var(--vp-c-brand-2);
}

.pricing-btn.secondary {
  background: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  border: 1px solid var(--vp-c-divider);
}

.pricing-btn.secondary:hover {
  border-color: var(--vp-c-brand-1);
}

/* Modal */
.pricing-modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}

.pricing-modal {
  background: var(--vp-c-bg);
  border: 1px solid var(--vp-c-divider);
  border-radius: 16px;
  padding: 48px 40px;
  text-align: center;
  max-width: 360px;
}

.pricing-modal-icon {
  font-size: 48px;
  margin-bottom: 16px;
}

.pricing-modal h3 {
  font-size: 24px;
  font-weight: 700;
  color: var(--vp-c-text-1);
  margin-bottom: 12px;
}

.pricing-modal p {
  font-size: 15px;
  color: var(--vp-c-text-2);
  margin-bottom: 24px;
  line-height: 1.6;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
