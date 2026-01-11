/**
 * Main entry point for the dashboard.
 * Wires together all modules and initializes event handlers.
 */

import { servicesState } from './state.js';
import { renderServices, updateServiceRow, renderHostFilters, showStatusToast } from './render.js';
import { toggleFilter, toggleSourceFilter, toggleHostFilter, toggleSort, applyFilter, updateHostFilterUI } from './filter.js';
import { toggleLogs, closeLogs, onLogsSearchInput, onLogsSearchKeydown, toggleLogsSearchMode, toggleLogsCaseSensitivity, toggleLogsRegex, toggleLogsBangAndPipe, navigateMatch } from './logs.js';
import { onTableSearchInput, onTableSearchKeydown, clearTableSearch, toggleTableCaseSensitivity, toggleTableRegex, toggleTableBangAndPipe, toggleTableSearchMode, navigateTableMatch, updateTableBangPipeToggleUI } from './table-search.js';
import { confirmServiceAction, executeServiceAction, confirmLogFlush, executeLogFlush } from './actions.js';
import { loadServices, checkAuthStatus, logout } from './api.js';
import { showHelpModal } from './help.js';
import { scrollToService } from './services.js';
import { connect as wsConnect, disconnect as wsDisconnect, on as wsOn, isConnected as wsIsConnected } from './websocket.js';
import { initColumnsState, toggleColumnDropdown, closeColumnDropdown, toggleColumnVisibility, resetColumnsToDefault, applyColumnVisibility, initClickOutsideHandler, getVisibleColumns, startColumnResize, resetColumnWidth } from './columns.js';

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
        toggleHostFilter: (host) => toggleHostFilter(host, callbacks),
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
        onTableSearchKeydown: (event) => onTableSearchKeydown(event, callbacks),
        clearTableSearch: () => clearTableSearch(callbacks),
        toggleTableCaseSensitivity: () => toggleTableCaseSensitivity(callbacks),
        toggleTableRegex: () => toggleTableRegex(callbacks),
        toggleTableBangAndPipe: () => toggleTableBangAndPipe(callbacks),
        toggleTableSearchMode: () => toggleTableSearchMode(callbacks),
        navigateTableMatch,
        
        // Service actions
        confirmServiceAction,
        executeServiceAction: () => executeServiceAction(doLoadServices),
        
        // Log flush actions
        confirmLogFlush,
        executeLogFlush: () => executeLogFlush(doLoadServices),
        
        // Help
        showHelpModal,
        
        // Navigation
        scrollToService,
        scrollToTop,
        
        // Auth
        logout,
        
        // Refresh
        loadServices: doLoadServices,
        
        // WebSocket
        wsConnect,
        wsDisconnect,
        wsIsConnected,
        
        // Column settings
        toggleColumnDropdown,
        toggleColumnVisibility: (columnId) => {
            toggleColumnVisibility(columnId);
            applyColumnVisibility(() => {
                // Re-render services with updated columns
                if (servicesState.all.length > 0) {
                    renderServices(servicesState.all, true, callbacks);
                    // Re-apply filters if active
                    if (servicesState.activeFilter || servicesState.activeSourceFilter || Object.keys(servicesState.activeHostFilters).length > 0) {
                        applyFilter(callbacks);
                    }
                }
            });
        },
        resetColumns: () => {
            resetColumnsToDefault();
            applyColumnVisibility(() => {
                // Re-render services with default columns
                if (servicesState.all.length > 0) {
                    renderServices(servicesState.all, true, callbacks);
                    // Re-apply filters if active
                    if (servicesState.activeFilter || servicesState.activeSourceFilter || Object.keys(servicesState.activeHostFilters).length > 0) {
                        applyFilter(callbacks);
                    }
                }
            });
            toggleColumnDropdown(); // Re-render dropdown
        },
        startColumnResize,
        resetColumnWidth,
        
        // Mobile status toast
        showStatusToast
    };
}

/**
 * Load services and render them.
 */
async function doLoadServices() {
    await loadServices({
        onSuccess: (services) => {
            renderServices(services, true, callbacks);
            
            // Render host filter badges
            renderHostFilters(services);
            updateHostFilterUI();
            
            // Re-apply filter if one is active
            if (servicesState.activeFilter || servicesState.activeSourceFilter || Object.keys(servicesState.activeHostFilters).length > 0) {
                applyFilter(callbacks);
            }
        },
        onError: () => {
            if (typeof document !== 'undefined') {
                const visibleColumns = getVisibleColumns();
                const colspan = visibleColumns.length || 8;
                document.getElementById('servicesTable').innerHTML = 
                    `<tr><td colspan="${colspan}" class="text-center text-danger">Error loading services</td></tr>`;
            }
        }
    });
}

/**
 * Initialize the dashboard.
 */
