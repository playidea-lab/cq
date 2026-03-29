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
  </Layout>
</template>
