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
    'src/assets/lottie/**',
    '**/.env*',
    '**/.dev.vars*',
  ],
}, {
  files: ['tests/**/*.test.ts'],
  rules: {
    'test/no-import-node-test': 'off',
  },
})
