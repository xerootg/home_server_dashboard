/**
 * Main entry point for the dashboard.
 * Wires together all modules and initializes event handlers.
 */

import { servicesState } from './state.js';
import { renderServices } from './render.js';
import { toggleFilter, toggleSourceFilter, toggleSort, applyFilter } from './filter.js';
import { toggleLogs, closeLogs, onLogsSearchInput, onLogsSearchKeydown, toggleLogsSearchMode, toggleLogsCaseSensitivity, toggleLogsRegex, toggleLogsBangAndPipe, navigateMatch } from './logs.js';
import { onTableSearchInput, onTableSearchKeydown, clearTableSearch, toggleTableCaseSensitivity, toggleTableRegex, toggleTableBangAndPipe } from './table-search.js';
import { confirmServiceAction, executeServiceAction } from './actions.js';
import { loadServices, checkAuthStatus, logout } from './api.js';
import { showHelpModal } from './help.js';
import { scrollToService } from './services.js';

// Create callbacks object for passing to modules
const callbacks = {
    closeLogs,
    onRowClick: toggleLogs
};

// Expose functions to window for onclick handlers in HTML
// This is necessary because esbuild bundles everything and the functions
// wouldn't otherwise be accessible from inline event handlers
if (typeof window !== 'undefined') {
    window.__dashboard = {
        // Filter functions
        toggleFilter: (filter) => toggleFilter(filter, callbacks),
        toggleSourceFilter: (source) => toggleSourceFilter(source, callbacks),
        toggleSort: (column) => toggleSort(column, callbacks),
        
        // Logs functions
        toggleLogs,
        closeLogs,
        onLogsSearchInput,
        onLogsSearchKeydown,
        toggleLogsSearchMode,
        toggleLogsCaseSensitivity,
        toggleLogsRegex,
        toggleLogsBangAndPipe,
        navigateMatch,
        
        // Table search functions
        onTableSearchInput: (term) => onTableSearchInput(term, callbacks),
        onTableSearchKeydown,
        clearTableSearch: () => clearTableSearch(callbacks),
        toggleTableCaseSensitivity: () => toggleTableCaseSensitivity(callbacks),
        toggleTableRegex: () => toggleTableRegex(callbacks),
        toggleTableBangAndPipe: () => toggleTableBangAndPipe(callbacks),
        
        // Service actions
        confirmServiceAction,
        executeServiceAction: () => executeServiceAction(doLoadServices),
        
        // Help
        showHelpModal,
        
        // Navigation
        scrollToService,
        
        // Auth
        logout,
        
        // Refresh
        loadServices: doLoadServices
    };
}

/**
 * Load services and render them.
 */
async function doLoadServices() {
    await loadServices({
        onSuccess: (services) => {
            renderServices(services, true, callbacks);
            
            // Re-apply filter if one is active
            if (servicesState.activeFilter || servicesState.activeSourceFilter) {
                applyFilter(callbacks);
            }
        },
        onError: () => {
            if (typeof document !== 'undefined') {
                document.getElementById('servicesTable').innerHTML = 
                    '<tr><td colspan="6" class="text-center text-danger">Error loading services</td></tr>';
            }
        }
    });
}

/**
 * Initialize the dashboard.
 */
async function init() {
    await checkAuthStatus();
    await doLoadServices();
}

// Initialize on DOM ready
if (typeof document !== 'undefined') {
    document.addEventListener('DOMContentLoaded', init);
}

// Export for testing
export { init, doLoadServices };
