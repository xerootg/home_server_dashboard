/**
 * Service lookup and navigation helpers.
 */

import { servicesState } from './state.js';

/**
 * Find a service's host_ip by name and host.
 * @param {string} serviceName - The service name
 * @param {string} host - The host name
 * @returns {string|null} The host IP or null if not found
 */
export function getServiceHostIP(serviceName, host) {
    const service = servicesState.all.find(s => s.name === serviceName && s.host === host);
    return service ? service.host_ip : null;
}

/**
 * Scroll to a service row in the table and highlight it briefly.
 * @param {string} serviceName - The service name
 * @param {string} host - The host name
 */
export function scrollToService(serviceName, host) {
    if (typeof document === 'undefined') return;
    
    // Find the service row by data attributes
    const selector = `tr.service-row[data-service="${CSS.escape(serviceName)}"][data-host="${CSS.escape(host)}"]`;
    const row = document.querySelector(selector);
    if (row) {
        // Scroll the row into view
        row.scrollIntoView({ behavior: 'smooth', block: 'center' });
        // Add highlight animation (blinks 8 times at 0.4s each = 3.2 seconds)
        row.classList.add('highlight-row');
        setTimeout(() => {
            row.classList.remove('highlight-row');
        }, 3200);
    }
}