async function init() {
    // Initialize click-outside handler for dropdowns
    initClickOutsideHandler();
    
    // Listen for column changes (from drag-drop reordering)
    if (typeof window !== 'undefined') {
        window.addEventListener('columnsChanged', () => {
            if (servicesState.all.length > 0) {
                renderServices(servicesState.all, true, callbacks);
                // Re-apply filters if active
                if (servicesState.activeFilter || servicesState.activeSourceFilter || Object.keys(servicesState.activeHostFilters).length > 0) {
                    applyFilter(callbacks);
                }
            }
        });
    }
    
    // Check auth first so we have the user ID for per-user column settings
    await checkAuthStatus();
    
    // Initialize column settings AFTER auth so we use the correct storage key
    initColumnsState();
    
    await doLoadServices();
    
    // Initialize table search UI (bangAndPipe is true by default)
    updateTableBangPipeToggleUI();
    
    // Initialize WebSocket connection for real-time updates
    initWebSocket();
    
    // Initialize sticky search bar detection
    initStickySearchBar();
}

/**
 * Initialize WebSocket and register event handlers.
 */
function initWebSocket() {
    // Handle service state updates
    wsOn('service_update', (payload) => {
        // Update the specific service row reactively
        updateServiceRow(payload, callbacks);
        
        // Also update the service in our state array
        const service = servicesState.all.find(s => 
            s.name === payload.service_name && 
            s.host === payload.host && 
            s.source === payload.source
        );
        if (service) {
            service.state = payload.current_state;
            service.status = payload.status;
        }
    });
    
    // Handle host unreachable events
    wsOn('host_unreachable', (payload) => {
        console.log('Host unreachable:', payload.host, payload.reason);
        showHostNotification('error', `Host ${payload.host} is unreachable: ${payload.reason}`);
    });
    
    // Handle host recovered events
    wsOn('host_recovered', (payload) => {
        console.log('Host recovered:', payload.host);
        showHostNotification('success', `Host ${payload.host} is back online`);
        // Refresh services to get updated state
        doLoadServices();
    });
    
    // Handle connection events
    wsOn('connect', () => {
        console.log('WebSocket connected - real-time updates enabled');
    });
    
    wsOn('disconnect', () => {
        console.log('WebSocket disconnected - will reconnect automatically');
    });
    
    // Start the connection
    wsConnect();
}

/**
 * Initialize sticky search bar behavior using IntersectionObserver.
 * Adds 'is-sticky' class when the search bar is pinned to the top.
 * Also controls the scroll-to-top button visibility.
 */
function initStickySearchBar() {
    if (typeof document === 'undefined') return;
    
    const searchContainer = document.querySelector('.table-search-container');
    const scrollToTopBtn = document.getElementById('scrollToTopBtn');
    if (!searchContainer) return;
    
    // Create a sentinel element to detect when search bar becomes sticky
    const sentinel = document.createElement('div');
    sentinel.className = 'table-search-sentinel';
    sentinel.style.cssText = 'height: 1px; width: 100%; pointer-events: none;';
    searchContainer.parentNode.insertBefore(sentinel, searchContainer);
    
    // Use IntersectionObserver to detect when sentinel goes out of view
    const observer = new IntersectionObserver(
        (entries) => {
            entries.forEach(entry => {
                // When sentinel is not visible, search bar is sticky
                if (!entry.isIntersecting) {
                    searchContainer.classList.add('is-sticky');
                    if (scrollToTopBtn) scrollToTopBtn.style.display = 'flex';
                } else {
                    searchContainer.classList.remove('is-sticky');
                    if (scrollToTopBtn) scrollToTopBtn.style.display = 'none';
                }
            });
        },
        { threshold: 0, rootMargin: '0px' }
    );
    
    observer.observe(sentinel);
}

/**
 * Scroll to the top of the page smoothly.
 */
function scrollToTop() {
    if (typeof window !== 'undefined') {
        window.scrollTo({ top: 0, behavior: 'smooth' });
    }
}

/**
 * Show a temporary notification for host events.
 * @param {string} type - 'error' or 'success'
 * @param {string} message - Notification message
 */
function showHostNotification(type, message) {
    if (typeof document === 'undefined') return;
    
    // Create notification element
    const notification = document.createElement('div');
    notification.className = `alert alert-${type === 'error' ? 'danger' : 'success'} alert-dismissible fade show host-notification`;
    notification.innerHTML = `
        <i class="bi bi-${type === 'error' ? 'exclamation-triangle' : 'check-circle'}"></i>
        ${message}
        <button type="button" class="btn-close" data-bs-dismiss="alert"></button>
    `;
    
    // Find or create notification container
    let container = document.getElementById('notificationContainer');
    if (!container) {
        container = document.createElement('div');
        container.id = 'notificationContainer';
        container.className = 'notification-container';
        document.body.appendChild(container);
    }
    
    container.appendChild(notification);
    
    // Auto-dismiss after 5 seconds
    setTimeout(() => {
        notification.classList.remove('show');
        setTimeout(() => notification.remove(), 150);
    }, 5000);
}

// Initialize on DOM ready
if (typeof document !== 'undefined') {
    document.addEventListener('DOMContentLoaded', init);
}

// Export for testing
export { init, doLoadServices };
