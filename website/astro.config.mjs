// @ts-check
import cloudflare from '@astrojs/cloudflare'
import tailwindcss from '@tailwindcss/vite'
import lottie from 'astro-integration-lottie'
import { defineConfig } from 'astro/config'

export default defineConfig({
  output: 'static',
  adapter: cloudflare({ imageService: 'compile' }),
  session: false,
  integrations: [
    lottie(),
  ],
  vite: {
    plugins: [
      tailwindcss(),
    ],
  },
})
