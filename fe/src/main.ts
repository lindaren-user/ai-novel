import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useAppStore } from './stores/app'
import './assets/css/main.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)
if (!window.location.pathname.startsWith('/share/')) {
  useAppStore(pinia).initializeAuth()
}
app.mount('#app')
