import pluginVue from 'eslint-plugin-vue'
import { defineConfigWithVueTs, vueTsConfigs } from '@vue/eslint-config-typescript'
import skipFormattingConfig from '@vue/eslint-config-prettier/skip-formatting'

export default defineConfigWithVueTs(
  {
    ignores: ['node_modules/**', 'dist/**', '.vite/**'],
  },
  pluginVue.configs['flat/recommended'],
  vueTsConfigs.recommendedTypeChecked,
  vueTsConfigs.stylisticTypeChecked,
  {
    files: ['**/*.vue', '**/*.ts'],
    rules: {
      'vue/require-default-prop': 'off',
      '@typescript-eslint/use-unknown-in-catch-callback-variable': 'error',
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
        },
      ],
      'vue/no-unused-vars': [
        'error',
        {
          ignorePattern: '^_',
        },
      ],
    },
  },
  skipFormattingConfig,
)
