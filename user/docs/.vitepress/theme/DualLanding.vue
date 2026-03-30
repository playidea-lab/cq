<script setup>
import { ref } from 'vue'
import { withBase } from 'vitepress'

const activeTab = ref(0)
const email = ref('')
const submitted = ref(false)
let touchStartX = 0

async function submitEmail() {
  if (!email.value || submitted.value) return
  try {
    // Store in Supabase waitlist table
    await fetch('https://fhuomvsswxiwbfqjsgit.supabase.co/rest/v1/cq_waitlist', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'apikey': 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6ImZodW9tdnNzd3hpd2JmcWpzZ2l0Iiwicm9sZSI6ImFub24iLCJpYXQiOjE3Njg1Mzg4NzgsImV4cCI6MjA4NDExNDg3OH0.vyEfPomCBdQCv81SCMbQGQlE-S3BYgAjyzSOydV_wU4',
        'Prefer': 'return=minimal'
      },
      body: JSON.stringify({ email: email.value })
    })
  } catch (e) {
    // Silent fail — still show success
  }
  submitted.value = true
}

function onTouchStart(e) {
  touchStartX = e.touches[0].clientX
}
function onTouchEnd(e) {
  const diff = e.changedTouches[0].clientX - touchStartX
  if (Math.abs(diff) > 50) {
    activeTab.value = diff > 0 ? 0 : 1
  }
}
</script>

