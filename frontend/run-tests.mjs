#!/usr/bin/env node
/**
 * Test runner for all JavaScript tests.
 * Imports and runs all test modules, then prints summary.
 * 
 * Usage: node frontend/run-tests.mjs
 */

import { printSummary, exit, resetResults } from './test-utils.mjs';

// Reset before running all tests
resetResults();

console.log('Running JavaScript tests...\n');

// Import all test modules - they run on import
await import('./utils.test.mjs');
await import('./state.test.mjs');
await import('./search-core.test.mjs');
await import('./filter.test.mjs');
await import('./render.test.mjs');
await import('./websocket.test.mjs');
await import('./window-exports.test.mjs');

// Print summary and exit
printSummary();
exit();
