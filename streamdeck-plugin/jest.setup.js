// Make Jest globals available in ESM VM-module mode.
// In `node --experimental-vm-modules` mode, `jest` is not auto-injected
// into the module scope even though injectGlobals is true.  Assigning it
// here ensures jest.fn(), jest.mock(), etc. work without explicit imports.
import { jest } from '@jest/globals';
globalThis.jest = jest;
