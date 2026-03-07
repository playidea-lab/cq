import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'CQ',
  description: 'AI Project Orchestration Engine for Claude Code',
  base: '/cq/',

  head: [
    ['link', { rel: 'icon', href: '/cq/favicon.ico' }],
  ],

  locales: {
    root: {
      label: 'English',
      lang: 'en',
      themeConfig: {
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
                { text: 'Remote Worker Setup', link: '/guide/worker-setup' },
                { text: 'C-Series Ecosystem', link: '/guide/ecosystem' },
              ]
            }
          ],
          '/workflow/': [
            {
              text: 'Workflow',
              items: [
                { text: 'Overview', link: '/workflow/' },
                { text: '/pi (Ideation)', link: '/workflow/pi' },
                { text: '/c4-plan', link: '/workflow/plan' },
                { text: '/c4-run', link: '/workflow/run' },
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
                { text: 'Your First Task', link: '/examples/first-task' },
                { text: 'Quick Bug Fix', link: '/examples/quick-fix' },
                { text: 'Feature Planning', link: '/examples/feature-planning' },
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
      }
    },

    ko: {
      label: '한국어',
      lang: 'ko',
      link: '/ko/',
      themeConfig: {
        nav: [
          { text: '가이드', link: '/ko/guide/' },
          { text: '워크플로우', link: '/ko/workflow/' },
          { text: '예시', link: '/ko/examples/' },
          { text: '레퍼런스', link: '/ko/reference/commands' },
          { text: 'For Agents', link: '/for-agents/' },
          { text: 'GitHub', link: 'https://github.com/PlayIdea-Lab/cq' }
        ],
        sidebar: {
          '/ko/guide/': [
            {
              text: '시작하기',
              items: [
                { text: 'CQ란?', link: '/ko/guide/' },
                { text: '설치', link: '/ko/guide/install' },
                { text: '빠른 시작', link: '/ko/guide/quickstart' },
                { text: '티어', link: '/ko/guide/tiers' },
                { text: '원격 워커 설정', link: '/ko/guide/worker-setup' },
                { text: 'C 시리즈 생태계', link: '/ko/guide/ecosystem' },
              ]
            }
          ],
          '/ko/workflow/': [
            {
              text: '워크플로우',
              items: [
                { text: '개요', link: '/ko/workflow/' },
                { text: '/pi (아이디어 탐색)', link: '/ko/workflow/pi' },
                { text: '/c4-plan', link: '/ko/workflow/plan' },
                { text: '/c4-run', link: '/ko/workflow/run' },
                { text: '/c4-finish', link: '/ko/workflow/finish' },
              ]
            }
          ],
          '/ko/reference/': [
            {
              text: '레퍼런스',
              items: [
                { text: '명령어', link: '/ko/reference/commands' },
                { text: '설정', link: '/ko/reference/config' },
                { text: '스킬', link: '/ko/reference/skills' },
              ]
            }
          ],
          '/ko/examples/': [
            {
              text: '예시',
              items: [
                { text: '개요', link: '/ko/examples/' },
                { text: '첫 번째 태스크', link: '/ko/examples/first-task' },
                { text: '빠른 버그 수정', link: '/ko/examples/quick-fix' },
                { text: '기능 계획', link: '/ko/examples/feature-planning' },
                { text: '분산 실험', link: '/ko/examples/distributed-experiments' },
              ]
            }
          ],
        },
      }
    }
  },

  themeConfig: {
    logo: '/logo.svg',
    siteTitle: 'CQ',

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
