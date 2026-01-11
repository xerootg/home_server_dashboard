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

/**
 * Format a log size in bytes to a human-readable string with K/M/G suffixes.
 * @param {number} bytes - Size in bytes
 * @returns {string} - Formatted size string (e.g., "1.5M", "256K", "2.1G")
 */
export function formatLogSize(bytes) {
    if (bytes === 0 || bytes === undefined || bytes === null) {
        return '0';
    }
    
    const units = ['', 'K', 'M', 'G', 'T'];
    let unitIndex = 0;
    let size = bytes;
    
    while (size >= 1024 && unitIndex < units.length - 1) {
        size /= 1024;
        unitIndex++;
    }
    
    // Format with appropriate precision
    if (unitIndex === 0) {
        // Bytes - no decimal
        return `${Math.round(size)}`;
    } else if (size >= 100) {
        // Large numbers - no decimal
        return `${Math.round(size)}${units[unitIndex]}`;
    } else if (size >= 10) {
        // Medium numbers - 1 decimal
        return `${size.toFixed(1)}${units[unitIndex]}`;
    } else {
        // Small numbers - 2 decimals
        return `${size.toFixed(2)}${units[unitIndex]}`;
    }
}
