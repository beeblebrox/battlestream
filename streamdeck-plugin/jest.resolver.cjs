// Custom Jest resolver for battlestream streamdeck plugin.
// Problem: jest.mock('../../foo.js') gets mapped to '../../foo' by moduleNameMapper,
// but resolveStubModuleNameAsync uses project root as basedir, so '../../foo' can't be found.
// Solution: when we see a relative path that doesn't resolve normally, try resolving
// it from src/ subdirectories.
const path = require('path');
const fs = require('fs');

const ROOT = path.resolve(__dirname);
const SRC = path.join(ROOT, 'src');

// Potential test-file basedirs for relative mock path resolution
const SEARCH_BASES = [
  SRC,
  path.join(SRC, '__tests__'),
  path.join(SRC, '__tests__', 'actions'),
];

module.exports = function resolver(moduleName, options) {
  const { defaultResolver, basedir, extensions } = options;

  // First, always try the default resolution (handles normal imports)
  try {
    return defaultResolver(moduleName, options);
  } catch {
    // fall through to custom logic
  }

  const extname = path.extname(moduleName);
  const isRelative = moduleName.startsWith('./') || moduleName.startsWith('../');

  if (isRelative) {
    const extsToTry = extname
      ? [extname]
      : (extensions || ['.ts', '.js', '.tsx', '.jsx']);

    for (const base of SEARCH_BASES) {
      for (const ext of extsToTry) {
        const candidate = path.resolve(base, moduleName) + (extname ? '' : ext);
        if (fs.existsSync(candidate)) {
          // Return via defaultResolver so ts-jest can transform it
          try {
            return defaultResolver(candidate, { ...options, basedir: base });
          } catch {
            return candidate;
          }
        }
      }
      // Also try without adding extension (for bare paths)
      if (!extname) {
        const candidate = path.resolve(base, moduleName + '.ts');
        if (fs.existsSync(candidate)) {
          try {
            return defaultResolver(candidate, { ...options, basedir: base });
          } catch {
            return candidate;
          }
        }
      }
    }
  }

  // If .js path, try .ts
  if (moduleName.endsWith('.js')) {
    const tsPath = moduleName.slice(0, -3) + '.ts';
    try {
      return defaultResolver(tsPath, options);
    } catch {
      // fall through
    }
  }

  throw new Error(`Cannot resolve module '${moduleName}' from '${basedir}'`);
};
