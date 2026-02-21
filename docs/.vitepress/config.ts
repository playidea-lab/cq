import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'CQ',
  description: 'AI Project Orchestration Engine for Claude Code',
  base: '/cq/',

  head: [
    ['link', { rel: 'icon', href: '/cq/favicon.ico' }],
  ],

  themeConfig: {
    logo: '/logo.svg',
    siteTitle: 'CQ',

    nav: [
      { text: 'Guide', link: '/guide/' },
      { text: 'Workflow', link: '/workflow/' },
      { text: 'Examples', link: '/examples/' },
      { text: 'Reference', link: '/reference/commands' },
      { text: 'For Agents', link: '/for-agents/' },
      { text: 'GitHub', link: 'https://github.com/PlayIdea-Lab/cq' }
    ],

    sidebar: {
      '/guide/': [
        {
          text: 'Getting Started',
          items: [
            { text: 'What is CQ?', link: '/guide/' },
            { text: 'Installation', link: '/guide/install' },
            { text: 'Quick Start', link: '/guide/quickstart' },
            { text: 'Tiers', link: '/guide/tiers' },
            { text: 'C-Series Ecosystem', link: '/guide/ecosystem' },
          ]
        }
      ],
      '/workflow/': [
        {
          text: 'Workflow',
          items: [
            { text: 'Overview', link: '/workflow/' },
            { text: '/c4-plan', link: '/workflow/plan' },
            { text: '/c4-refine', link: '/workflow/refine' },
            { text: '/c4-run', link: '/workflow/run' },
            { text: '/c4-polish', link: '/workflow/polish' },
            { text: '/c4-finish', link: '/workflow/finish' },
          ]
        }
      ],
      '/reference/': [
        {
          text: 'Reference',
          items: [
            { text: 'Commands', link: '/reference/commands' },
            { text: 'Config', link: '/reference/config' },
            { text: 'Skills', link: '/reference/skills' },
          ]
        }
      ],
      '/examples/': [
        {
          text: 'Examples',
          items: [
            { text: 'Overview', link: '/examples/' },
            { text: 'Feature Planning', link: '/examples/feature-planning' },
            { text: 'Quick Bug Fix', link: '/examples/quick-fix' },
            { text: 'Distributed Experiments', link: '/examples/distributed-experiments' },
          ]
        }
      ],
      '/for-agents/': [
        {
          text: 'For AI Agents',
          items: [
            { text: 'Overview', link: '/for-agents/' },
            { text: 'llms.txt', link: '/for-agents/llms-txt' },
            { text: 'Skills JSONL', link: '/for-agents/skills-jsonl' },
          ]
        }
      ]
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/PlayIdea-Lab/cq' }
    ],

    footer: {
      message: 'Released under MIT + Commons Clause. Free to use, commercial resale prohibited.',
      copyright: 'Copyright © 2025 PlayIdea-Lab'
    },

    search: {
      provider: 'local'
    }
  }
})
