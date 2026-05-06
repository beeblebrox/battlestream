export default {
  preset: 'ts-jest/presets/default-esm',
  testEnvironment: 'node',
  testMatch: ['**/src/__tests__/**/*.test.ts'],
  // Override preset: .ts files are compiled as ESM by ts-jest but Jest itself
  // should NOT mark them as ESM via extensionsToTreatAsEsm.  Omitting .ts here
  // means Jest falls back to package.json "type":"module" for .js output, while
  // .ts source files go through loadCjsAsEsm — which honours jest.mock() factories.
  extensionsToTreatAsEsm: ['.tsx', '.mts'],
  transform: {
    '^.+\\.ts$': ['ts-jest', { useESM: true, tsconfig: { module: 'ES2022' } }],
  },
  resolver: './jest.resolver.cjs',
  moduleNameMapper: {
    '^(\\.{1,2}/.*)\\.js$': '$1',
  },
  setupFiles: ['./jest.setup.js'],
};
