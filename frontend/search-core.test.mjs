/**
 * Tests for search-core.js
 */

import { describe, it, assert, assertEqual, assertDeepEqual } from './test-utils.mjs';
import { 
    parseSearchTerm, 
    textMatches, 
    evaluateAST, 
    getSearchRegex,
    hasInversePrefix,
    findAllMatches
} from './search-core.js';

// ============================================================================
// parseSearchTerm tests
// ============================================================================

describe('parseSearchTerm', () => {
    it('returns empty pattern for empty string', () => {
        assertDeepEqual(parseSearchTerm('', false), { pattern: '', isInverse: false });
    });

    it('returns pattern as-is in plain text mode', () => {
        assertDeepEqual(parseSearchTerm('hello', false), { pattern: 'hello', isInverse: false });
    });

    it('does not parse ! prefix in plain text mode', () => {
        assertDeepEqual(parseSearchTerm('!hello', false), { pattern: '!hello', isInverse: false });
    });

    it('parses ! prefix as inverse in regex mode', () => {
        assertDeepEqual(parseSearchTerm('!error', true), { pattern: 'error', isInverse: true });
    });

    it('parses escaped \\! as literal in regex mode', () => {
        assertDeepEqual(parseSearchTerm('\\!important', true), { pattern: 'important', isInverse: false });
    });

    it('handles ! with empty pattern in regex mode', () => {
        assertDeepEqual(parseSearchTerm('!', true), { pattern: '', isInverse: true });
    });

    it('handles regular pattern in regex mode', () => {
        assertDeepEqual(parseSearchTerm('error|warn', true), { pattern: 'error|warn', isInverse: false });
    });
});

// ============================================================================
// textMatches tests - Plain Text Mode
// ============================================================================

describe('textMatches - Plain Text Mode', () => {
    it('returns false for empty search term', () => {
        assertEqual(textMatches('hello world', '', {}), false);
    });

    it('matches substring case-insensitively by default', () => {
        assertEqual(textMatches('Hello World', 'world', {}), true);
        assertEqual(textMatches('Hello World', 'WORLD', {}), true);
    });

    it('matches case-sensitively when enabled', () => {
        assertEqual(textMatches('Hello World', 'World', { caseSensitive: true }), true);
        assertEqual(textMatches('Hello World', 'world', { caseSensitive: true }), false);
    });

    it('returns false when substring not found', () => {
        assertEqual(textMatches('Hello World', 'xyz', {}), false);
    });
});

// ============================================================================
// textMatches tests - Regex Mode
// ============================================================================

describe('textMatches - Regex Mode', () => {
    it('matches regex pattern case-insensitively by default', () => {
        assertEqual(textMatches('ERROR: something failed', 'error', { regex: true }), true);
        assertEqual(textMatches('error: something failed', 'ERROR', { regex: true }), true);
    });

    it('matches regex pattern case-sensitively when enabled', () => {
        assertEqual(textMatches('ERROR: something failed', 'ERROR', { regex: true, caseSensitive: true }), true);
        assertEqual(textMatches('ERROR: something failed', 'error', { regex: true, caseSensitive: true }), false);
    });

    it('supports regex alternation', () => {
        assertEqual(textMatches('This is a warning', 'error|warn', { regex: true }), true);
        assertEqual(textMatches('This is an error', 'error|warn', { regex: true }), true);
        assertEqual(textMatches('This is info', 'error|warn', { regex: true }), false);
    });

    it('supports regex character classes', () => {
        assertEqual(textMatches('Request took 123ms', '\\d+ms', { regex: true }), true);
        assertEqual(textMatches('Request took fast', '\\d+ms', { regex: true }), false);
    });

    it('supports regex anchors', () => {
        assertEqual(textMatches('ERROR: something', '^ERROR', { regex: true }), true);
        assertEqual(textMatches('Something ERROR', '^ERROR', { regex: true }), false);
    });

    it('returns false for invalid regex', () => {
        assertEqual(textMatches('hello world', '[invalid', { regex: true }), false);
    });
});

// ============================================================================
// textMatches tests - Inverse Matching
// ============================================================================

describe('textMatches - Inverse Matching', () => {
    it('inverse matches lines NOT containing pattern', () => {
        assertEqual(textMatches('This is info', '!error', { regex: true }), true);
        assertEqual(textMatches('This is an error', '!error', { regex: true }), false);
    });

    it('inverse with empty pattern matches all', () => {
        assertEqual(textMatches('anything', '!', { regex: true }), true);
        assertEqual(textMatches('', '!', { regex: true }), true);
    });

    it('inverse works with regex patterns', () => {
        assertEqual(textMatches('INFO: all good', '!error|warn', { regex: true }), true);
        assertEqual(textMatches('WARNING: check this', '!error|warn', { regex: true }), false);
        assertEqual(textMatches('ERROR: failed', '!error|warn', { regex: true }), false);
    });

    it('inverse respects case sensitivity', () => {
        assertEqual(textMatches('This has ERROR', '!error', { regex: true, caseSensitive: true }), true);
        assertEqual(textMatches('This has error', '!error', { regex: true, caseSensitive: true }), false);
    });

    it('escaped exclamation searches for literal pattern', () => {
        assertEqual(textMatches('Important', '\\!Important', { regex: true }), true);
        assertEqual(textMatches('Not here', '\\!Important', { regex: true }), false);
    });
});

