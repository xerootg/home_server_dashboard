/**
 * Filtering and sorting functions for services.
 * 
 * Tristate filter modes:
 * - null/undefined: No filter active (include all)
 * - 'include': Include matching items (shown with blue border)
 * - 'exclude': Exclude matching items (shown with red border)
 * - 'exclusive': Only show matching items, same as include but semantic difference (shown with green border)
 */

import { servicesState, tableSearchState } from './state.js';
import { textMatches, evaluateAST } from './search-core.js';
import { renderServices, renderHostFilters } from './render.js';

/**
 * Tristate filter mode cycle order.
 * Clicking cycles: include -> exclude -> exclusive -> (clear)
 * After exclusive, clicking again clears the filter.
 */
const FILTER_MODE_CYCLE = ['include', 'exclude', 'exclusive'];

/**
 * Get next filter mode in cycle.
 * @param {string|null|undefined} currentMode - Current filter mode
 * @returns {string|null} Next mode in cycle, or null to clear
 */
export function getNextFilterMode(currentMode) {
    // If no mode or invalid mode, start with 'include'
    const currentIndex = FILTER_MODE_CYCLE.indexOf(currentMode);
    if (currentIndex === -1) {
        return 'include';
    }
    // After the last mode (exclusive), return null to clear
    if (currentIndex === FILTER_MODE_CYCLE.length - 1) {
        return null;
    }
    return FILTER_MODE_CYCLE[currentIndex + 1];
}

/**
 * Sort services by column.
 * @param {Array} services - Array of services to sort
 * @param {string} column - Column name to sort by
 * @param {string} direction - 'asc' or 'desc'
 * @returns {Array} Sorted services array
 */
export function sortServices(services, column, direction) {
    return services.sort((a, b) => {
        let valueA, valueB;
        
        switch (column) {
            case 'name':
                valueA = a.name.toLowerCase();
                valueB = b.name.toLowerCase();
                break;
            case 'project':
                valueA = a.project.toLowerCase();
                valueB = b.project.toLowerCase();
                break;
            case 'host':
                valueA = (a.host || '').toLowerCase();
                valueB = (b.host || '').toLowerCase();
                break;
            case 'container':
                valueA = a.container_name.toLowerCase();
                valueB = b.container_name.toLowerCase();
                break;
            case 'status':
                valueA = a.status.toLowerCase();
                valueB = b.status.toLowerCase();
                break;
            case 'image':
                valueA = a.image.toLowerCase();
                valueB = b.image.toLowerCase();
                break;
            case 'log_size':
                valueA = a.log_size || 0;
                valueB = b.log_size || 0;
                break;
            default:
                return 0;
        }
        
        let comparison = 0;
        if (valueA < valueB) comparison = -1;
        if (valueA > valueB) comparison = 1;
        
        return direction === 'desc' ? -comparison : comparison;
    });
}

/**
 * Check if a service matches the table search term.
 * @param {Object} service - The service object
 * @returns {boolean} Whether the service matches
 */
export function serviceMatchesTableSearch(service) {
    if (!tableSearchState.term) return true;
    
    // Combine all searchable fields into a single string for matching
    const searchableText = [
        service.name || '',
        service.project || '',
        service.host || '',
        service.container_name || '',
        service.status || '',
        service.state || '',
        service.image || '',
        service.source || '',
        ...(service.ports || []).map(p => String(p.host_port)),
        ...(service.traefik_urls || []).map(url => {
            try {
                return new URL(url).hostname;
            } catch (e) {
                return url;
            }
        })
    ].join(' ');
    
    return tableTextMatches(searchableText, tableSearchState.term);
}

/**
 * Check if text matches the table search term using current search settings.
 * @param {string} text - The text to search in
 * @param {string} searchTerm - The search term
 * @returns {boolean} Whether the text matches
 */
export function tableTextMatches(text, searchTerm) {
    if (!searchTerm) return true;
    
    // Bang-and-pipe mode: use AST evaluation
    if (tableSearchState.bangAndPipe) {
        if (!tableSearchState.ast) return false;
        return evaluateAST(tableSearchState.ast, text, tableSearchState.caseSensitive);
    }
    
    return textMatches(text, searchTerm, {
        caseSensitive: tableSearchState.caseSensitive,
        regex: tableSearchState.regex,
        bangAndPipe: false,
        ast: null
    });
}

