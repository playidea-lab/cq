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
            <span class="text-logo">Claude Code</span>
            <span class="text-sep">·</span>
            <span class="text-logo">ChatGPT</span>
            <span class="text-sep">·</span>
            <span class="text-logo">Cursor</span>
            <span class="text-sep">·</span>
            <span class="text-logo">Gemini CLI</span>
            <span class="text-sep">·</span>
            <span class="text-logo">Codex CLI</span>
          </div>
          <div class="platform-row">
            <span class="text-logo">Mac</span>
            <span class="text-sep">·</span>
            <span class="text-logo">Linux</span>
            <span class="text-sep">·</span>
            <span class="text-logo">Windows</span>
            <span class="text-sep">·</span>
            <span class="text-logo">WSL2</span>
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
                <li>1 device at a time</li>
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
          <p class="contact-line">Enterprise or team inquiry? <a href="mailto:cm@playidealab.com">cm@playidealab.com</a></p>
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
.logo-row, .platform-row {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.platform-row {
  margin-top: 16px;
}
.text-logo {
  font-size: 16px;
  font-weight: 600;
  color: var(--vp-c-text-1);
}
.text-sep {
  color: var(--vp-c-text-3);
  font-size: 16px;
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
.contact-line {
  margin-top: 24px;
  font-size: 14px;
  color: var(--vp-c-text-3);
}
.contact-line a {
  color: var(--vp-c-brand-1);
  text-decoration: none;
}
.contact-line a:hover {
  text-decoration: underline;
}

/* Responsive */
@media (max-width: 768px) {
  .steps { flex-direction: column; align-items: center; }
  .step-arrow { transform: rotate(90deg); margin: 0; }
  .pricing-grid { grid-template-columns: 1fr; max-width: 360px; }
  .logo-row { gap: 20px; }
}
</style>
