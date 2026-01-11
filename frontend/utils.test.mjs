/**
 * Tests for utils.js
 */

import { describe, it, assert, assertEqual } from './test-utils.mjs';
import { escapeHtml, getStatusClass, formatLogSize } from './utils.js';

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

describe('formatLogSize', () => {
    it('returns 0 for zero bytes', () => {
        assertEqual(formatLogSize(0), '0');
    });

    it('returns 0 for undefined', () => {
        assertEqual(formatLogSize(undefined), '0');
    });

    it('returns 0 for null', () => {
        assertEqual(formatLogSize(null), '0');
    });

    it('formats bytes without suffix', () => {
        assertEqual(formatLogSize(500), '500');
        assertEqual(formatLogSize(1023), '1023');
    });

    it('formats kilobytes with K suffix', () => {
        assertEqual(formatLogSize(1024), '1.00K');
        assertEqual(formatLogSize(2048), '2.00K');
        assertEqual(formatLogSize(10240), '10.0K');
        assertEqual(formatLogSize(102400), '100K');
    });

    it('formats megabytes with M suffix', () => {
        assertEqual(formatLogSize(1048576), '1.00M');
        assertEqual(formatLogSize(1572864), '1.50M');
        assertEqual(formatLogSize(10485760), '10.0M');
        assertEqual(formatLogSize(104857600), '100M');
    });

    it('formats gigabytes with G suffix', () => {
        assertEqual(formatLogSize(1073741824), '1.00G');
        assertEqual(formatLogSize(2147483648), '2.00G');
    });

    it('formats terabytes with T suffix', () => {
        assertEqual(formatLogSize(1099511627776), '1.00T');
    });
});
