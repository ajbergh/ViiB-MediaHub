import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: { environment: 'node', exclude: ['**/node_modules/**', '**/.worktrees/**', '**/dist/**', '**/build/**'], include: ['**/*.test.ts', '**/*.test.tsx'] },
});
