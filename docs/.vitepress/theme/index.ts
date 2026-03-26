import DefaultTheme from 'vitepress/theme'
import { h } from 'vue'
import { useData, useRoute } from 'vitepress'
import giscusTalk from 'vitepress-plugin-comment-with-giscus'
import InstallCmd from './InstallCmd.vue'

export default {
  extends: DefaultTheme,
  Layout() {
    return h(DefaultTheme.Layout, null, {
      'home-hero-actions-after': () => h(InstallCmd),
    })
  },
  setup() {
    const { frontmatter } = useData()
    const route = useRoute()

    giscusTalk(
      {
        repo: 'PlayIdea-Lab/cq',
        repoId: 'R_kgDORVX9iw',
        category: 'Q&A',
        categoryId: 'DIC_kwDORVX9i84C5S_p',
        mapping: 'pathname',
        strict: '0',
        reactionsEnabled: '1',
        emitMetadata: '0',
        inputPosition: 'top',
        theme: 'preferred_color_scheme',
        lang: 'en',
      },
      {
        frontmatter,
        route,
      },
      true, // activate on all pages
    )
  },
}
