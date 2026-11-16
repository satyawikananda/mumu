// @ts-check
import cloudflare from '@astrojs/cloudflare'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'astro/config'

export default defineConfig({
  output: 'static',
  adapter: cloudflare({ imageService: 'compile' }),
  session: false,
  vite: {
    plugins: [
      tailwindcss(),
    ],
  },
})
