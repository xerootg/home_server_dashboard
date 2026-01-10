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
// Table Search Functions (extracted from app.js for testing)
// ============================================================================

let tableSearchCaseSensitive = false;
let tableSearchRegex = false;
let tableSearchBangAndPipe = false;
let tableBangAndPipeAST = null;
let tableSearchError = '';

function tableTextMatches(text, searchTerm) {
    if (!searchTerm) return true;
    
    // Bang-and-pipe mode: use AST evaluation
    if (tableSearchBangAndPipe) {
        if (!tableBangAndPipeAST) return false;
        return evaluateAST(tableBangAndPipeAST, text);
    }
    
    // Regex mode with ! prefix for inverse
    if (tableSearchRegex && searchTerm.startsWith('!') && !searchTerm.startsWith('\\!')) {
        const pattern = searchTerm.slice(1);
        if (!pattern) return true;
        try {
            const flags = tableSearchCaseSensitive ? '' : 'i';
            const regex = new RegExp(pattern, flags);
            return !regex.test(text);
        } catch (e) {
            tableSearchError = 'Invalid regex: ' + e.message;
            return false;
        }
    }
    
    // Regex mode with escaped \!
    let effectivePattern = searchTerm;
    if (tableSearchRegex && searchTerm.startsWith('\\!')) {
        effectivePattern = searchTerm.slice(2);
    }
    
    // Standard regex mode
    if (tableSearchRegex) {
        try {
            const flags = tableSearchCaseSensitive ? '' : 'i';
            const regex = new RegExp(effectivePattern, flags);
            return regex.test(text);
        } catch (e) {
            tableSearchError = 'Invalid regex: ' + e.message;
            return false;
        }
    }
    
    // Plain text mode
    if (tableSearchCaseSensitive) {
        return text.includes(searchTerm);
    }
    return text.toLowerCase().includes(searchTerm.toLowerCase());
}

function serviceMatchesTableSearch(service, searchTerm) {
    if (!searchTerm) return true;
    
    const searchableText = [
        service.name || '',
        service.project || '',
        service.host || '',
        service.container_name || '',
        service.status || '',
        service.state || '',
        service.image || '',
        service.source || '',
        // Include ports for searching by port number
        ...(service.ports || []).map(p => String(p.host_port)),
        // Include traefik URLs for searching by hostname
        ...(service.traefik_urls || []).map(url => {
            try {
                return new URL(url).hostname;
            } catch (e) {
                return url;
            }
        })
    ].join(' ');
    
    return tableTextMatches(searchableText, searchTerm);
}

// Reset table search state before each test suite
function resetTableSearchState() {
    tableSearchCaseSensitive = false;
    tableSearchRegex = false;
    tableSearchBangAndPipe = false;
    tableBangAndPipeAST = null;
    tableSearchError = '';
}

// ============================================================================
// Table Search Tests
// ============================================================================

describe('Table Search - Plain Text', () => {
    test('empty search matches everything', () => {
        resetTableSearchState();
        assertEqual(tableTextMatches('any text', ''), true);
    });

    test('case insensitive by default', () => {
        resetTableSearchState();
        assertEqual(tableTextMatches('Docker Container', 'docker'), true);
        assertEqual(tableTextMatches('DOCKER', 'docker'), true);
        assertEqual(tableTextMatches('docker', 'DOCKER'), true);
    });

    test('case sensitive when enabled', () => {
        resetTableSearchState();
        tableSearchCaseSensitive = true;
        assertEqual(tableTextMatches('Docker Container', 'docker'), false);
        assertEqual(tableTextMatches('Docker Container', 'Docker'), true);
    });

    test('partial matches work', () => {
        resetTableSearchState();
        assertEqual(tableTextMatches('my-container-name', 'container'), true);
        assertEqual(tableTextMatches('nginx:latest', 'nginx'), true);
    });

    test('no match returns false', () => {
        resetTableSearchState();
        assertEqual(tableTextMatches('docker container', 'systemd'), false);
    });
});

