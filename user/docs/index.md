---
layout: home

hero:
  name: "CQ"
  text: "GPU Anywhere, Anytime, Anything."
  tagline: "Your GPUs are idle 70% of the time. CQ connects them to AI — zero config, any OS, encrypted."
  actions:
    - theme: brand
      text: Quick Start
      link: /guide/quickstart
    - theme: alt
      text: Installation
      link: /guide/install

features:
  - icon: "\U0001F50C"
    title: GPU from Anywhere
    details: "Access your lab GPUs from home — no SSH, no VPN, no port forwarding. Relay traverses NAT and firewalls automatically."

  - icon: "\U0001F504"
    title: AI Runs Experiments
    details: "Tell AI what to try. It distributes experiments across your GPUs, collects results, and designs the next round — while you sleep."

  - icon: "\U0001F9E0"
    title: Knowledge Compounds
    details: "Every experiment result is recorded. Next session, AI references past findings. 6 months of knowledge becomes your moat."

  - icon: "\U0001F512"
    title: End-to-End Encrypted
    details: "X25519 + NaCl box encryption. The relay server sees only routing metadata — never your code, data, or results."
---

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```
