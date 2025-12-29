/**
 * Tests for log search functionality in app.js
 * Run with: node tests/js/search_test.js
 * 
 * These tests verify the regex matching, case sensitivity, and inverse matching
 * features of the log search widget.
 */

// Simulate the global state variables from app.js
let logsSearchRegex = false;
let logsSearchCaseSensitive = false;

// ============================================================================
// Functions extracted from app.js for testing
// ============================================================================

function parseSearchTerm(searchTerm) {
    if (!searchTerm) return { pattern: '', isInverse: false };
    
    // Only support ! prefix in regex mode
    if (!logsSearchRegex) {
        return { pattern: searchTerm, isInverse: false };
    }
    
    // Check for escaped \! at start
    if (searchTerm.startsWith('\\!')) {
        return { pattern: searchTerm.slice(2), isInverse: false };
    }
    
    // Check for ! prefix (inverse match)
    if (searchTerm.startsWith('!')) {
        return { pattern: searchTerm.slice(1), isInverse: true };
    }
    
    return { pattern: searchTerm, isInverse: false };
}

function textMatches(text, searchTerm) {
    if (!searchTerm) return false;
    
    const { pattern, isInverse } = parseSearchTerm(searchTerm);
    if (!pattern) return isInverse; // Empty pattern after ! means match all (inverse of nothing)
    
    let matches;
    if (logsSearchRegex) {
        try {
            const flags = logsSearchCaseSensitive ? '' : 'i';
            const regex = new RegExp(pattern, flags);
            matches = regex.test(text);
        } catch (e) {
            // Invalid regex - no match
            return false;
        }
    } else {
        if (logsSearchCaseSensitive) {
            matches = text.includes(pattern);
        } else {
            matches = text.toLowerCase().includes(pattern.toLowerCase());
        }
    }
    
    return isInverse ? !matches : matches;
}

function getSearchRegex(searchTerm) {
    if (!searchTerm) return null;
    
    const { pattern } = parseSearchTerm(searchTerm);
    if (!pattern) return null;
    
    try {
        const flags = logsSearchCaseSensitive ? 'g' : 'gi';
        if (logsSearchRegex) {
            return new RegExp(`(${pattern})`, flags);
        } else {
            const escapedTerm = pattern.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
            return new RegExp(`(${escapedTerm})`, flags);
        }
    } catch (e) {
        return null;
    }
}

// ============================================================================
// Test Framework
// ============================================================================

let testsPassed = 0;
let testsFailed = 0;
const failures = [];

function assert(condition, message) {
    if (!condition) {
        throw new Error(message || 'Assertion failed');
    }
}

