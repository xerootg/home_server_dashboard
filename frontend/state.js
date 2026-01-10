/**
 * Shared application state.
 * Centralized state management for cross-module access.
 */

/**
 * Logs viewer state
 */
export const logsState = {
    eventSource: null,
    activeLogsRow: null,
    searchTerm: '',
    caseSensitive: false,
    regex: false,
    bangAndPipe: false,
    mode: 'filter', // 'filter' or 'find'
    currentMatchIndex: -1,
    allMatches: [],
    error: '',
    ast: null,
    debounceTimer: null
};

/**
 * Table/services state
 */
export const servicesState = {
    all: [],
    activeFilter: null,
    activeSourceFilter: null,
    sortColumn: null,
    sortDirection: 'asc'
};

/**
 * Table search state
 */
export const tableSearchState = {
    term: '',
    caseSensitive: false,
    regex: false,
    bangAndPipe: false,
    error: '',
    ast: null,
    debounceTimer: null
};

/**
 * Action modal state
 */
export const actionState = {
    eventSource: null,
    pending: null
};

/**
 * Help modal cache
 */
export const helpState = {
    contentCache: null
};

/**
 * Auth state
 */
export const authState = {
    status: null
};

/**
 * Reset logs state to defaults.
 * Called when closing logs viewer.
 */
export function resetLogsState() {
    logsState.eventSource = null;
    logsState.activeLogsRow = null;
    logsState.searchTerm = '';
    logsState.caseSensitive = false;
    logsState.regex = false;
    logsState.bangAndPipe = false;
    logsState.mode = 'filter';
    logsState.currentMatchIndex = -1;
    logsState.allMatches = [];
    logsState.error = '';
    logsState.ast = null;
    if (logsState.debounceTimer) {
        clearTimeout(logsState.debounceTimer);
        logsState.debounceTimer = null;
    }
}

/**
 * Reset table search state to defaults.
 */
export function resetTableSearchState() {
    tableSearchState.term = '';
    tableSearchState.caseSensitive = false;
    tableSearchState.regex = false;
    tableSearchState.bangAndPipe = false;
    tableSearchState.error = '';
    tableSearchState.ast = null;
    if (tableSearchState.debounceTimer) {
        clearTimeout(tableSearchState.debounceTimer);
        tableSearchState.debounceTimer = null;
    }
}
