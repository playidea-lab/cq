<script setup>
import DefaultTheme from 'vitepress/theme'
import { useData } from 'vitepress'
import HeroInstall from './HeroInstall.vue'

const { Layout } = DefaultTheme
const { frontmatter } = useData()
</script>

<template>
  <Layout>
    <template #home-hero-after>
      <HeroInstall v-if="frontmatter.layout === 'home'" />
    </template>
    <template #home-features-after>
      <div v-if="frontmatter.layout === 'home'" class="home-extra">

        <!-- Works with -->
        <section class="works-with">
          <h2>Works with any MCP-compatible AI</h2>
          <div class="logo-row">
            <div class="logo-item">
              <svg class="brand-icon" viewBox="0 0 24 24" fill="none"><path d="M16.5 3.5C14.76 3.5 13.09 4.26 12 5.6C10.91 4.26 9.24 3.5 7.5 3.5C4.42 3.5 2 5.92 2 9C2 12.78 5.4 15.89 10.55 20.54L12 21.85L13.45 20.54C18.6 15.89 22 12.78 22 9C22 5.92 19.58 3.5 16.5 3.5Z" fill="#D97706"/></svg>
              <span>Claude Code</span>
            </div>
            <div class="logo-item">
              <svg class="brand-icon" viewBox="0 0 24 24"><circle cx="12" cy="12" r="10" fill="#10A37F"/><path d="M12 7v5l3 3" stroke="white" stroke-width="2" fill="none" stroke-linecap="round"/></svg>
              <span>ChatGPT</span>
            </div>
            <div class="logo-item">
              <svg class="brand-icon" viewBox="0 0 24 24"><rect x="3" y="3" width="18" height="18" rx="4" fill="#000"/><path d="M8 16V8l8 8V8" stroke="white" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>
              <span>Cursor</span>
            </div>
            <div class="logo-item">
              <svg class="brand-icon" viewBox="0 0 24 24"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" stroke="#4285F4" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>
              <span>Gemini CLI</span>
            </div>
            <div class="logo-item">
              <svg class="brand-icon" viewBox="0 0 24 24"><rect x="2" y="4" width="20" height="16" rx="3" fill="#0D1117"/><path d="M7 10h10M7 14h6" stroke="#58A6FF" stroke-width="2" stroke-linecap="round"/></svg>
              <span>Codex CLI</span>
            </div>
          </div>
          <div class="platform-row">
            <div class="platform-item">🍎 Mac</div>
            <div class="platform-item">🐧 Linux</div>
            <div class="platform-item">🪟 Windows</div>
            <div class="platform-item">📦 WSL2</div>
          </div>
        </section>

        <!-- How it works -->
        <section class="how-it-works">
          <h2>How it works</h2>
          <div class="steps">
            <div class="step">
              <div class="step-num">1</div>
              <h3>Install</h3>
              <p>One command. No Docker, no Kubernetes, no config files.</p>
              <code>curl -fsSL https://pilab.kr/install | sh</code>
            </div>
            <div class="step-arrow">→</div>
            <div class="step">
              <div class="step-num">2</div>
              <h3>Serve</h3>
              <p>Your GPU connects to the relay. NAT, firewalls — all bypassed automatically.</p>
              <code>cq serve</code>
            </div>
            <div class="step-arrow">→</div>
            <div class="step">
              <div class="step-num">3</div>
              <h3>Control</h3>
              <p>From anywhere, tell AI to run experiments on your GPUs. Results flow back automatically.</p>
              <code>"Run lr sweep on my GPU"</code>
            </div>
          </div>
        </section>

        <!-- Pricing -->
        <section class="pricing">
          <h2>Pricing</h2>
          <p class="pricing-sub">Infrastructure is free. Intelligence is the product.</p>
          <div class="pricing-grid">
            <div class="plan">
              <h3>Free</h3>
              <div class="price">$0</div>
              <ul>
                <li>1 GPU connected</li>
                <li>10 GB Drive storage</li>
                <li>Unlimited knowledge records</li>
                <li>E2E encrypted relay</li>
                <li>All MCP tools</li>
              </ul>
            </div>
            <div class="plan featured">
              <h3>Pro</h3>
              <div class="price coming-soon">—<span></span></div>
              <ul>
                <li>Unlimited GPUs</li>
                <li>50 GB Drive storage</li>
                <li>AI autonomous experiment loop</li>
                <li>AI experiment recommendations</li>
                <li>Priority support</li>
              </ul>
              <div class="plan-badge">Coming Soon</div>
            </div>
            <div class="plan">
              <h3>Team</h3>
              <div class="price coming-soon">—<span></span></div>
              <ul>
                <li>Everything in Pro</li>
                <li>200 GB shared Drive</li>
                <li>Team GPU pooling</li>
                <li>Shared knowledge base</li>
                <li>Team dashboard</li>
              </ul>
              <div class="plan-badge">Coming Soon</div>
            </div>
          </div>
        </section>

        <!-- Final CTA -->
        <section class="final-cta">
          <h2>Your GPUs are waiting.</h2>
          <p>Get started in 30 seconds.</p>
          <a href="/cq/guide/quickstart" class="cta-btn">Quick Start →</a>
        </section>

      </div>
    </template>
  </Layout>