/**
 * Toggle status filter with tristate cycling.
 * @param {string} filter - 'running', 'stopped', or 'all'
 * @param {Object} callbacks - Callback functions
 */
export function toggleFilter(filter, callbacks = {}) {
    if (filter === 'all') {
        // 'all' clears all status filters
        servicesState.activeFilter = null;
    } else {
        // Get current filter state
        const current = servicesState.activeFilter;
        
        if (current && current.status === filter) {
            // Same filter clicked - cycle through modes
            const nextMode = getNextFilterMode(current.mode);
            if (nextMode === null) {
                servicesState.activeFilter = null;
            } else {
                servicesState.activeFilter = { status: filter, mode: nextMode };
            }
        } else {
            // Different filter or no filter - start with include
            servicesState.activeFilter = { status: filter, mode: 'include' };
        }
    }
    
    updateStatusFilterUI();
    applyFilter(callbacks);
}

/**
 * Update status filter card UI based on current state.
 */
export function updateStatusFilterUI() {
    if (typeof document === 'undefined') return;
    
    document.querySelectorAll('.stat-card:not(.source-filter):not(.host-filter)').forEach(card => {
        card.classList.remove('active', 'filter-exclude', 'filter-exclusive');
    });
    
    if (servicesState.activeFilter && servicesState.activeFilter.status) {
        const activeCard = document.querySelector(`.stat-card[data-filter="${servicesState.activeFilter.status}"]`);
        if (activeCard) {
            activeCard.classList.add('active');
            if (servicesState.activeFilter.mode === 'exclude') {
                activeCard.classList.add('filter-exclude');
            } else if (servicesState.activeFilter.mode === 'exclusive') {
                activeCard.classList.add('filter-exclusive');
            }
        }
    }
}

/**
 * Toggle source filter with tristate cycling.
 * @param {string} source - 'docker', 'systemd', 'traefik', 'homeassistant', or null to clear
 * @param {Object} callbacks - Callback functions
 */
export function toggleSourceFilter(source, callbacks = {}) {
    // Get current filter state
    const current = servicesState.activeSourceFilter;
    
    if (current && current.source === source) {
        // Same filter clicked - cycle through modes
        const nextMode = getNextFilterMode(current.mode);
        if (nextMode === null) {
            servicesState.activeSourceFilter = null;
        } else {
            servicesState.activeSourceFilter = { source: source, mode: nextMode };
        }
    } else {
        // Different filter or no filter - start with include
        servicesState.activeSourceFilter = { source: source, mode: 'include' };
    }
    
    updateSourceFilterUI();
    applyFilter(callbacks);
}

/**
 * Update source filter card UI based on current state.
 */
export function updateSourceFilterUI() {
    if (typeof document === 'undefined') return;
    
    document.querySelectorAll('.stat-card.source-filter').forEach(card => {
        card.classList.remove('active', 'filter-exclude', 'filter-exclusive');
    });
    
    if (servicesState.activeSourceFilter && servicesState.activeSourceFilter.source) {
        const activeCard = document.querySelector(`.stat-card[data-source-filter="${servicesState.activeSourceFilter.source}"]`);
        if (activeCard) {
            activeCard.classList.add('active');
            if (servicesState.activeSourceFilter.mode === 'exclude') {
                activeCard.classList.add('filter-exclude');
            } else if (servicesState.activeSourceFilter.mode === 'exclusive') {
                activeCard.classList.add('filter-exclusive');
            }
        }
    }
}

/**
 * Toggle host filter with tristate cycling.
 * @param {string} host - Host name to filter
 * @param {Object} callbacks - Callback functions
 */
export function toggleHostFilter(host, callbacks = {}) {
    const currentMode = servicesState.activeHostFilters[host] || null;
    const nextMode = getNextFilterMode(currentMode);
    
    if (nextMode === null) {
        delete servicesState.activeHostFilters[host];
    } else {
        servicesState.activeHostFilters[host] = nextMode;
    }
    
    updateHostFilterUI();
    applyFilter(callbacks);
}

/**
 * Update host filter badge UI based on current state.
 */
