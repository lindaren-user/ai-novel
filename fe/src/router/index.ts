import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'home',
      component: () => import('@/pages/HomePage.vue'),
    },
    {
      path: '/workspace',
      name: 'workspace',
      component: () => import('@/pages/WorkspacePage.vue'),
    },
    {
      path: '/share/:type/:token',
      name: 'share-reader',
      component: () => import('@/pages/ShareReaderPage.vue'),
    },
  ],
})

export default router