<template>
  <div class="dual-landing">
    <!-- Header -->
    <div class="hero-section">
      <h1 class="title">CQ</h1>
      <p class="subtitle">AI that knows you. GPU that works for you.</p>
    </div>

    <!-- Tabs -->
    <div class="tabs">
      <button :class="['tab', { active: activeTab === 0 }]" @click="activeTab = 0">
        🧠 Developers
      </button>
      <button :class="['tab', { active: activeTab === 1 }]" @click="activeTab = 1">
        🔌 Researchers
      </button>
    </div>

    <!-- Swipe container -->
    <div class="swipe-container" @touchstart="onTouchStart" @touchend="onTouchEnd">
      <div class="swipe-track" :style="{ transform: `translateX(-${activeTab * 50}%)` }">

        <!-- Panel 1: Developers -->
        <div class="panel">
          <div class="panel-hero">
            <h2>Knowledge: Connect, Adapt, Evolve.</h2>
            <p>Switch AI. Keep everything. Your knowledge follows you across Claude, ChatGPT, Cursor, and beyond.</p>
            <img :src="withBase('/demo-dev.svg')" alt="CQ Knowledge Demo" class="panel-demo" />
          </div>
          <div class="features">
            <div class="feature">
              <div class="feature-icon">🔗</div>
              <h3>Connect</h3>
              <p>Any MCP-compatible AI becomes your workspace. Claude Code, ChatGPT, Cursor, Gemini — one brain across all.</p>
            </div>
            <div class="feature">
              <div class="feature-icon">🎯</div>
              <h3>Adapt</h3>
              <p>CQ learns your coding style, judgment, and preferences. Session by session, it becomes more you.</p>
            </div>
            <div class="feature">
              <div class="feature-icon">🔄</div>
              <h3>Evolve</h3>
              <p>AI runs experiments autonomously — code refactors, prompt variations, data analysis. It tries, compares, and finds the best.</p>
            </div>
            <div class="feature">
              <div class="feature-icon">🤝</div>
              <h3>Share</h3>
              <p>Team knowledge compounds. A teammate's discovery becomes your AI's context. Collective intelligence, not isolated sessions.</p>
            </div>
          </div>
        </div>

        <!-- Panel 2: Researchers -->
        <div class="panel">
          <div class="panel-hero">
            <h2>GPU Anywhere, Anytime, Anything.</h2>
            <p>Your GPUs are idle 70% of the time. CQ connects them to AI — zero config, any OS, encrypted.</p>
            <img :src="withBase('/demo.svg')" alt="CQ GPU Demo" class="panel-demo" />
          </div>
          <div class="features">
            <div class="feature">
              <div class="feature-icon">🔌</div>
              <h3>GPU from Anywhere</h3>
              <p>Access your lab GPUs from home — no SSH, no VPN, no port forwarding. Relay traverses NAT and firewalls automatically.</p>
            </div>
            <div class="feature">
              <div class="feature-icon">🔄</div>
              <h3>AI Runs Experiments</h3>
              <p>Tell AI what to try. It distributes experiments across your GPUs, collects results, and designs the next round — while you sleep.</p>
            </div>
            <div class="feature">
              <div class="feature-icon">🧠</div>
              <h3>Knowledge Compounds</h3>
              <p>Every experiment result is recorded. Next session, AI references past findings. 6 months of knowledge becomes your moat.</p>
            </div>
            <div class="feature">
              <div class="feature-icon">🔒</div>
              <h3>End-to-End Encrypted</h3>
              <p>X25519 + NaCl box encryption. The relay server sees only routing metadata — never your code, data, or results.</p>
            </div>
          </div>
        </div>

      </div>
    </div>

    <!-- Dot indicator -->
    <div class="dots">
      <span :class="['dot', { active: activeTab === 0 }]" @click="activeTab = 0"></span>
      <span :class="['dot', { active: activeTab === 1 }]" @click="activeTab = 1"></span>
    </div>

    <!-- Shared bottom sections -->
    <div class="bottom-sections">
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
            <h3>Connect</h3>
            <p>Add CQ to your AI tool. Knowledge starts flowing automatically.</p>
            <code>cq serve</code>
          </div>
          <div class="step-arrow">→</div>
          <div class="step">
            <div class="step-num">3</div>
            <h3>Evolve</h3>
            <p>Every session, AI knows you better. Your experiments compound.</p>
            <code>"It just works."</code>
          </div>
        </div>
      </section>

      <!-- Coming Soon + Notify -->
      <section class="coming-soon-section">
        <div class="coming-soon-badge">Coming Soon</div>
        <h2>CQ is launching soon.</h2>
        <p class="coming-sub">Free tier available now. Pro & Team plans are on the way.</p>
        <p class="coming-sub">Get notified when we officially launch.</p>

        <form class="notify-form" @submit.prevent="submitEmail">
          <input
            v-model="email"
            type="email"
            placeholder="your@email.com"
            class="notify-input"
            required
          />
          <button type="submit" class="notify-btn" :disabled="submitted">
            {{ submitted ? '✓ Subscribed!' : 'Notify Me' }}
          </button>
        </form>
        <p v-if="submitted" class="notify-thanks">We'll let you know when CQ launches. 🎉</p>
      </section>

      <!-- CTA -->
      <section class="final-cta">
        <h2>Try the free version now.</h2>
        <p>Get started in 30 seconds.</p>
        <a href="/cq/guide/quickstart" class="cta-btn">Quick Start →</a>
        <p class="contact-line">Enterprise or team inquiry? <a href="mailto:cm@playidealab.com">cm@playidealab.com</a></p>
      </section>
    </div>
  </div>
</template>

<style scoped>
.dual-landing {
  max-width: 1152px;
  margin: 0 auto;
  padding: 0 24px;
}

/* Hero */
.hero-section {
  text-align: center;
  padding: 80px 0 40px;
}
.title {
  font-size: 48px;
  font-weight: 800;
  background: linear-gradient(135deg, var(--vp-c-brand-1), var(--vp-c-brand-2));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 12px;
}
.subtitle {
  font-size: 20px;
  color: var(--vp-c-text-2);
  max-width: 500px;
  margin: 0 auto;
  line-height: 1.5;
}

/* Tabs */
.tabs {
  display: flex;
  justify-content: center;
  gap: 0;
  margin: 32px auto;
  max-width: 400px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 12px;
  overflow: hidden;
}
.tab {
  flex: 1;
  padding: 12px 24px;
  font-size: 15px;
  font-weight: 600;
  border: none;
  background: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
  cursor: pointer;
  transition: all 0.2s;
}
.tab.active {
  background: var(--vp-c-brand-1);
  color: white;
}