export function updateHostFilterUI() {
    if (typeof document === 'undefined') return;
    
    document.querySelectorAll('.host-filter-badge').forEach(badge => {
        const host = badge.dataset.host;
        const mode = servicesState.activeHostFilters[host] || null;
        
        badge.classList.remove('active', 'filter-exclude', 'filter-exclusive');
        
        if (mode) {
            badge.classList.add('active');
            if (mode === 'exclude') {
                badge.classList.add('filter-exclude');
            } else if (mode === 'exclusive') {
                badge.classList.add('filter-exclusive');
            }
        }
    });
}

/**
 * Toggle sort on a column.
 * @param {string} column - Column name
 * @param {Object} callbacks - Callback functions
 */
export function toggleSort(column, callbacks = {}) {
    if (servicesState.sortColumn === column) {
        servicesState.sortDirection = servicesState.sortDirection === 'asc' ? 'desc' : 'asc';
    } else {
        servicesState.sortColumn = column;
        servicesState.sortDirection = 'asc';
    }
    
    updateSortIndicators();
    applyFilter(callbacks);
}

/**
 * Update sort indicator arrows in table headers.
 */
export function updateSortIndicators() {
    if (typeof document === 'undefined') return;
    
    document.querySelectorAll('th[data-sort] .sort-indicator').forEach(el => {
        el.textContent = '';
    });
    
    if (servicesState.sortColumn) {
        const th = document.querySelector(`th[data-sort="${servicesState.sortColumn}"] .sort-indicator`);
        if (th) {
            th.textContent = servicesState.sortDirection === 'asc' ? ' ▲' : ' ▼';
        }
    }
}

/**
 * Apply all filters and re-render the services table.
 * @param {Object} callbacks - Callback functions
 */
export function applyFilter(callbacks = {}) {
    let services = [...servicesState.all];
    
    // Apply status filter with tristate logic
    if (servicesState.activeFilter && servicesState.activeFilter.status) {
        const { status, mode } = servicesState.activeFilter;
        services = services.filter(service => {
            const isRunning = service.state.toLowerCase() === 'running';
            let matches;
            if (status === 'running') {
                matches = isRunning;
            } else if (status === 'stopped') {
                matches = !isRunning;
            } else {
                matches = true;
            }
            
            // Apply mode: include/exclusive show matches, exclude hides matches
            if (mode === 'exclude') {
                return !matches;
            }
            return matches; // include or exclusive
        });
    }
    
    // Apply source filter with tristate logic
    if (servicesState.activeSourceFilter && servicesState.activeSourceFilter.source) {
        const { source, mode } = servicesState.activeSourceFilter;
        services = services.filter(service => {
            let matches;
            if (source === 'traefik') {
                matches = service.source === 'traefik' || 
                       (service.traefik_urls && service.traefik_urls.length > 0);
            } else if (source === 'homeassistant') {
                matches = service.source === 'homeassistant' || 
                       service.source === 'homeassistant-addon';
            } else {
                matches = service.source === source;
            }
            
            // Apply mode
            if (mode === 'exclude') {
                return !matches;
            }
            return matches; // include or exclusive
        });
    }
    
    // Apply host filters with tristate logic
    const hostFilters = servicesState.activeHostFilters;
    const activeHostFilters = Object.entries(hostFilters).filter(([_, mode]) => mode);
    
    if (activeHostFilters.length > 0) {
        // Check if any filter is in exclusive mode
        const exclusiveHosts = activeHostFilters.filter(([_, mode]) => mode === 'exclusive').map(([host]) => host);
        const includeHosts = activeHostFilters.filter(([_, mode]) => mode === 'include').map(([host]) => host);
        const excludeHosts = activeHostFilters.filter(([_, mode]) => mode === 'exclude').map(([host]) => host);
        
        services = services.filter(service => {
            const serviceHost = service.host || '';
            
            // If any exclusive filters, only show services matching those hosts
            if (exclusiveHosts.length > 0) {
                if (!exclusiveHosts.includes(serviceHost)) {
                    return false;
                }
            }
            
            // Exclude hosts marked for exclusion
            if (excludeHosts.includes(serviceHost)) {
                return false;
            }
            
            // If only include filters (no exclusive), show services matching include or unfiltered
            if (includeHosts.length > 0 && exclusiveHosts.length === 0) {
                // When we have include filters, we show: included hosts + hosts not in any filter
                const allFilteredHosts = [...includeHosts, ...excludeHosts];
                if (allFilteredHosts.includes(serviceHost)) {
                    return includeHosts.includes(serviceHost);
                }
                // Service host is not in any filter, show it
                return true;
            }
            
            return true;
        });
    }
    
    // Apply table search filter (only in filter mode)
    const servicesBeforeSearch = services.length;
    let matchingServices = [];
    
    if (tableSearchState.term) {
        if (tableSearchState.mode === 'filter') {
            // Filter mode: hide non-matching services
            services = services.filter(service => serviceMatchesTableSearch(service));
        } else {
            // Find mode: show all services, but track which ones match
            matchingServices = services.filter(service => serviceMatchesTableSearch(service));
        }
    }
    
    // Update match count UI (filter mode)
    if (tableSearchState.mode === 'filter') {
        updateTableMatchCountUI(services.length, servicesBeforeSearch);
    }
    
    // Apply sort
    if (servicesState.sortColumn) {
        services = sortServices(services, servicesState.sortColumn, servicesState.sortDirection);
    }
    
    renderServices(services, false, callbacks);
    
    // In find mode, update match tracking after render
    // Skip scrolling when toggling modes to avoid jarring scroll behavior
    if (tableSearchState.term && tableSearchState.mode === 'find') {
        updateTableAllMatches(matchingServices, true);
    }
}

