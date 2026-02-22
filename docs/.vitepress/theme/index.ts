import DefaultTheme from 'vitepress/theme'
import { h } from 'vue'
import InstallCmd from './InstallCmd.vue'

export default {
  extends: DefaultTheme,
  Layout() {
    return h(DefaultTheme.Layout, null, {
      'home-hero-actions-after': () => h(InstallCmd),
    })
  },
}
