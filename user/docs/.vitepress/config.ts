import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'CQ',
  description: 'GPU Anywhere, Anytime, Anything',
  base: '/cq/',

  ignoreDeadLinks: true,

  markdown: {
    attrs: { disable: true },
  },

  srcExclude: [
    '**/specs/**',
    '**/reviews/**',
    '**/plans/**',
    '**/PLAN-*',
    '**/plan-*',
    '**/antipattern-*',
    '**/c4-to-cq-*',
    '**/c9-*',
    '**/deployment-topology.md',
    '**/developer-experience.md',
    '**/ARCHITECTURE.md',
    '**/ROADMAP.md',
    '**/config-guide.md',
    '**/usage-guide.md',
    '**/developer-guide/**',
    '**/gpu-worker/**',
    '**/ops/**',
    '**/node_modules/**',
  ],

  locales: {
    root: {
      label: 'English',
      lang: 'en',
    },
    ko: {
      label: '한국어',
      lang: 'ko',
      link: '/ko/',
      themeConfig: {
        nav: [
          { text: 'Guide', link: '/ko/guide/install' },
          { text: 'Examples', link: '/ko/examples/first-task' },
          { text: 'Reference', link: '/ko/reference/commands' },
          {
            text: 'GitHub',
            link: 'https://github.com/PlayIdea-Lab/cq',
          },
        ],
        sidebar: {
          '/ko/guide/': [
            {
              text: '시작하기',
              items: [
                { text: '설치', link: '/ko/guide/install' },
                { text: '빠른 시작', link: '/ko/guide/quickstart' },
                { text: '가격 (Free/Pro/Team)', link: '/ko/guide/tiers' },
              ],
            },
            {
              text: 'GPU Anywhere',
              items: [
                { text: 'Relay (NAT 통과)', link: '/ko/guide/relay' },
                { text: 'GPU 워커 설정', link: '/ko/guide/worker' },
                { text: '워커 구성', link: '/ko/guide/worker-setup' },
              ],
            },
            {
              text: 'AI 워크스페이스',
              items: [
                { text: '지식 루프', link: '/ko/guide/growth-loop' },
                { text: '원격 AI 워크스페이스', link: '/ko/guide/remote-brain' },
                { text: 'Craft (스킬 & 에이전트)', link: '/ko/guide/craft' },
                { text: 'DAG & Cron', link: '/ko/guide/dag-cron' },
              ],
            },
          ],
          '/ko/examples/': [
            {
              text: '워크플로우',
              items: [
                { text: '첫 태스크', link: '/ko/examples/first-task' },
                { text: '버그 수정', link: '/ko/examples/bug-fix' },
                { text: '기능 기획', link: '/ko/examples/feature-planning' },
              ],
            },
            {
              text: '지식',
              items: [
                { text: 'Remote MCP 연결', link: '/ko/examples/remote-mcp' },
                { text: 'ChatGPT → Claude', link: '/ko/examples/chatgpt-to-claude' },
                { text: '아이디어 세션', link: '/ko/examples/idea-sessions' },
                { text: '지식 루프', link: '/ko/examples/growth-loop-in-action' },
              ],
            },
            {
              text: '연구',
              items: [
                { text: '연구 루프', link: '/ko/examples/research-loop' },
              ],
            },
          ],
          '/ko/reference/': [
            {
              text: 'Reference',
              items: [
                { text: 'Commands', link: '/ko/reference/commands' },
                { text: 'Skills', link: '/ko/reference/skills' },
                { text: 'Tools', link: '/ko/reference/tools' },
                { text: 'Architecture', link: '/ko/reference/architecture' },
              ],
            },
          ],
        },
      },
    },
  },

  themeConfig: {
    search: {
      provider: 'local'
    },

    nav: [
      { text: 'Guide', link: '/guide/install' },
      { text: 'Examples', link: '/examples/first-task' },
      { text: 'Reference', link: '/reference/commands' },
      {
        text: 'GitHub',
        link: 'https://github.com/PlayIdea-Lab/cq',
      },
    ],

    sidebar: {
      '/guide/': [
        {
          text: 'Getting Started',
          items: [
            { text: 'Installation', link: '/guide/install' },
            { text: 'Quickstart', link: '/guide/quickstart' },
            { text: 'Pricing (Free/Pro/Team)', link: '/guide/tiers' },
          ],
        },
        {
          text: 'GPU Anywhere',
          items: [
            { text: 'Relay (NAT Traversal)', link: '/guide/relay' },
            { text: 'GPU Worker Setup', link: '/guide/worker' },
            { text: 'Worker Configuration', link: '/guide/worker-setup' },
          ],
        },
        {
          text: 'AI Workspace',
          items: [
            { text: 'Knowledge Loop', link: '/guide/growth-loop' },
            { text: 'Remote AI Workspace', link: '/guide/remote-brain' },
            { text: 'Craft (Skills & Agents)', link: '/guide/craft' },
            { text: 'DAG & Cron', link: '/guide/dag-cron' },
          ],
        },
      ],
      '/examples/': [
        {
          text: 'Workflow',
          items: [
            { text: 'First Task', link: '/examples/first-task' },
            { text: 'Bug Fix', link: '/examples/bug-fix' },
            { text: 'Feature Planning', link: '/examples/feature-planning' },
          ],
        },
        {
          text: 'Knowledge',
          items: [
            { text: 'Remote MCP Connection', link: '/examples/remote-mcp' },
            { text: 'ChatGPT → Claude', link: '/examples/chatgpt-to-claude' },
            { text: 'Idea Sessions', link: '/examples/idea-sessions' },
            { text: 'Knowledge Loop', link: '/examples/growth-loop-in-action' },
          ],
        },
        {
          text: 'Research',
          items: [
            { text: 'Research Loop', link: '/examples/research-loop' },
          ],
        },
      ],
      '/reference/': [
        {
          text: 'Reference',
          items: [
            { text: 'Commands', link: '/reference/commands' },
            { text: 'Skills', link: '/reference/skills' },
            { text: 'Tools', link: '/reference/tools' },
            { text: 'Architecture', link: '/reference/architecture' },
          ],
        },
      ],
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/PlayIdea-Lab/cq' },
    ],
  },
})