describe('Table Search - Regex Mode', () => {
    test('basic regex pattern', () => {
        resetTableSearchState();
        tableSearchRegex = true;
        assertEqual(tableTextMatches('container-123', 'container-\\d+'), true);
        assertEqual(tableTextMatches('container-abc', 'container-\\d+'), false);
    });

    test('regex with anchors', () => {
        resetTableSearchState();
        tableSearchRegex = true;
        assertEqual(tableTextMatches('nginx', '^nginx$'), true);
        assertEqual(tableTextMatches('my-nginx', '^nginx$'), false);
    });

    test('regex alternation', () => {
        resetTableSearchState();
        tableSearchRegex = true;
        assertEqual(tableTextMatches('docker', 'docker|systemd'), true);
        assertEqual(tableTextMatches('systemd', 'docker|systemd'), true);
        assertEqual(tableTextMatches('podman', 'docker|systemd'), false);
    });

    test('inverse regex with ! prefix', () => {
        resetTableSearchState();
        tableSearchRegex = true;
        assertEqual(tableTextMatches('systemd', '!docker'), true);
        assertEqual(tableTextMatches('docker', '!docker'), false);
    });

    test('escaped ! at start matches literal !', () => {
        resetTableSearchState();
        tableSearchRegex = true;
        // \! escapes the ! prefix - pattern becomes "important", which matches "important"
        assertEqual(tableTextMatches('!important', '\\!important'), true);
        // Text without "important" should not match when pattern is "important"
        assertEqual(tableTextMatches('!other', '\\!important'), false);
    });

    test('invalid regex returns false', () => {
        resetTableSearchState();
        tableSearchRegex = true;
        assertEqual(tableTextMatches('text', '[invalid'), false);
    });
});

describe('Table Search - Bang & Pipe Mode', () => {
    test('simple pattern with AST', () => {
        resetTableSearchState();
        tableSearchBangAndPipe = true;
        tableBangAndPipeAST = { type: 'pattern', pattern: 'docker', regex: 'docker' };
        assertEqual(tableTextMatches('docker container', 'docker'), true);
        assertEqual(tableTextMatches('systemd unit', 'docker'), false);
    });

    test('OR expression', () => {
        resetTableSearchState();
        tableSearchBangAndPipe = true;
        tableBangAndPipeAST = {
            type: 'or',
            children: [
                { type: 'pattern', pattern: 'docker', regex: 'docker' },
                { type: 'pattern', pattern: 'systemd', regex: 'systemd' }
            ]
        };
        assertEqual(tableTextMatches('docker', 'ignored'), true);
        assertEqual(tableTextMatches('systemd', 'ignored'), true);
        assertEqual(tableTextMatches('podman', 'ignored'), false);
    });

    test('AND expression', () => {
        resetTableSearchState();
        tableSearchBangAndPipe = true;
        tableBangAndPipeAST = {
            type: 'and',
            children: [
                { type: 'pattern', pattern: 'docker', regex: 'docker' },
                { type: 'pattern', pattern: 'running', regex: 'running' }
            ]
        };
        assertEqual(tableTextMatches('docker running', 'ignored'), true);
        assertEqual(tableTextMatches('docker stopped', 'ignored'), false);
        assertEqual(tableTextMatches('systemd running', 'ignored'), false);
    });

    test('NOT expression', () => {
        resetTableSearchState();
        tableSearchBangAndPipe = true;
        tableBangAndPipeAST = {
            type: 'not',
            child: { type: 'pattern', pattern: 'stopped', regex: 'stopped' }
        };
        assertEqual(tableTextMatches('running', 'ignored'), true);
        assertEqual(tableTextMatches('stopped', 'ignored'), false);
    });

    test('returns false without valid AST', () => {
        resetTableSearchState();
        tableSearchBangAndPipe = true;
        tableBangAndPipeAST = null;
        assertEqual(tableTextMatches('docker', 'docker'), false);
    });
});