/**
 * Update table match count display (for filter mode).
 * @param {number} matchCount - Number of matching services
 * @param {number} totalCount - Total number of services before search
 */
export function updateTableMatchCountUI(matchCount, totalCount) {
    if (typeof document === 'undefined') return;
    
    // In find mode, table-search.js handles the count display
    if (tableSearchState.mode === 'find') return;
    
    const countEl = document.getElementById('tableMatchCount');
    if (!countEl) return;
    
    if (!tableSearchState.term) {
        countEl.textContent = '';
        countEl.classList.remove('no-matches');
        return;
    }
    
    if (tableSearchState.error) {
        countEl.textContent = 'Invalid';
        countEl.classList.add('no-matches');
        return;
    }
    
    if (matchCount === 0) {
        countEl.textContent = 'No matches';
        countEl.classList.add('no-matches');
    } else {
        countEl.textContent = `${matchCount} of ${totalCount}`;
        countEl.classList.remove('no-matches');
    }
}

/**
 * Update the list of matching services for find mode navigation.
 * @param {Array} matchingServices - Array of services that match the search
 * @param {boolean} skipScroll - If true, don't scroll to the first match
 */
export function updateTableAllMatches(matchingServices, skipScroll = false) {
    if (typeof document === 'undefined') return;
    
    // Clear current match highlight
    document.querySelectorAll('.service-row.current-match').forEach(el => {
        el.classList.remove('current-match');
    });
    
    // Build the matches array with references to DOM rows
    tableSearchState.allMatches = [];
    
    matchingServices.forEach(service => {
        const selector = `tr.service-row[data-service="${CSS.escape(service.name)}"][data-host="${CSS.escape(service.host || '')}"]`;
        const row = document.querySelector(selector);
        if (row) {
            tableSearchState.allMatches.push({ service, row });
        }
    });
    
    // Update the match count UI
    const countEl = document.getElementById('tableMatchCount');
    if (countEl) {
        if (tableSearchState.allMatches.length === 0) {
            countEl.textContent = 'No matches';
            countEl.classList.add('no-matches');
        } else {
            // If no match is selected yet, auto-select the first one
            if (tableSearchState.currentMatchIndex === -1 && tableSearchState.allMatches.length > 0) {
                tableSearchState.currentMatchIndex = 0;
                highlightCurrentTableMatch(skipScroll);
            }
            countEl.textContent = `${tableSearchState.currentMatchIndex + 1} of ${tableSearchState.allMatches.length}`;
            countEl.classList.remove('no-matches');
        }
    }
}

/**
 * Highlight the current match and optionally scroll to it.
 * @param {boolean} skipScroll - If true, don't scroll to the match
 */
function highlightCurrentTableMatch(skipScroll = false) {
    if (tableSearchState.currentMatchIndex < 0 || 
        tableSearchState.currentMatchIndex >= tableSearchState.allMatches.length) return;
    
    const match = tableSearchState.allMatches[tableSearchState.currentMatchIndex];
    if (!match || !match.row) return;
    
    match.row.classList.add('current-match');
    if (!skipScroll) {
        match.row.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }
}
