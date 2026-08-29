import { defineConfig, mergeConfig, configDefaults } from 'vitest/config'
import viteConfig from './vite.config.ts'

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: 'jsdom',
      setupFiles: ['./src/test/setup.ts'],
      globals: false,
      coverage: {
        provider: 'v8',
        reporter: ['text', 'lcov', 'html'],
        include: ['src/**/*.{ts,tsx}'],
        exclude: [
          ...(configDefaults.coverage?.exclude ?? []),
          'src/main.tsx',
          'src/vite-env.d.ts',
          'src/i18n/i18next.d.ts',
          'src/types/**',
          'src/i18n/locales/**',
          'src/**/index.ts',
          'src/test/**',
        ],
      },
    },
  }),
)