// ============================================================================
// evaluateAST tests
// ============================================================================

describe('evaluateAST - Pattern Matching', () => {
    it('matches simple pattern case-insensitively by default', () => {
        const ast = { type: 'pattern', pattern: 'error', regex: 'error' };
        assertEqual(evaluateAST(ast, 'ERROR: something happened', false), true);
        assertEqual(evaluateAST(ast, 'This is fine', false), false);
    });

    it('matches case-sensitively when enabled', () => {
        const ast = { type: 'pattern', pattern: 'Error', regex: 'Error' };
        assertEqual(evaluateAST(ast, 'Error: something happened', true), true);
        assertEqual(evaluateAST(ast, 'ERROR: something happened', true), false);
    });

    it('matches with regex special chars escaped', () => {
        const ast = { type: 'pattern', pattern: '[error]', regex: '\\[error\\]' };
        assertEqual(evaluateAST(ast, 'Got [error] in output', false), true);
        assertEqual(evaluateAST(ast, 'Got error in output', false), false);
    });
});

describe('evaluateAST - OR Expressions', () => {
    it('matches if any child matches', () => {
        const ast = {
            type: 'or',
            children: [
                { type: 'pattern', pattern: 'error', regex: 'error' },
                { type: 'pattern', pattern: 'warning', regex: 'warning' }
            ]
        };
        assertEqual(evaluateAST(ast, 'ERROR: critical', false), true);
        assertEqual(evaluateAST(ast, 'WARNING: something', false), true);
        assertEqual(evaluateAST(ast, 'INFO: normal', false), false);
    });

    it('handles empty children', () => {
        const ast = { type: 'or', children: [] };
        assertEqual(evaluateAST(ast, 'anything', false), false);
    });
});

describe('evaluateAST - AND Expressions', () => {
    it('matches only if all children match', () => {
        const ast = {
            type: 'and',
            children: [
                { type: 'pattern', pattern: 'docker', regex: 'docker' },
                { type: 'pattern', pattern: 'error', regex: 'error' }
            ]
        };
        assertEqual(evaluateAST(ast, 'docker container error', false), true);
        assertEqual(evaluateAST(ast, 'docker container started', false), false);
        assertEqual(evaluateAST(ast, 'systemd error', false), false);
    });

    it('handles empty children', () => {
        const ast = { type: 'and', children: [] };
        assertEqual(evaluateAST(ast, 'anything', false), true);
    });
});

describe('evaluateAST - NOT Expressions', () => {
    it('inverts the match result', () => {
        const ast = {
            type: 'not',
            child: { type: 'pattern', pattern: 'debug', regex: 'debug' }
        };
        assertEqual(evaluateAST(ast, 'ERROR: something', false), true);
        assertEqual(evaluateAST(ast, 'DEBUG: tracing', false), false);
    });
});

describe('evaluateAST - Complex Nested Expressions', () => {
    it('(error | warning) & !debug matches correctly', () => {
        const ast = {
            type: 'and',
            children: [
                {
                    type: 'or',
                    children: [
                        { type: 'pattern', pattern: 'error', regex: 'error' },
                        { type: 'pattern', pattern: 'warning', regex: 'warning' }
                    ]
                },
                {
                    type: 'not',
                    child: { type: 'pattern', pattern: 'debug', regex: 'debug' }
                }
            ]
        };
        assertEqual(evaluateAST(ast, 'ERROR: critical failure', false), true);
        assertEqual(evaluateAST(ast, 'WARNING: disk space low', false), true);
        assertEqual(evaluateAST(ast, 'DEBUG: error tracing', false), false);
        assertEqual(evaluateAST(ast, 'INFO: normal operation', false), false);
    });

    it('docker & !health matches non-healthcheck docker lines', () => {
        const ast = {
            type: 'and',
            children: [
                { type: 'pattern', pattern: 'docker', regex: 'docker' },
                {
                    type: 'not',
                    child: { type: 'pattern', pattern: 'health', regex: 'health' }
                }
            ]
        };
        assertEqual(evaluateAST(ast, 'docker: container started', false), true);
        assertEqual(evaluateAST(ast, 'docker: health check passed', false), false);
        assertEqual(evaluateAST(ast, 'systemd: service running', false), false);
    });
});

