/**
 * Help modal functionality.
 */

import { helpState } from './state.js';
import { escapeHtml } from './utils.js';

/**
 * Show the help modal with documentation.
 */
export async function showHelpModal() {
    if (typeof document === 'undefined' || typeof bootstrap === 'undefined') return;
    
    const modal = new bootstrap.Modal(document.getElementById('helpModal'));
    const body = document.getElementById('helpModalBody');
    
    modal.show();
    
    // Load content if not cached
    if (!helpState.contentCache) {
        try {
            const response = await fetch('/api/docs/bangandpipe');
            if (!response.ok) throw new Error('Failed to load documentation');
            helpState.contentCache = await response.text();
        } catch (e) {
            body.innerHTML = '<div class="alert alert-danger">Failed to load documentation: ' + escapeHtml(e.message) + '</div>';
            return;
        }
    }
    
    body.innerHTML = '<div class="help-content">' + helpState.contentCache + '</div>';
}
