import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
  },
  {
    // shadcn/ui primitives are generated and co-export a cva() `*Variants`
    // constant next to the component (e.g. button.tsx → buttonVariants),
    // which trips the Fast Refresh rule. Fast Refresh purity is irrelevant
    // for generated primitives, so scope the rule off for that dir only —
    // it stays strict everywhere else, and survives `pnpm ui` regen.
    files: ['**/components/ui/**'],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
])
