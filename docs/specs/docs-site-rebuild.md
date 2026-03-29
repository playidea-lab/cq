feature: docs-site-rebuild
domain: docs
description: CQ 문서 사이트를 EN/KO VitePress로 재구성하고 cq.pilab.kr에 배포. README 간결화.
source: /pi idea (.c4/ideas/docs-site-rebuild.md)

requirements:
  - id: R1
    pattern: event-driven
    text: "WHEN visitor lands on cq.pilab.kr THEN understands 3 CQ values (Brain, Gates, Growth) within 3 min"
  - id: R2
    pattern: event-driven
    text: "WHEN user follows /guide/install THEN CQ installed within 2 min"
  - id: R3
    pattern: event-driven
    text: "WHEN user follows /guide/quickstart THEN first task complete within 5 min"
  - id: R4
    pattern: ubiquitous
    text: "Every EN page has a /ko/ mirror"
  - id: R5
    pattern: event-driven
    text: "WHEN push to main THEN GitHub Actions builds VitePress and deploys to gh-pages"
  - id: R6
    pattern: event-driven
    text: "WHEN reading README THEN directed to cq.pilab.kr for details"
  - id: R7
    pattern: unwanted
    text: "Internal docs (specs/, reviews/, plans/) MUST NOT appear on Pages"

non_functional:
  - "Custom domain: cq.pilab.kr (CNAME → playidea-lab.github.io)"
  - "VitePress build passes without errors"
  - "Existing repo docs/ symlink structure preserved"
  - "EN default, /ko/ for Korean localization"

out_of_scope:
  - "Edge Agent (cq hub edge) documentation"
  - "GIF/video demos creation"
  - "Internal developer docs migration (stays in repo only)"
  - "DNS CNAME setup (manual infra step)"
