/**
 * Minimal test framework for Node.js.
 * Provides describe/it/assert functions similar to Mocha/Jest.
 */

let testsPassed = 0;
let testsFailed = 0;
const failures = [];
let currentSuite = '';

/**
 * Assert that a condition is true.
 * @param {boolean} condition - The condition to check
 * @param {string} message - Error message if assertion fails
 */
export function assert(condition, message) {
    if (!condition) {
        throw new Error(message || 'Assertion failed');
    }
}

/**
 * Assert that two values are strictly equal.
 * @param {*} actual - The actual value
 * @param {*} expected - The expected value
 * @param {string} message - Error message prefix
 */
export function assertEqual(actual, expected, message) {
    if (actual !== expected) {
        throw new Error(`${message || 'Assertion failed'}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
    }
}

/**
 * Assert that two values are deeply equal (JSON comparison).
 * @param {*} actual - The actual value
 * @param {*} expected - The expected value
 * @param {string} message - Error message prefix
 */
export function assertDeepEqual(actual, expected, message) {
    const actualStr = JSON.stringify(actual);
    const expectedStr = JSON.stringify(expected);
    if (actualStr !== expectedStr) {
        throw new Error(`${message || 'Assertion failed'}: expected ${expectedStr}, got ${actualStr}`);
    }
}

/**
 * Define a test suite.
 * @param {string} suiteName - Name of the test suite
 * @param {Function} fn - Function containing test cases
 */
export function describe(suiteName, fn) {
    currentSuite = suiteName;
    console.log(`\n${suiteName}`);
    fn();
}

/**
 * Define a test case.
 * @param {string} name - Name of the test
 * @param {Function} fn - Test function (can be async)
 */
export function it(name, fn) {
    return test(name, fn);
}

/**
 * Define a test case (alias for it).
 * @param {string} name - Name of the test
 * @param {Function} fn - Test function (can be async)
 */
export async function test(name, fn) {
    try {
        const result = fn();
        if (result instanceof Promise) {
            await result;
        }
        testsPassed++;
        console.log(`  ✓ ${name}`);
    } catch (e) {
        testsFailed++;
        failures.push({ suite: currentSuite, name, error: e.message });
        console.log(`  ✗ ${name}`);
        console.log(`    ${e.message}`);
    }
}

/**
 * Get test results summary.
 * @returns {{passed: number, failed: number, failures: Array}}
 */
export function getResults() {
    return {
        passed: testsPassed,
        failed: testsFailed,
        failures: [...failures]
    };
}

/**
 * Reset test counters.
 */
export function resetResults() {
    testsPassed = 0;
    testsFailed = 0;
    failures.length = 0;
}

/**
 * Print test summary and exit with appropriate code.
 */
export function printSummary() {
    console.log('\n' + '='.repeat(50));
    console.log(`Tests: ${testsPassed} passed, ${testsFailed} failed`);
    
    if (failures.length > 0) {
        console.log('\nFailures:');
        failures.forEach((f, i) => {
            console.log(`  ${i + 1}. ${f.suite} > ${f.name}`);
            console.log(`     ${f.error}`);
        });
    }
    
    console.log('='.repeat(50));
}

/**
 * Exit with appropriate code based on test results.
 */
export function exit() {
    process.exit(testsFailed > 0 ? 1 : 0);
}