// ============================================================================
// getSearchRegex tests
// ============================================================================

describe('getSearchRegex', () => {
    it('returns null for empty search term', () => {
        assertEqual(getSearchRegex('', {}), null);
    });

    it('escapes special chars in plain text mode', () => {
        const regex = getSearchRegex('hello.world', {});
        assert(regex !== null, 'regex should not be null');
        assertEqual(regex.test('hello.world'), true);
        assertEqual(regex.test('helloXworld'), false);
    });

    it('does not escape in regex mode', () => {
        let regex = getSearchRegex('hello.world', { regex: true });
        assert(regex !== null, 'regex should not be null');
        assertEqual(regex.test('hello.world'), true);
        
        regex = getSearchRegex('hello.world', { regex: true });
        assertEqual(regex.test('helloXworld'), true);
    });

    it('returns null for invalid regex in regex mode', () => {
        assertEqual(getSearchRegex('[invalid', { regex: true }), null);
    });

    it('uses case-insensitive flag by default', () => {
        const regex = getSearchRegex('hello', {});
        assert(regex !== null, 'regex should not be null');
        assertEqual(regex.flags.includes('i'), true);
    });

    it('uses case-sensitive when enabled', () => {
        const regex = getSearchRegex('hello', { caseSensitive: true });
        assert(regex !== null, 'regex should not be null');
        assertEqual(regex.flags.includes('i'), false);
    });

    it('returns null for bang-and-pipe mode', () => {
        assertEqual(getSearchRegex('error | warn', { bangAndPipe: true }), null);
    });
});

// ============================================================================
// hasInversePrefix tests
// ============================================================================

describe('hasInversePrefix', () => {
    it('returns false when not in regex mode', () => {
        assertEqual(hasInversePrefix('!error', false), false);
    });

    it('returns true for ! prefix in regex mode', () => {
        assertEqual(hasInversePrefix('!error', true), true);
    });

    it('returns false for escaped \\! in regex mode', () => {
        assertEqual(hasInversePrefix('\\!error', true), false);
    });

    it('returns false for empty string', () => {
        assertEqual(hasInversePrefix('', true), false);
    });
});

// ============================================================================
// findAllMatches tests
// ============================================================================

describe('findAllMatches', () => {
    it('returns empty array for empty search term', () => {
        assertDeepEqual(findAllMatches('hello world', '', {}), []);
    });

    it('finds all occurrences in plain text mode', () => {
        const matches = findAllMatches('hello hello hello', 'hello', {});
        assertEqual(matches.length, 3);
        assertEqual(matches[0].position, 0);
        assertEqual(matches[1].position, 6);
        assertEqual(matches[2].position, 12);
    });

    it('respects case sensitivity', () => {
        const insensitive = findAllMatches('Hello HELLO hello', 'hello', {});
        assertEqual(insensitive.length, 3);
        
        const sensitive = findAllMatches('Hello HELLO hello', 'hello', { caseSensitive: true });
        assertEqual(sensitive.length, 1);
        assertEqual(sensitive[0].position, 12);
    });

    it('handles regex patterns', () => {
        const matches = findAllMatches('abc 123 def 456', '\\d+', { regex: true });
        assertEqual(matches.length, 2);
        assertEqual(matches[0].position, 4);
        assertEqual(matches[1].position, 12);
    });
});

// ============================================================================
// Integration Scenarios
// ============================================================================

describe('Integration Scenarios', () => {
    it('filter Docker logs for errors excluding health checks', () => {
        const logLines = [
            'ERROR: Database connection failed',
            'INFO: Health check passed',
            'ERROR: Health check failed',
            'INFO: Request processed',
            'WARN: Slow query detected'
        ];
        
        const errorMatches = logLines.filter(line => textMatches(line, 'error', { regex: true }));
        assertEqual(errorMatches.length, 2);
        
        const nonHealthMatches = logLines.filter(line => textMatches(line, '!health', { regex: true }));
        assertEqual(nonHealthMatches.length, 3);
    });

    it('case-insensitive search across log levels', () => {
        const logLines = [
            'error: something wrong',
            'ERROR: Something Wrong',
            'Error: Mixed case',
            'info: all good'
        ];
        
        const matches = logLines.filter(line => textMatches(line, 'error', { regex: true }));
        assertEqual(matches.length, 3);
    });

    it('regex with numeric patterns', () => {
        const logLines = [
            'Request took 5ms',
            'Request took 150ms',
            'Request took 2500ms',
            'Request completed'
        ];
        
        const slowRequests = logLines.filter(line => textMatches(line, '\\d{3,}ms', { regex: true }));
        assertEqual(slowRequests.length, 2);
    });
});
