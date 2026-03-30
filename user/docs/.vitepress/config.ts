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
              text: 'Getting Started',
              items: [
                { text: 'Installation', link: '/ko/guide/install' },
                { text: 'Quickstart', link: '/ko/guide/quickstart' },
              ],
            },
            {
              text: 'Core Concepts',
              items: [
                { text: 'Tiers', link: '/ko/guide/tiers' },
                { text: 'Growth Loop', link: '/ko/guide/growth-loop' },
              ],
            },
            {
              text: 'Infrastructure',
              items: [
                { text: 'Worker Setup', link: '/ko/guide/worker' },
                { text: 'Remote Brain', link: '/ko/guide/relay' },
              ],
            },
            {
              text: 'Advanced',
              items: [
                { text: 'Craft', link: '/ko/guide/craft' },
                { text: 'DAG & Cron', link: '/ko/guide/dag-cron' },
                { text: 'Relay', link: '/ko/guide/relay' },
                { text: 'Worker', link: '/ko/guide/worker' },
              ],
            },
          ],
          '/ko/examples/': [
            {
              text: 'Workflow',
              items: [
                { text: 'First Task', link: '/ko/examples/first-task' },
                { text: 'Bug Fix', link: '/ko/examples/bug-fix' },
                { text: 'Feature Planning', link: '/ko/examples/feature-planning' },
              ],
            },
            {
              text: 'Brain',
              items: [
                { text: 'Remote MCP 연결', link: '/ko/examples/remote-mcp' },
                { text: 'ChatGPT → Claude', link: '/ko/examples/chatgpt-to-claude' },
                { text: '아이디어 세션', link: '/ko/examples/idea-sessions' },
                { text: 'Growth Loop', link: '/ko/examples/growth-loop-in-action' },
              ],
            },
            {
              text: 'Research',
              items: [
                { text: 'Research Loop', link: '/ko/examples/research-loop' },
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
          ],
        },
        {
          text: 'Core Concepts',
          items: [
            { text: 'Tiers', link: '/guide/tiers' },
            { text: 'Growth Loop', link: '/guide/growth-loop' },
          ],
        },
        {
          text: 'Infrastructure',
          items: [
            { text: 'Worker Setup', link: '/guide/worker' },
            { text: 'Remote Brain', link: '/guide/relay' },
          ],
        },
        {
          text: 'Advanced',
          items: [
            { text: 'Craft', link: '/guide/craft' },
            { text: 'DAG & Cron', link: '/guide/dag-cron' },
            { text: 'Relay', link: '/guide/relay' },
            { text: 'Worker', link: '/guide/worker' },
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
          text: 'Brain',
          items: [
            { text: 'Connect Remote MCP', link: '/examples/remote-mcp' },
            { text: 'ChatGPT → Claude', link: '/examples/chatgpt-to-claude' },
            { text: 'Idea Sessions', link: '/examples/idea-sessions' },
            { text: 'Growth Loop', link: '/examples/growth-loop-in-action' },
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
