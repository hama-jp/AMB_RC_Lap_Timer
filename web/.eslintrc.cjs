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
    ecmaFeatures: { jsx: true },
  },
  plugins: ['@typescript-eslint', 'react', 'react-hooks'],
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended-type-checked',
    'plugin:react/recommended',
    'plugin:react/jsx-runtime',
    'plugin:react-hooks/recommended',
  ],
  settings: { react: { version: 'detect' } },
  ignorePatterns: [
    'dist/',
    'node_modules/',
    '.eslintrc.cjs',
    'postcss.config.cjs',
    'tailwind.config.js',
  ],
  rules: {
    '@typescript-eslint/consistent-type-imports': 'error',
    '@typescript-eslint/no-unused-vars': [
      'error',
      { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
    ],
    'no-restricted-imports': [
      'error',
      {
        // Production app/protocol code must run in browsers; tests may freely
        // import node:* via the override below.
        patterns: [{ group: ['node:*'], message: 'Node-only imports are not allowed in src/' }],
      },
    ],
  },
  overrides: [
    {
      files: ['tests/**/*.{ts,tsx}', 'scripts/**/*.ts', 'vite.config.ts', 'tailwind.config.js'],
      rules: { 'no-restricted-imports': 'off' },
    },
  ],
};