function assertEqual(actual, expected, message) {
    if (actual !== expected) {
        throw new Error(`${message || 'Assertion failed'}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
    }
}

function assertDeepEqual(actual, expected, message) {
    const actualStr = JSON.stringify(actual);
    const expectedStr = JSON.stringify(expected);
    if (actualStr !== expectedStr) {
        throw new Error(`${message || 'Assertion failed'}: expected ${expectedStr}, got ${actualStr}`);
    }
}

function test(name, fn) {
    // Reset state before each test
    logsSearchRegex = false;
    logsSearchCaseSensitive = false;
    
    try {
        fn();
        testsPassed++;
        console.log(`  ✓ ${name}`);
    } catch (e) {
        testsFailed++;
        failures.push({ name, error: e.message });
        console.log(`  ✗ ${name}`);
        console.log(`    ${e.message}`);
    }
}

function describe(suiteName, fn) {
    console.log(`\n${suiteName}`);
    fn();
}

// ============================================================================
// Tests
// ============================================================================

describe('parseSearchTerm', () => {
    test('returns empty pattern for empty string', () => {
        assertDeepEqual(parseSearchTerm(''), { pattern: '', isInverse: false });
    });

    test('returns pattern as-is in plain text mode', () => {
        logsSearchRegex = false;
        assertDeepEqual(parseSearchTerm('hello'), { pattern: 'hello', isInverse: false });
    });

    test('does not parse ! prefix in plain text mode', () => {
        logsSearchRegex = false;
        assertDeepEqual(parseSearchTerm('!hello'), { pattern: '!hello', isInverse: false });
    });

    test('parses ! prefix as inverse in regex mode', () => {
        logsSearchRegex = true;
        assertDeepEqual(parseSearchTerm('!error'), { pattern: 'error', isInverse: true });
    });

    test('parses escaped \\! as literal in regex mode', () => {
        logsSearchRegex = true;
        assertDeepEqual(parseSearchTerm('\\!important'), { pattern: 'important', isInverse: false });
    });

    test('handles ! with empty pattern in regex mode', () => {
        logsSearchRegex = true;
        assertDeepEqual(parseSearchTerm('!'), { pattern: '', isInverse: true });
    });

    test('handles regular pattern in regex mode', () => {
        logsSearchRegex = true;
        assertDeepEqual(parseSearchTerm('error|warn'), { pattern: 'error|warn', isInverse: false });
    });
});

describe('textMatches - Plain Text Mode', () => {
    test('returns false for empty search term', () => {
        assertEqual(textMatches('hello world', ''), false);
    });

    test('matches substring case-insensitively by default', () => {
        logsSearchCaseSensitive = false;
        assertEqual(textMatches('Hello World', 'world'), true);
        assertEqual(textMatches('Hello World', 'WORLD'), true);
    });

    test('matches case-sensitively when enabled', () => {
        logsSearchCaseSensitive = true;
        assertEqual(textMatches('Hello World', 'World'), true);
        assertEqual(textMatches('Hello World', 'world'), false);
    });

    test('returns false when substring not found', () => {
        assertEqual(textMatches('Hello World', 'xyz'), false);
    });
});

describe('textMatches - Regex Mode', () => {
    test('matches regex pattern case-insensitively by default', () => {
        logsSearchRegex = true;
        assertEqual(textMatches('ERROR: something failed', 'error'), true);
        assertEqual(textMatches('error: something failed', 'ERROR'), true);
    });

    test('matches regex pattern case-sensitively when enabled', () => {
        logsSearchRegex = true;
        logsSearchCaseSensitive = true;
        assertEqual(textMatches('ERROR: something failed', 'ERROR'), true);
        assertEqual(textMatches('ERROR: something failed', 'error'), false);
    });

    test('supports regex alternation', () => {
        logsSearchRegex = true;
        assertEqual(textMatches('This is a warning', 'error|warn'), true);
        assertEqual(textMatches('This is an error', 'error|warn'), true);
        assertEqual(textMatches('This is info', 'error|warn'), false);
    });

    test('supports regex character classes', () => {
        logsSearchRegex = true;
        assertEqual(textMatches('Request took 123ms', '\\d+ms'), true);
        assertEqual(textMatches('Request took fast', '\\d+ms'), false);
    });

    test('supports regex anchors', () => {
        logsSearchRegex = true;
        assertEqual(textMatches('ERROR: something', '^ERROR'), true);
        assertEqual(textMatches('Something ERROR', '^ERROR'), false);
    });

    test('returns false for invalid regex', () => {
        logsSearchRegex = true;
        assertEqual(textMatches('hello world', '[invalid'), false);
    });
});

describe('textMatches - Inverse Matching', () => {
    test('inverse matches lines NOT containing pattern', () => {
        logsSearchRegex = true;
        assertEqual(textMatches('This is info', '!error'), true);
        assertEqual(textMatches('This is an error', '!error'), false);
    });

    test('inverse with empty pattern matches all', () => {
        logsSearchRegex = true;
        assertEqual(textMatches('anything', '!'), true);
        assertEqual(textMatches('', '!'), true);
    });

    test('inverse works with regex patterns', () => {
        logsSearchRegex = true;
        assertEqual(textMatches('INFO: all good', '!error|warn'), true);
        assertEqual(textMatches('WARNING: check this', '!error|warn'), false);
        assertEqual(textMatches('ERROR: failed', '!error|warn'), false);
    });

    test('inverse respects case sensitivity', () => {
        logsSearchRegex = true;
        logsSearchCaseSensitive = true;
        assertEqual(textMatches('This has ERROR', '!error'), true); // ERROR != error
        assertEqual(textMatches('This has error', '!error'), false);
    });

    test('escaped exclamation searches for literal pattern', () => {
        logsSearchRegex = true;
        // \! means "don't treat ! as inverse, search for the pattern after it"
        // So \!Important searches for "Important" (the ! is just escaped/removed)
        assertEqual(textMatches('Important', '\\!Important'), true);
        assertEqual(textMatches('Not here', '\\!Important'), false);
    });
});

describe('getSearchRegex', () => {
    test('returns null for empty search term', () => {
        assertEqual(getSearchRegex(''), null);
    });

    test('escapes special chars in plain text mode', () => {
        logsSearchRegex = false;
        const regex = getSearchRegex('hello.world');
        assert(regex !== null, 'regex should not be null');
        assertEqual(regex.test('hello.world'), true);
        assertEqual(regex.test('helloXworld'), false); // dot should not match any char
    });

    test('does not escape in regex mode', () => {
        logsSearchRegex = true;
        // Each call to getSearchRegex creates a fresh regex, avoiding lastIndex issues
        let regex = getSearchRegex('hello.world');
        assert(regex !== null, 'regex should not be null');
        assertEqual(regex.test('hello.world'), true);
        
        // Get fresh regex for next test (global flag maintains lastIndex state)
        regex = getSearchRegex('hello.world');
        assertEqual(regex.test('helloXworld'), true); // dot matches any char
        
        regex = getSearchRegex('hello.world');
        assertEqual(regex.test('hello world'), true); // dot matches space too
    });

    test('returns null for invalid regex in regex mode', () => {
        logsSearchRegex = true;
        assertEqual(getSearchRegex('[invalid'), null);
    });

    test('uses case-insensitive flag by default', () => {
        const regex = getSearchRegex('hello');
        assert(regex !== null, 'regex should not be null');
        assertEqual(regex.flags.includes('i'), true);
    });

    test('uses case-sensitive when enabled', () => {
        logsSearchCaseSensitive = true;
        const regex = getSearchRegex('hello');
        assert(regex !== null, 'regex should not be null');
        assertEqual(regex.flags.includes('i'), false);
    });

    test('strips ! prefix for inverse patterns in regex mode', () => {
        logsSearchRegex = true;
        const regex = getSearchRegex('!error');
        assert(regex !== null, 'regex should not be null');
        assertEqual(regex.test('error'), true); // The regex itself matches 'error'
    });

    test('returns null for inverse with empty pattern', () => {
        logsSearchRegex = true;
        assertEqual(getSearchRegex('!'), null);
    });
});

describe('Integration Scenarios', () => {
    test('filter Docker logs for errors excluding health checks', () => {
        logsSearchRegex = true;
        // Simulating: want to see errors but not health check errors
        // Using inverse on health, then checking for error separately
        const logLines = [
            'ERROR: Database connection failed',
            'INFO: Health check passed',
            'ERROR: Health check failed',
            'INFO: Request processed',
            'WARN: Slow query detected'
        ];
        
        // Pattern: error
        const errorMatches = logLines.filter(line => textMatches(line, 'error'));
        assertEqual(errorMatches.length, 2);
        
        // Pattern: !health (lines without "health")  
        const nonHealthMatches = logLines.filter(line => textMatches(line, '!health'));
        assertEqual(nonHealthMatches.length, 3);
    });

    test('case-insensitive search across log levels', () => {
        logsSearchRegex = true;
        const logLines = [
            'error: something wrong',
            'ERROR: Something Wrong',
            'Error: Mixed case',
            'info: all good'
        ];
        
        const matches = logLines.filter(line => textMatches(line, 'error'));
        assertEqual(matches.length, 3);
    });

    test('regex with numeric patterns', () => {
        logsSearchRegex = true;
        const logLines = [
            'Request took 5ms',
            'Request took 150ms',
            'Request took 2500ms',
            'Request completed'
        ];
        
        // Find requests taking 100ms or more (3+ digits before ms)
        const slowRequests = logLines.filter(line => textMatches(line, '\\d{3,}ms'));
        assertEqual(slowRequests.length, 2);
    });
});

// ============================================================================
// Bang & Pipe AST Evaluation Tests
// ============================================================================

// evaluateAST function from app.js
function evaluateAST(ast, text) {
    if (!ast) return false;
    switch (ast.type) {
        case 'pattern':
            const flags = logsSearchCaseSensitive ? '' : 'i';
            const regex = new RegExp(ast.regex, flags);
            return regex.test(text);
        case 'or':
            return ast.children.some(child => evaluateAST(child, text));
        case 'and':
            return ast.children.every(child => evaluateAST(child, text));
        case 'not':
            return !evaluateAST(ast.child, text);
        default:
            return false;
    }
}

describe('evaluateAST - Pattern Matching', () => {
    test('matches simple pattern case-insensitively by default', () => {
        const ast = { type: 'pattern', pattern: 'error', regex: 'error' };
        assertEqual(evaluateAST(ast, 'ERROR: something happened'), true);
        assertEqual(evaluateAST(ast, 'This is fine'), false);
    });

    test('matches case-sensitively when enabled', () => {
        logsSearchCaseSensitive = true;
        const ast = { type: 'pattern', pattern: 'Error', regex: 'Error' };
        assertEqual(evaluateAST(ast, 'Error: something happened'), true);
        assertEqual(evaluateAST(ast, 'ERROR: something happened'), false);
    });

    test('matches with regex special chars escaped', () => {
        const ast = { type: 'pattern', pattern: '[error]', regex: '\\[error\\]' };
        assertEqual(evaluateAST(ast, 'Got [error] in output'), true);
        assertEqual(evaluateAST(ast, 'Got error in output'), false);
    });
});

describe('evaluateAST - OR Expressions', () => {
    test('matches if any child matches', () => {
        const ast = {
            type: 'or',
            children: [
                { type: 'pattern', pattern: 'error', regex: 'error' },
                { type: 'pattern', pattern: 'warning', regex: 'warning' }
            ]
        };
        assertEqual(evaluateAST(ast, 'ERROR: critical'), true);
        assertEqual(evaluateAST(ast, 'WARNING: something'), true);
        assertEqual(evaluateAST(ast, 'INFO: normal'), false);
    });

    test('handles empty children', () => {
        const ast = { type: 'or', children: [] };
        assertEqual(evaluateAST(ast, 'anything'), false);
    });
});

describe('evaluateAST - AND Expressions', () => {
    test('matches only if all children match', () => {
        const ast = {
            type: 'and',
            children: [
                { type: 'pattern', pattern: 'docker', regex: 'docker' },
                { type: 'pattern', pattern: 'error', regex: 'error' }
            ]
        };
        assertEqual(evaluateAST(ast, 'docker container error'), true);
        assertEqual(evaluateAST(ast, 'docker container started'), false);
        assertEqual(evaluateAST(ast, 'systemd error'), false);
    });

    test('handles empty children', () => {
        const ast = { type: 'and', children: [] };
        assertEqual(evaluateAST(ast, 'anything'), true); // empty AND is true
    });
});

describe('evaluateAST - NOT Expressions', () => {
    test('inverts the match result', () => {
        const ast = {
            type: 'not',
            child: { type: 'pattern', pattern: 'debug', regex: 'debug' }
        };
        assertEqual(evaluateAST(ast, 'ERROR: something'), true);
        assertEqual(evaluateAST(ast, 'DEBUG: tracing'), false);
    });
});

describe('evaluateAST - Complex Nested Expressions', () => {
    test('(error | warning) & !debug matches correctly', () => {
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
        assertEqual(evaluateAST(ast, 'ERROR: critical failure'), true);
        assertEqual(evaluateAST(ast, 'WARNING: disk space low'), true);
        assertEqual(evaluateAST(ast, 'DEBUG: error tracing'), false);  // has debug
        assertEqual(evaluateAST(ast, 'INFO: normal operation'), false); // no error/warning
    });

    test('docker & !health matches non-healthcheck docker lines', () => {
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
        const logLines = [
            'docker container started',
            'docker healthcheck passed',
            'docker container error',
            'systemd unit started'
        ];
        const filtered = logLines.filter(line => evaluateAST(ast, line));
        assertEqual(filtered.length, 2);
        assertEqual(filtered[0], 'docker container started');
        assertEqual(filtered[1], 'docker container error');
    });

    test('deeply nested expression', () => {
        // ((a & b) | (c & d)) & !e
        const ast = {
            type: 'and',
            children: [
                {
                    type: 'or',
                    children: [
                        {
                            type: 'and',
                            children: [
                                { type: 'pattern', pattern: 'a', regex: 'a' },
                                { type: 'pattern', pattern: 'b', regex: 'b' }
                            ]
                        },
                        {
                            type: 'and',
                            children: [
                                { type: 'pattern', pattern: 'c', regex: 'c' },
                                { type: 'pattern', pattern: 'd', regex: 'd' }
                            ]
                        }
                    ]
                },
                {
                    type: 'not',
                    child: { type: 'pattern', pattern: 'e', regex: 'e' }
                }
            ]
        };
        assertEqual(evaluateAST(ast, 'ab'), true);   // a&b matches
        assertEqual(evaluateAST(ast, 'cd'), true);   // c&d matches
        assertEqual(evaluateAST(ast, 'abe'), false); // has e
        assertEqual(evaluateAST(ast, 'ac'), false);  // neither a&b nor c&d
    });
});

describe('evaluateAST - Edge Cases', () => {
    test('returns false for null AST', () => {
        assertEqual(evaluateAST(null, 'anything'), false);
    });

    test('returns false for undefined AST', () => {
        assertEqual(evaluateAST(undefined, 'anything'), false);
    });

    test('returns false for unknown node type', () => {
        const ast = { type: 'unknown' };
        assertEqual(evaluateAST(ast, 'anything'), false);
    });
});

// ============================================================================
// Run tests and report
// ============================================================================

console.log('\n========================================');
console.log('Log Search Tests');
console.log('========================================');

// Tests are run when describe() is called above

console.log('\n========================================');
console.log(`Results: ${testsPassed} passed, ${testsFailed} failed`);
console.log('========================================\n');

if (testsFailed > 0) {
    console.log('Failed tests:');
    failures.forEach(f => {
        console.log(`  - ${f.name}: ${f.error}`);
    });
    process.exit(1);
}

process.exit(0);