describe('Table Search - Service Matching', () => {
    const sampleService = {
        name: 'nginx',
        project: 'web-stack',
        host: 'nas',
        container_name: 'web-nginx-1',
        status: 'running (healthy)',
        state: 'running',
        image: 'nginx:1.25-alpine',
        source: 'docker',
        ports: [
            { host_port: 8080, container_port: 80, protocol: 'tcp' },
            { host_port: 8443, container_port: 443, protocol: 'tcp' }
        ],
        traefik_urls: [
            'https://nginx.example.com',
            'https://web.mysite.org'
        ]
    };

    test('matches service name', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, 'nginx'), true);
    });

    test('matches project', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, 'web-stack'), true);
    });

    test('matches host', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, 'nas'), true);
    });

    test('matches container name', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, 'web-nginx-1'), true);
    });

    test('matches status', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, 'healthy'), true);
    });

    test('matches state', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, 'running'), true);
    });

    test('matches image', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, 'alpine'), true);
    });

    test('matches source', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, 'docker'), true);
    });

    test('matches port number', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, '8080'), true);
        assertEqual(serviceMatchesTableSearch(sampleService, '8443'), true);
    });

    test('matches traefik hostname', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, 'nginx.example.com'), true);
        assertEqual(serviceMatchesTableSearch(sampleService, 'web.mysite.org'), true);
    });

    test('matches partial traefik hostname', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, 'example.com'), true);
        assertEqual(serviceMatchesTableSearch(sampleService, 'mysite'), true);
    });

    test('no match returns false', () => {
        resetTableSearchState();
        assertEqual(serviceMatchesTableSearch(sampleService, 'postgresql'), false);
    });

    test('handles missing fields gracefully', () => {
        resetTableSearchState();
        const minimalService = { name: 'test' };
        assertEqual(serviceMatchesTableSearch(minimalService, 'test'), true);
        assertEqual(serviceMatchesTableSearch(minimalService, 'missing'), false);
    });

    test('regex matching on services', () => {
        resetTableSearchState();
        tableSearchRegex = true;
        assertEqual(serviceMatchesTableSearch(sampleService, 'nginx.*alpine'), true);
        assertEqual(serviceMatchesTableSearch(sampleService, '^nas$'), false); // Not anchored to start
    });

    test('bang and pipe on services', () => {
        resetTableSearchState();
        tableSearchBangAndPipe = true;
        tableBangAndPipeAST = {
            type: 'and',
            children: [
                { type: 'pattern', pattern: 'docker', regex: 'docker' },
                { type: 'pattern', pattern: 'running', regex: 'running' }
            ]
        };
        assertEqual(serviceMatchesTableSearch(sampleService, 'docker & running'), true);
        
        tableBangAndPipeAST = {
            type: 'and',
            children: [
                { type: 'pattern', pattern: 'systemd', regex: 'systemd' },
                { type: 'pattern', pattern: 'running', regex: 'running' }
            ]
        };
        assertEqual(serviceMatchesTableSearch(sampleService, 'systemd & running'), false);
    });
});

// ============================================================================
// Port Navigation Functions (extracted from app.js for testing)
// ============================================================================

// Simulated global state for allServices
let allServices = [];

