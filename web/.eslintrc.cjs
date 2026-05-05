/* eslint-env node */
module.exports = {
  root: true,
  env: { browser: true, es2022: true, node: true },
  parser: '@typescript-eslint/parser',
  parserOptions: {
    ecmaVersion: 2022,
    sourceType: 'module',
    project: ['./tsconfig.json'],
    tsconfigRootDir: __dirname,
  },
  plugins: ['@typescript-eslint'],
  extends: ['eslint:recommended', 'plugin:@typescript-eslint/recommended-type-checked'],
  ignorePatterns: ['dist/', 'node_modules/', '.eslintrc.cjs'],
  rules: {
    '@typescript-eslint/consistent-type-imports': 'error',
    '@typescript-eslint/no-unused-vars': [
      'error',
      { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
    ],
    'no-restricted-imports': [
      'error',
      {
        // Production parser code must run in browsers; tests may freely import
        // node:* via the override below.
        patterns: [{ group: ['node:*'], message: 'Node-only imports are not allowed in src/' }],
      },
    ],
  },
  overrides: [
    {
      files: ['tests/**/*.ts', 'scripts/**/*.ts', 'vite.config.ts'],
      rules: { 'no-restricted-imports': 'off' },
    },
  ],
};