</template>

<style scoped>
.home-extra {
  max-width: 1152px;
  margin: 0 auto;
  padding: 0 24px;
}

/* Works with */
.works-with {
  text-align: center;
  padding: 64px 0;
  border-top: 1px solid var(--vp-c-divider);
}
.works-with h2 {
  font-size: 20px;
  color: var(--vp-c-text-2);
  font-weight: 400;
  margin-bottom: 32px;
}
.logo-row {
  display: flex;
  justify-content: center;
  gap: 40px;
  flex-wrap: wrap;
}
.logo-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 500;
  color: var(--vp-c-text-1);
}
.brand-icon {
  width: 24px;
  height: 24px;
  flex-shrink: 0;
}
.platform-row {
  display: flex;
  justify-content: center;
  gap: 32px;
  margin-top: 28px;
  flex-wrap: wrap;
}
.platform-item {
  font-size: 15px;
  color: var(--vp-c-text-2);
  font-weight: 500;
}

/* How it works */
.how-it-works {
  padding: 64px 0;
  border-top: 1px solid var(--vp-c-divider);
}
.how-it-works h2 {
  text-align: center;
  font-size: 28px;
  margin-bottom: 48px;
}
.steps {
  display: flex;
  align-items: flex-start;
  justify-content: center;
  gap: 24px;
}
.step {
  flex: 1;
  max-width: 280px;
  text-align: center;
}
.step-num {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--vp-c-brand-1);
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 18px;
  margin: 0 auto 16px;
}
.step h3 {
  font-size: 20px;
  margin-bottom: 8px;
}
.step p {
  font-size: 14px;
  color: var(--vp-c-text-2);
  margin-bottom: 12px;
  line-height: 1.5;
}
.step code {
  font-size: 13px;
  background: var(--vp-c-bg-soft);
  padding: 6px 12px;
  border-radius: 6px;
  border: 1px solid var(--vp-c-divider);
}
.step-arrow {
  font-size: 28px;
  color: var(--vp-c-text-3);
  margin-top: 40px;
}

/* Pricing */
.pricing {
  padding: 64px 0;
  border-top: 1px solid var(--vp-c-divider);
}
.pricing h2 {
  text-align: center;
  font-size: 28px;
  margin-bottom: 8px;
}
.pricing-sub {
  text-align: center;
  color: var(--vp-c-text-2);
  font-size: 16px;
  margin-bottom: 48px;
}
.pricing-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 24px;
  max-width: 900px;
  margin: 0 auto;
}
.plan {
  border: 1px solid var(--vp-c-divider);
  border-radius: 12px;
  padding: 32px 24px;
  text-align: center;
}
.plan.featured {
  border-color: var(--vp-c-brand-1);
  box-shadow: 0 4px 24px rgba(100, 108, 255, 0.15);
}
.plan h3 {
  font-size: 20px;
  margin-bottom: 8px;
}
.price {
  font-size: 36px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
  margin-bottom: 24px;
}
.price span {
  font-size: 16px;
  font-weight: 400;
  color: var(--vp-c-text-2);
}
.plan ul {
  list-style: none;
  padding: 0;
  text-align: left;
}
.plan li {
  padding: 8px 0;
  font-size: 14px;
  color: var(--vp-c-text-2);
  border-bottom: 1px solid var(--vp-c-divider);
}
.plan li:last-child { border-bottom: none; }
.plan li::before {
  content: '✓ ';
  color: var(--vp-c-brand-1);
  font-weight: 700;
}
.price.coming-soon {
  color: var(--vp-c-text-3);
}
.plan-badge {
  margin-top: 16px;
  font-size: 12px;
  font-weight: 600;
  color: var(--vp-c-brand-1);
  background: var(--vp-c-brand-soft);
  padding: 4px 12px;
  border-radius: 12px;
  display: inline-block;
}

/* Final CTA */
.final-cta {
  text-align: center;
  padding: 80px 0;
  border-top: 1px solid var(--vp-c-divider);
}
.final-cta h2 {
  font-size: 32px;
  margin-bottom: 8px;
}
.final-cta p {
  font-size: 18px;
  color: var(--vp-c-text-2);
  margin-bottom: 24px;
}
.cta-btn {
  display: inline-block;
  padding: 12px 32px;
  background: var(--vp-c-brand-1);
  color: white;
  border-radius: 8px;
  font-size: 16px;
  font-weight: 600;
  text-decoration: none;
  transition: background 0.2s;
}
.cta-btn:hover {
  background: var(--vp-c-brand-2);
}

/* Responsive */
@media (max-width: 768px) {
  .steps { flex-direction: column; align-items: center; }
  .step-arrow { transform: rotate(90deg); margin: 0; }
  .pricing-grid { grid-template-columns: 1fr; max-width: 360px; }
  .logo-row { gap: 20px; }
}
</style>