// Simulated escapeHtml function
function escapeHtml(text) {
    const div = { textContent: '', innerHTML: '' };
    div.textContent = text;
    // Simple escape for testing - in browser this uses DOM
    return String(text)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

// Find a service's host_ip by name and host
function getServiceHostIP(serviceName, host) {
    const service = allServices.find(s => s.name === serviceName && s.host === host);
    return service ? service.host_ip : null;
}

// Simulated renderPorts function (core logic extracted for testing)
function renderPortData(ports, hostIP, currentService) {
    if (!ports || ports.length === 0) {
        return [];
    }
    const targetHost = hostIP || 'localhost';
    const currentHost = currentService ? currentService.host : '';
    
    return ports
        .filter(port => !port.hidden)
        .map(port => {
            let type; // 'link', 'scroll', 'source-link'
            let displayText;
            let titleText;
            let url = null;
            let scrollTarget = null;
            let badgeClass;
            
            if (port.label) {
                type = 'link';
                url = `http://${targetHost}:${port.host_port}`;
                displayText = port.label;
                titleText = `${port.label} - Port ${port.host_port} (${port.protocol})`;
                badgeClass = 'port-link badge bg-info text-dark me-1';
            } else if (port.target_service) {
                type = 'scroll';
                scrollTarget = { serviceName: port.target_service, host: currentHost };
                displayText = `→${port.target_service}:${port.host_port}`;
                titleText = `Click to go to ${port.target_service} (port ${port.host_port}, ${port.protocol})`;
                badgeClass = 'port-link-scroll badge bg-secondary text-light me-1';
            } else if (port.source_service) {
                type = 'source-link';
                const sourceIP = getServiceHostIP(port.source_service, currentHost) || targetHost;
                url = `http://${sourceIP}:${port.host_port}`;
                displayText = `${port.source_service}:${port.host_port}`;
                titleText = `Open port ${port.host_port} on ${port.source_service} (${port.protocol})`;
                badgeClass = 'port-link badge bg-info text-dark me-1';
            } else {
                type = 'link';
                url = `http://${targetHost}:${port.host_port}`;
                displayText = `:${port.host_port}`;
                titleText = `Open port ${port.host_port} (${port.protocol})`;
                badgeClass = 'port-link badge bg-info text-dark me-1';
            }
            
            return { type, displayText, titleText, url, scrollTarget, badgeClass, port };
        });
}

function resetPortNavigationState() {
    allServices = [];
}

// ============================================================================
// Port Navigation Tests
// ============================================================================

describe('getServiceHostIP', () => {
    test('returns host_ip for matching service', () => {
        allServices = [
            { name: 'gluetun', host: 'nas', host_ip: '192.168.1.10' },
            { name: 'firefox', host: 'nas', host_ip: '192.168.1.10' }
        ];
        assertEqual(getServiceHostIP('gluetun', 'nas'), '192.168.1.10');
    });

    test('returns null for non-existent service', () => {
        allServices = [
            { name: 'gluetun', host: 'nas', host_ip: '192.168.1.10' }
        ];
        assertEqual(getServiceHostIP('nonexistent', 'nas'), null);
    });

    test('returns null for wrong host', () => {
        allServices = [
            { name: 'gluetun', host: 'nas', host_ip: '192.168.1.10' }
        ];
        assertEqual(getServiceHostIP('gluetun', 'other-host'), null);
    });

    test('handles empty allServices array', () => {
        allServices = [];
        assertEqual(getServiceHostIP('gluetun', 'nas'), null);
    });

    test('returns correct IP when multiple hosts have same service name', () => {
        allServices = [
            { name: 'nginx', host: 'nas', host_ip: '192.168.1.10' },
            { name: 'nginx', host: 'cloud', host_ip: '10.0.0.5' }
        ];
        assertEqual(getServiceHostIP('nginx', 'nas'), '192.168.1.10');
        assertEqual(getServiceHostIP('nginx', 'cloud'), '10.0.0.5');
    });
});

describe('renderPortData - Regular Ports', () => {
    test('returns empty array for null ports', () => {
        resetPortNavigationState();
        const result = renderPortData(null, '192.168.1.10', { host: 'nas' });
        assertDeepEqual(result, []);
    });

    test('returns empty array for empty ports', () => {
        resetPortNavigationState();
        const result = renderPortData([], '192.168.1.10', { host: 'nas' });
        assertDeepEqual(result, []);
    });

    test('renders regular port as link', () => {
        resetPortNavigationState();
        const ports = [{ host_port: 8080, container_port: 80, protocol: 'tcp' }];
        const result = renderPortData(ports, '192.168.1.10', { host: 'nas' });
        assertEqual(result.length, 1);
        assertEqual(result[0].type, 'link');
        assertEqual(result[0].url, 'http://192.168.1.10:8080');
        assertEqual(result[0].displayText, ':8080');
        assert(result[0].titleText.includes('8080'));
    });

    test('uses localhost when hostIP is null', () => {
        resetPortNavigationState();
        const ports = [{ host_port: 3000, protocol: 'tcp' }];
        const result = renderPortData(ports, null, { host: 'nas' });
        assertEqual(result[0].url, 'http://localhost:3000');
    });

    test('filters hidden ports', () => {
        resetPortNavigationState();
        const ports = [
            { host_port: 8080, protocol: 'tcp', hidden: false },
            { host_port: 9000, protocol: 'tcp', hidden: true },
            { host_port: 3000, protocol: 'tcp' }
        ];
        const result = renderPortData(ports, '192.168.1.10', { host: 'nas' });
        assertEqual(result.length, 2);
        assertEqual(result[0].port.host_port, 8080);
        assertEqual(result[1].port.host_port, 3000);
    });
});

describe('renderPortData - Labeled Ports', () => {
    test('uses custom label for display', () => {
        resetPortNavigationState();
        const ports = [{ host_port: 8080, protocol: 'tcp', label: 'Admin' }];
        const result = renderPortData(ports, '192.168.1.10', { host: 'nas' });
        assertEqual(result[0].type, 'link');
        assertEqual(result[0].displayText, 'Admin');
        assert(result[0].titleText.includes('Admin'));
        assertEqual(result[0].url, 'http://192.168.1.10:8080');
    });
});

describe('renderPortData - Target Service Ports (scroll behavior)', () => {
    test('renders target_service port as scroll action', () => {
        resetPortNavigationState();
        const ports = [{ host_port: 8193, protocol: 'tcp', target_service: 'qbittorrent' }];
        const result = renderPortData(ports, '192.168.1.10', { host: 'nas' });
        assertEqual(result.length, 1);
        assertEqual(result[0].type, 'scroll');
        assertEqual(result[0].url, null);
        assertDeepEqual(result[0].scrollTarget, { serviceName: 'qbittorrent', host: 'nas' });
        assertEqual(result[0].displayText, '→qbittorrent:8193');
        assert(result[0].titleText.includes('qbittorrent'));
        assert(result[0].titleText.includes('Click to go to'));
    });

    test('uses current service host for scroll target', () => {
        resetPortNavigationState();
        const ports = [{ host_port: 8193, protocol: 'tcp', target_service: 'firefox' }];
        const result = renderPortData(ports, '192.168.1.10', { host: 'cloud-server' });
        assertEqual(result[0].scrollTarget.host, 'cloud-server');
    });

    test('handles missing current service host', () => {
        resetPortNavigationState();
        const ports = [{ host_port: 8193, protocol: 'tcp', target_service: 'firefox' }];
        const result = renderPortData(ports, '192.168.1.10', null);
        assertEqual(result[0].scrollTarget.host, '');
    });
});

describe('renderPortData - Source Service Ports (link to source IP)', () => {
    test('renders source_service port with source IP', () => {
        allServices = [
            { name: 'gluetun', host: 'nas', host_ip: '192.168.1.10' },
            { name: 'firefox', host: 'nas', host_ip: '192.168.1.10' }
        ];
        const ports = [{ host_port: 8193, protocol: 'tcp', source_service: 'gluetun' }];
        const currentService = { name: 'firefox', host: 'nas' };
        const result = renderPortData(ports, '192.168.1.10', currentService);
        assertEqual(result.length, 1);
        assertEqual(result[0].type, 'source-link');
        assertEqual(result[0].url, 'http://192.168.1.10:8193');
        assertEqual(result[0].displayText, 'gluetun:8193');
        assert(result[0].titleText.includes('gluetun'));
    });

    test('falls back to target host when source service not found', () => {
        allServices = [
            { name: 'firefox', host: 'nas', host_ip: '192.168.1.10' }
        ];
        const ports = [{ host_port: 8193, protocol: 'tcp', source_service: 'missing-vpn' }];
        const currentService = { name: 'firefox', host: 'nas' };
        const result = renderPortData(ports, '10.0.0.1', currentService);
        // Should fall back to the passed hostIP since source not found
        assertEqual(result[0].url, 'http://10.0.0.1:8193');
    });

    test('uses correct source IP from different host', () => {
        allServices = [
            { name: 'gluetun', host: 'nas', host_ip: '192.168.1.10' },
            { name: 'gluetun', host: 'cloud', host_ip: '10.0.0.5' },
            { name: 'firefox', host: 'cloud', host_ip: '10.0.0.5' }
        ];
        const ports = [{ host_port: 8193, protocol: 'tcp', source_service: 'gluetun' }];
        const currentService = { name: 'firefox', host: 'cloud' };
        const result = renderPortData(ports, '10.0.0.5', currentService);
        assertEqual(result[0].url, 'http://10.0.0.5:8193');
    });
});

describe('renderPortData - Complex Scenarios', () => {
    test('handles mixed port types', () => {
        allServices = [
            { name: 'gluetun', host: 'nas', host_ip: '192.168.1.10' },
            { name: 'qbittorrent', host: 'nas', host_ip: '192.168.1.10' }
        ];
        const ports = [
            { host_port: 8080, protocol: 'tcp' },  // Regular
            { host_port: 8193, protocol: 'tcp', target_service: 'qbittorrent' },  // Scroll
            { host_port: 9000, protocol: 'tcp', label: 'Admin' },  // Labeled
        ];
        const currentService = { name: 'gluetun', host: 'nas' };
        const result = renderPortData(ports, '192.168.1.10', currentService);
        
        assertEqual(result.length, 3);
        assertEqual(result[0].type, 'link');
        assertEqual(result[1].type, 'scroll');
        assertEqual(result[2].type, 'link');
        assertEqual(result[2].displayText, 'Admin');
    });

    test('VPN container with multiple remapped ports', () => {
        allServices = [
            { name: 'gluetun', host: 'nas', host_ip: '192.168.1.10' },
            { name: 'qbittorrent-books', host: 'nas', host_ip: '192.168.1.10' },
            { name: 'qbittorrent-movies', host: 'nas', host_ip: '192.168.1.10' }
        ];
        // Ports on gluetun pointing to other services
        const gluetunPorts = [
            { host_port: 8193, protocol: 'tcp', target_service: 'qbittorrent-books' },
            { host_port: 8194, protocol: 'tcp', target_service: 'qbittorrent-movies' }
        ];
        const gluetunService = { name: 'gluetun', host: 'nas' };
        const result = renderPortData(gluetunPorts, '192.168.1.10', gluetunService);
        
        assertEqual(result.length, 2);
        assertEqual(result[0].type, 'scroll');
        assertEqual(result[0].scrollTarget.serviceName, 'qbittorrent-books');
        assertEqual(result[1].type, 'scroll');
        assertEqual(result[1].scrollTarget.serviceName, 'qbittorrent-movies');
    });

    test('service with port from VPN container', () => {
        allServices = [
            { name: 'gluetun', host: 'nas', host_ip: '192.168.1.10' },
            { name: 'qbittorrent-books', host: 'nas', host_ip: '192.168.1.10' }
        ];
        // Port on qbittorrent-books coming from gluetun
        const qbPorts = [
            { host_port: 8193, protocol: 'tcp', source_service: 'gluetun' }
        ];
        const qbService = { name: 'qbittorrent-books', host: 'nas' };
        const result = renderPortData(qbPorts, '192.168.1.10', qbService);
        
        assertEqual(result.length, 1);
        assertEqual(result[0].type, 'source-link');
        assertEqual(result[0].url, 'http://192.168.1.10:8193');
        assertEqual(result[0].displayText, 'gluetun:8193');
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
