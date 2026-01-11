/**
 * Tests for state.js
 */

import { describe, it, assertEqual, assertDeepEqual } from './test-utils.mjs';
import { 
    logsState, 
    servicesState, 
    tableSearchState, 
    actionState,
    helpState,
    authState,
    resetLogsState,
    resetTableSearchState
} from './state.js';

describe('logsState', () => {
    it('has default values', () => {
        assertEqual(logsState.eventSource, null);
        assertEqual(logsState.activeLogsRow, null);
        assertEqual(logsState.searchTerm, '');
        assertEqual(logsState.caseSensitive, false);
        assertEqual(logsState.regex, false);
        assertEqual(logsState.bangAndPipe, true);
        assertEqual(logsState.mode, 'filter');
    });
});

describe('resetLogsState', () => {
    it('resets all logs state values', () => {
        // Modify state
        logsState.searchTerm = 'test';
        logsState.caseSensitive = true;
        logsState.regex = true;
        logsState.bangAndPipe = true;
        logsState.mode = 'find';
        logsState.currentMatchIndex = 5;
        logsState.error = 'some error';
        
        // Reset
        resetLogsState();
        
        // Verify
        assertEqual(logsState.searchTerm, '');
        assertEqual(logsState.caseSensitive, false);
        assertEqual(logsState.regex, false);
        assertEqual(logsState.bangAndPipe, true);
        assertEqual(logsState.mode, 'filter');
        assertEqual(logsState.currentMatchIndex, -1);
        assertEqual(logsState.error, '');
    });
});

describe('servicesState', () => {
    it('has default values', () => {
        // May have been modified by other tests, so just check structure
        assertEqual(Array.isArray(servicesState.all), true);
        assertEqual(servicesState.sortDirection, 'asc');
    });
});

describe('tableSearchState', () => {
    it('has default values', () => {
        assertEqual(tableSearchState.term, '');
        assertEqual(tableSearchState.caseSensitive, false);
        assertEqual(tableSearchState.regex, false);
        assertEqual(tableSearchState.bangAndPipe, true);
    });
});

describe('resetTableSearchState', () => {
    it('resets all table search state values', () => {
        // Modify state
        tableSearchState.term = 'test';
        tableSearchState.caseSensitive = true;
        tableSearchState.regex = true;
        tableSearchState.bangAndPipe = true;
        tableSearchState.error = 'some error';
        
        // Reset
        resetTableSearchState();
        
        // Verify
        assertEqual(tableSearchState.term, '');
        assertEqual(tableSearchState.caseSensitive, false);
        assertEqual(tableSearchState.regex, false);
        assertEqual(tableSearchState.bangAndPipe, true);
        assertEqual(tableSearchState.error, '');
    });
});

describe('actionState', () => {
    it('has default values', () => {
        assertEqual(actionState.eventSource, null);
        assertEqual(actionState.pending, null);
    });
});

describe('helpState', () => {
    it('has default values', () => {
        assertEqual(helpState.contentCache, null);
    });
});

describe('authState', () => {
    it('has default values', () => {
        assertEqual(authState.status, null);
    });
});