/* Swipe */
.swipe-container {
  overflow: hidden;
  border-radius: 16px;
}
.swipe-track {
  display: flex;
  width: 200%;
  transition: transform 0.4s cubic-bezier(0.4, 0, 0.2, 1);
}
.panel {
  width: 50%;
  padding: 0 24px;
}
.panel-hero {
  text-align: center;
  padding: 40px 0;
}
.panel-hero h2 {
  font-size: 32px;
  font-weight: 700;
  margin-bottom: 12px;
}
.panel-hero p {
  font-size: 16px;
  color: var(--vp-c-text-2);
  max-width: 560px;
  margin: 0 auto;
  line-height: 1.6;
}
.panel-demo {
  max-width: 520px;
  width: 100%;
  margin: 32px auto 0;
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
  display: block;
}

/* Features */
.features {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 20px;
  max-width: 800px;
  margin: 0 auto 40px;
}
.feature {
  border: 1px solid var(--vp-c-divider);
  border-radius: 12px;
  padding: 24px;
}
.feature-icon {
  font-size: 28px;
  margin-bottom: 12px;
}
.feature h3 {
  font-size: 18px;
  margin-bottom: 8px;
}
.feature p {
  font-size: 14px;
  color: var(--vp-c-text-2);
  line-height: 1.6;
}

/* Dots */
.dots {
  display: flex;
  justify-content: center;
  gap: 8px;
  margin: 24px 0 48px;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--vp-c-divider);
  cursor: pointer;
  transition: background 0.2s;
}
.dot.active {
  background: var(--vp-c-brand-1);
  width: 24px;
  border-radius: 4px;
}

/* Bottom sections (reuse from HomeLayout) */
.bottom-sections { max-width: 1152px; margin: 0 auto; }

.works-with { text-align: center; padding: 64px 0; border-top: 1px solid var(--vp-c-divider); }
.works-with h2 { font-size: 20px; color: var(--vp-c-text-2); font-weight: 400; margin-bottom: 32px; }
.logo-row, .platform-row { display: flex; justify-content: center; align-items: center; gap: 12px; flex-wrap: wrap; }
.platform-row { margin-top: 16px; }
.text-logo { font-size: 16px; font-weight: 600; color: var(--vp-c-text-1); }
.text-sep { color: var(--vp-c-text-3); font-size: 16px; }

.how-it-works { padding: 64px 0; border-top: 1px solid var(--vp-c-divider); }
.how-it-works h2 { text-align: center; font-size: 28px; margin-bottom: 48px; }
.steps { display: flex; align-items: flex-start; justify-content: center; gap: 24px; }
.step { flex: 1; max-width: 280px; text-align: center; }
.step-num { width: 40px; height: 40px; border-radius: 50%; background: var(--vp-c-brand-1); color: white; display: flex; align-items: center; justify-content: center; font-weight: 700; font-size: 18px; margin: 0 auto 16px; }
.step h3 { font-size: 20px; margin-bottom: 8px; }
.step p { font-size: 14px; color: var(--vp-c-text-2); margin-bottom: 12px; line-height: 1.5; }
.step code { font-size: 13px; background: var(--vp-c-bg-soft); padding: 6px 12px; border-radius: 6px; border: 1px solid var(--vp-c-divider); }
.step-arrow { font-size: 28px; color: var(--vp-c-text-3); margin-top: 40px; }

