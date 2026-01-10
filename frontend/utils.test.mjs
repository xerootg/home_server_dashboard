/**
 * Tests for utils.js
 */

import { describe, it, assert, assertEqual } from './test-utils.mjs';
import { escapeHtml, getStatusClass } from './utils.js';

describe('escapeHtml', () => {
    it('escapes HTML special characters', () => {
        assertEqual(escapeHtml('<script>'), '&lt;script&gt;');
        assertEqual(escapeHtml('a & b'), 'a &amp; b');
        assertEqual(escapeHtml('"quoted"'), '&quot;quoted&quot;');
        assertEqual(escapeHtml("'single'"), '&#039;single&#039;');
    });

    it('handles empty string', () => {
        assertEqual(escapeHtml(''), '');
    });

    it('handles plain text', () => {
        assertEqual(escapeHtml('hello world'), 'hello world');
    });
});

describe('getStatusClass', () => {
    it('returns running for running state', () => {
        assertEqual(getStatusClass('running', 'Up 5 minutes'), 'running');
    });

    it('returns unhealthy for running but unhealthy', () => {
        assertEqual(getStatusClass('running', 'Up 5 minutes (unhealthy)'), 'unhealthy');
    });

    it('returns stopped for stopped state', () => {
        assertEqual(getStatusClass('stopped', 'Exited'), 'stopped');
    });

    it('is case insensitive', () => {
        assertEqual(getStatusClass('RUNNING', 'UP'), 'running');
        assertEqual(getStatusClass('Running', 'Up (UNHEALTHY)'), 'unhealthy');
    });
});
