/**
 * Filtering and sorting functions for services.
 */

import { servicesState, tableSearchState } from './state.js';
import { textMatches, evaluateAST } from './search-core.js';
import { renderServices } from './render.js';

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
 * Toggle status filter.
 * @param {string} filter - 'running', 'stopped', or null to clear
 * @param {Object} callbacks - Callback functions
 */
export function toggleFilter(filter, callbacks = {}) {
    if (servicesState.activeFilter === filter) {
        servicesState.activeFilter = null;
    } else {
        servicesState.activeFilter = filter;
    }
    
    if (typeof document !== 'undefined') {
        document.querySelectorAll('.stat-card:not(.source-filter)').forEach(card => {
            card.classList.remove('active');
        });
        
        if (servicesState.activeFilter) {
            const activeCard = document.querySelector(`.stat-card[data-filter="${servicesState.activeFilter}"]`);
            if (activeCard) {
                activeCard.classList.add('active');
            }
        }
    }
    
    applyFilter(callbacks);
}

/**
 * Toggle source filter.
 * @param {string} source - 'docker', 'systemd', 'traefik', or null to clear
 * @param {Object} callbacks - Callback functions
 */
export function toggleSourceFilter(source, callbacks = {}) {
    if (servicesState.activeSourceFilter === source) {
        servicesState.activeSourceFilter = null;
    } else {
        servicesState.activeSourceFilter = source;
    }
    
    if (typeof document !== 'undefined') {
        document.querySelectorAll('.stat-card.source-filter').forEach(card => {
            card.classList.remove('active');
        });
        
        if (servicesState.activeSourceFilter) {
            const activeCard = document.querySelector(`.stat-card[data-source-filter="${servicesState.activeSourceFilter}"]`);
            if (activeCard) {
                activeCard.classList.add('active');
            }
        }
    }
    
    applyFilter(callbacks);
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
    
    // Apply status filter
    if (servicesState.activeFilter && servicesState.activeFilter !== 'all') {
        services = services.filter(service => {
            const isRunning = service.state.toLowerCase() === 'running';
            if (servicesState.activeFilter === 'running') {
                return isRunning;
            } else if (servicesState.activeFilter === 'stopped') {
                return !isRunning;
            }
            return true;
        });
    }
    
    // Apply source filter
    if (servicesState.activeSourceFilter) {
        services = services.filter(service => {
            if (servicesState.activeSourceFilter === 'traefik') {
                return service.source === 'traefik' || 
                       (service.traefik_urls && service.traefik_urls.length > 0);
            }
            return service.source === servicesState.activeSourceFilter;
        });
    }
    
    // Apply table search filter
    const servicesBeforeSearch = services.length;
    if (tableSearchState.term) {
        services = services.filter(service => serviceMatchesTableSearch(service));
    }
    
    // Update match count UI
    updateTableMatchCountUI(services.length, servicesBeforeSearch);
    
    // Apply sort
    if (servicesState.sortColumn) {
        services = sortServices(services, servicesState.sortColumn, servicesState.sortDirection);
    }
    
    renderServices(services, false, callbacks);
}

/**
 * Update table match count display.
 * @param {number} matchCount - Number of matching services
 * @param {number} totalCount - Total number of services before search
 */
export function updateTableMatchCountUI(matchCount, totalCount) {
    if (typeof document === 'undefined') return;
    
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