.pricing { padding: 64px 0; border-top: 1px solid var(--vp-c-divider); }
.pricing h2 { text-align: center; font-size: 28px; margin-bottom: 8px; }
.pricing-sub { text-align: center; color: var(--vp-c-text-2); font-size: 16px; margin-bottom: 48px; }
.pricing-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 24px; max-width: 900px; margin: 0 auto; }
.plan { border: 1px solid var(--vp-c-divider); border-radius: 12px; padding: 32px 24px; text-align: center; }
.plan.featured { border-color: var(--vp-c-brand-1); box-shadow: 0 4px 24px rgba(100, 108, 255, 0.15); }
.plan h3 { font-size: 20px; margin-bottom: 8px; }
.price { font-size: 36px; font-weight: 700; color: var(--vp-c-brand-1); margin-bottom: 24px; }
.price span { font-size: 16px; font-weight: 400; color: var(--vp-c-text-2); }
.price.coming-soon { color: var(--vp-c-text-3); }
.plan-badge { margin-top: 16px; font-size: 12px; font-weight: 600; color: var(--vp-c-brand-1); background: var(--vp-c-brand-soft); padding: 4px 12px; border-radius: 12px; display: inline-block; }
.plan ul { list-style: none; padding: 0; text-align: left; }
.plan li { padding: 8px 0; font-size: 14px; color: var(--vp-c-text-2); border-bottom: 1px solid var(--vp-c-divider); }
.plan li:last-child { border-bottom: none; }
.plan li::before { content: '✓ '; color: var(--vp-c-brand-1); font-weight: 700; }

/* Coming Soon */
.coming-soon-section {
  text-align: center;
  padding: 80px 0;
  border-top: 1px solid var(--vp-c-divider);
}
.coming-soon-badge {
  display: inline-block;
  font-size: 14px;
  font-weight: 700;
  color: var(--vp-c-brand-1);
  background: var(--vp-c-brand-soft);
  padding: 6px 20px;
  border-radius: 20px;
  margin-bottom: 24px;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}
.coming-soon-section h2 {
  font-size: 32px;
  margin-bottom: 12px;
}
.coming-sub {
  font-size: 16px;
  color: var(--vp-c-text-2);
  margin-bottom: 8px;
}
.notify-form {
  display: flex;
  justify-content: center;
  gap: 8px;
  margin-top: 32px;
  max-width: 440px;
  margin-left: auto;
  margin-right: auto;
}
.notify-input {
  flex: 1;
  padding: 12px 16px;
  font-size: 15px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background: var(--vp-c-bg-soft);
  color: var(--vp-c-text-1);
  outline: none;
  transition: border-color 0.2s;
}
.notify-input:focus {
  border-color: var(--vp-c-brand-1);
}
.notify-btn {
  padding: 12px 24px;
  font-size: 15px;
  font-weight: 600;
  border: none;
  border-radius: 8px;
  background: var(--vp-c-brand-1);
  color: white;
  cursor: pointer;
  transition: background 0.2s;
  white-space: nowrap;
}
.notify-btn:hover:not(:disabled) {
  background: var(--vp-c-brand-2);
}
.notify-btn:disabled {
  background: var(--vp-c-brand-2);
  cursor: default;
}
.notify-thanks {
  margin-top: 16px;
  font-size: 15px;
  color: var(--vp-c-brand-1);
  font-weight: 500;
}

.final-cta { text-align: center; padding: 80px 0; border-top: 1px solid var(--vp-c-divider); }
.final-cta h2 { font-size: 32px; margin-bottom: 8px; }
.final-cta p { font-size: 18px; color: var(--vp-c-text-2); margin-bottom: 24px; }
.cta-btn { display: inline-block; padding: 12px 32px; background: var(--vp-c-brand-1); color: white; border-radius: 8px; font-size: 16px; font-weight: 600; text-decoration: none; transition: background 0.2s; }
.cta-btn:hover { background: var(--vp-c-brand-2); }
.contact-line { margin-top: 24px; font-size: 14px; color: var(--vp-c-text-3); }
.contact-line a { color: var(--vp-c-brand-1); text-decoration: none; }
.contact-line a:hover { text-decoration: underline; }

@media (max-width: 768px) {
  .features { grid-template-columns: 1fr; }
  .steps { flex-direction: column; align-items: center; }
  .step-arrow { transform: rotate(90deg); margin: 0; }
  .pricing-grid { grid-template-columns: 1fr; max-width: 360px; }
  .title { font-size: 36px; }
}
</style>
