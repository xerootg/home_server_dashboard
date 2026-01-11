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
    bangAndPipe: true,
    mode: 'filter', // 'filter' or 'find'
    currentMatchIndex: -1,
    allMatches: [],
    error: '',
    ast: null,
    debounceTimer: null
};

/**
 * Filter state values:
 * - null: include (default, no filtering)
 * - 'include': explicitly include, same as active filter
 * - 'exclude': exclude matches from results
 * - 'exclusive': only show matches (same as include for single filters)
 */

/**
 * Table/services state
 */
export const servicesState = {
    all: [],
    activeFilter: null,           // Status filter: null | { status: 'running'|'stopped', mode: 'include'|'exclude'|'exclusive' }
    activeSourceFilter: null,     // Source filter: null | { source: string, mode: 'include'|'exclude'|'exclusive' }
    activeHostFilters: {},        // Host filters: { [hostname]: 'include'|'exclude'|'exclusive' }
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
    bangAndPipe: true,
    error: '',
    ast: null,
    debounceTimer: null,
    mode: 'filter', // 'filter' or 'find'
    currentMatchIndex: -1,
    allMatches: [] // Array of { service, index } for navigation
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
 * WebSocket state
 */
export const websocketState = {
    socket: null,
    status: 'disconnected', // 'disconnected', 'connecting', 'connected', 'reconnecting', 'error'
    reconnecting: false,
    reconnectDelay: 1000,
    reconnectTimer: null
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
    logsState.bangAndPipe = true;
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
    tableSearchState.bangAndPipe = true;
    tableSearchState.error = '';
    tableSearchState.ast = null;
    tableSearchState.mode = 'filter';
    tableSearchState.currentMatchIndex = -1;
    tableSearchState.allMatches = [];
    if (tableSearchState.debounceTimer) {
        clearTimeout(tableSearchState.debounceTimer);
        tableSearchState.debounceTimer = null;
    }
}
