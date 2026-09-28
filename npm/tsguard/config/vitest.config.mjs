import { defineConfig } from 'vitest/config';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
export default defineConfig({
  root: process.cwd(),
  resolve: { alias: [{ find: /^vitest$/, replacement: fileURLToPath(import.meta.resolve('vitest')) }] },
  test: {
    globals: true,
    coverage: {
      provider: 'custom',
      customProviderModule: require.resolve('@vitest/coverage-v8'),
      include: ['**/*.{ts,tsx,js,jsx}'],
      exclude: ['**/node_modules/**', '**/*.test.*', '**/*.spec.*', '**/coverage/**', '**/dist/**', '**/*.config.*'],
    },
  },
});
