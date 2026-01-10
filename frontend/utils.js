/**
 * Utility functions for the dashboard.
 * Pure functions with no DOM dependencies - testable in Node.js.
 */

/**
 * Escape HTML special characters to prevent XSS attacks.
 * @param {string} text - The text to escape
 * @returns {string} - HTML-escaped text
 */
export function escapeHtml(text) {
    if (typeof document !== 'undefined') {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
    // Node.js fallback for testing
    return String(text)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

/**
 * Get CSS class for service status badge.
 * @param {string} state - Service state (running, stopped, etc.)
 * @param {string} status - Service status details
 * @returns {string} - CSS class name (running, unhealthy, stopped)
 */
export function getStatusClass(state, status) {
    state = state.toLowerCase();
    status = status.toLowerCase();

    if (state === 'running') {
        if (status.includes('unhealthy')) {
            return 'unhealthy';
        }
        return 'running';
    }
    return 'stopped';
}
