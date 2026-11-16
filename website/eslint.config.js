import antfu from '@antfu/eslint-config'

export default antfu({
  isInEditor: false,
  astro: true,
  typescript: true,
  stylistic: {
    indent: 2,
  },
  formatters: {
    astro: true,
  },
  ignores: [
    'dist/**',
    '.astro/**',
    '.wrangler/**',
    '.cloudflare/**',
    '**/.env*',
    '**/.dev.vars*',
  ],
})
