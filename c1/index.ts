/**
 * c1/index.ts
 * Entry point — loads and initializes platform adapters.
 * Run: bun run c1/index.ts
 */

import { AdapterRegistry } from "./core/adapter.js";
import { DoorayAdapter } from "./adapters/dooray/index.js";

const registry = new AdapterRegistry();

// Register Dooray adapter
const dooray = new DoorayAdapter();
registry.register(dooray);

console.log(`Registered adapters: ${registry.ids().join(", ")}`);

await registry.initializeAll();
console.log("All adapters initialized.");
console.log(`Dooray webhook listener ready on port ${(dooray.config.options as Record<string, unknown>).listenPort}`);
