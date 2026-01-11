/**
 * Tests to verify that all onclick handlers in index.html reference
 * functions that are actually exported to window.__dashboard in main.jsx.
 */

import { describe, it, assert } from './test-utils.mjs';
import { readFileSync } from 'fs';
import { dirname, join } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));

/**
 * Extract all function names exposed on window.__dashboard from main.jsx.
 */
function getExportedFunctions() {
    const mainPath = join(__dirname, 'main.jsx');
    const content = readFileSync(mainPath, 'utf-8');
    
    // Match the window.__dashboard = { ... } block
    const dashboardMatch = content.match(/window\.__dashboard\s*=\s*\{([^}]+(?:\{[^}]*\}[^}]*)*)\}/s);
    if (!dashboardMatch) {
        throw new Error('Could not find window.__dashboard assignment in main.jsx');
    }
    
    const block = dashboardMatch[1];
    const functions = new Set();
    
    // Match property names (key: value or shorthand key)
    // Handles: functionName, functionName: ..., functionName: () => ...
    const propRegex = /^\s*(\w+)\s*(?::|,|$)/gm;
    let match;
    while ((match = propRegex.exec(block)) !== null) {
        functions.add(match[1]);
    }
    
    return functions;
}

/**
 * Extract all function calls from onclick handlers in index.html.
 */
function getOnclickHandlers() {
    const htmlPath = join(__dirname, '..', 'static', 'index.html');
    const content = readFileSync(htmlPath, 'utf-8');
    
    const handlers = [];
    
    // Match onclick="..." attributes
    const onclickRegex = /onclick="([^"]+)"/g;
    let match;
    while ((match = onclickRegex.exec(content)) !== null) {
        handlers.push(match[1]);
    }
    
    return handlers;
}

/**
 * Parse a handler string and extract the function name being called.
 * Handles window.__dashboard.functionName(...) patterns.
 */
function extractFunctionName(handler) {
    // Match window.__dashboard.functionName(...)
    const dashboardMatch = handler.match(/window\.__dashboard\.(\w+)\s*\(/);
    if (dashboardMatch) {
        return { name: dashboardMatch[1], hasDashboardPrefix: true };
    }
    
    // Match bare function calls like functionName(...)
    const bareMatch = handler.match(/^(\w+)\s*\(/);
    if (bareMatch) {
        return { name: bareMatch[1], hasDashboardPrefix: false };
    }
    
    return null;
}

describe('Window Exports Validation', () => {
    it('window.__dashboard exports can be parsed from main.jsx', () => {
        const exports = getExportedFunctions();
        assert(exports.size > 0, 'Should find exported functions');
        assert(exports.has('toggleFilter'), 'Should export toggleFilter');
        assert(exports.has('toggleSourceFilter'), 'Should export toggleSourceFilter');
        assert(exports.has('toggleSort'), 'Should export toggleSort');
        assert(exports.has('loadServices'), 'Should export loadServices');
        assert(exports.has('logout'), 'Should export logout');
    });
    
    it('all onclick handlers use window.__dashboard prefix', () => {
        const handlers = getOnclickHandlers();
        assert(handlers.length > 0, 'Should find onclick handlers in HTML');
        
        const missingPrefix = [];
        for (const handler of handlers) {
            const parsed = extractFunctionName(handler);
            if (parsed && !parsed.hasDashboardPrefix) {
                missingPrefix.push(`${parsed.name}() - should be window.__dashboard.${parsed.name}()`);
            }
        }
        
        assert(
            missingPrefix.length === 0,
            `Found onclick handlers without window.__dashboard prefix:\n  ${missingPrefix.join('\n  ')}`
        );
    });
    
    it('all onclick handlers reference exported functions', () => {
        const exports = getExportedFunctions();
        const handlers = getOnclickHandlers();
        
        const notExported = [];
        for (const handler of handlers) {
            const parsed = extractFunctionName(handler);
            if (parsed && !exports.has(parsed.name)) {
                notExported.push(`${parsed.name}() - not found in window.__dashboard exports`);
            }
        }
        
        assert(
            notExported.length === 0,
            `Found onclick handlers referencing non-exported functions:\n  ${notExported.join('\n  ')}`
        );
    });
});

// List of expected exports for documentation and quick reference
describe('Expected Window Exports', () => {
    const expectedExports = [
        'toggleFilter',
        'toggleSourceFilter',
        'toggleHostFilter',
        'toggleSort',
        'toggleLogs',
        'closeLogs',
        'onLogsSearchInput',
        'onLogsSearchKeydown',
        'toggleLogsSearchMode',
        'toggleLogsCaseSensitivity',
        'toggleLogsRegex',
        'toggleLogsBangAndPipe',
        'navigateMatch',
        'onTableSearchInput',
        'onTableSearchKeydown',
        'clearTableSearch',
        'toggleTableCaseSensitivity',
        'toggleTableRegex',
        'toggleTableBangAndPipe',
        'confirmServiceAction',
        'executeServiceAction',
        'showHelpModal',
        'scrollToService',
        'logout',
        'loadServices'
    ];
    
    it('exports all expected functions', () => {
        const exports = getExportedFunctions();
        const missing = expectedExports.filter(fn => !exports.has(fn));
        
        assert(
            missing.length === 0,
            `Missing expected exports:\n  ${missing.join('\n  ')}`
        );
    });
});
