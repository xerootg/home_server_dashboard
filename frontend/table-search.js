/**
 * Table search functionality.
 */

import { escapeHtml } from './utils.js';
import { tableSearchState } from './state.js';
import { applyFilter } from './filter.js';
import { showHelpModal } from './help.js';

/**
 * Handle table search input.
 * @param {string} searchTerm - The search term
 * @param {Object} callbacks - Callback functions
 */
export function onTableSearchInput(searchTerm, callbacks = {}) {
    tableSearchState.term = searchTerm;
    
    // Show/hide clear button
    const clearBtn = document.getElementById('tableClearBtn');
    if (clearBtn) {
        clearBtn.style.display = searchTerm ? 'flex' : 'none';
    }
    
    if (tableSearchState.bangAndPipe && searchTerm) {
        // Debounce the API call for bang-and-pipe mode
        if (tableSearchState.debounceTimer) {
            clearTimeout(tableSearchState.debounceTimer);
        }
        tableSearchState.debounceTimer = setTimeout(() => {
            compileTableBangAndPipe(searchTerm, callbacks);
        }, 150);
    } else {
        tableSearchState.ast = null;
        hideTableError();
        applyFilter(callbacks);
    }
}

/**
 * Handle keydown in table search input.
 * @param {KeyboardEvent} event - The keyboard event
 */
export function onTableSearchKeydown(event) {
    if (event.key === 'Escape') {
        clearTableSearch();
    }
}

/**
 * Clear table search.
 * @param {Object} callbacks - Callback functions
 */
export function clearTableSearch(callbacks = {}) {
    tableSearchState.term = '';
    tableSearchState.ast = null;
    tableSearchState.error = '';
    
    const input = document.getElementById('tableSearchInput');
    if (input) input.value = '';
    
    const clearBtn = document.getElementById('tableClearBtn');
    if (clearBtn) clearBtn.style.display = 'none';
    
    hideTableError();
    applyFilter(callbacks);
}

/**
 * Compile Bang & Pipe expression via API.
 * @param {string} expr - The expression to compile
 * @param {Object} callbacks - Callback functions
 */
export async function compileTableBangAndPipe(expr, callbacks = {}) {
    try {
        const response = await fetch('/api/bangAndPipeToRegex?expr=' + encodeURIComponent(expr));
        const result = await response.json();
        
        if (result.valid) {
            tableSearchState.ast = result.ast;
            tableSearchState.error = '';
            hideTableError();
        } else {
            tableSearchState.ast = null;
            tableSearchState.error = result.error.message;
            showTableError(result.error);
        }
        
        applyFilter(callbacks);
    } catch (e) {
        console.error('Error compiling bang-and-pipe expression:', e);
        tableSearchState.ast = null;
        tableSearchState.error = 'Failed to compile expression';
        applyFilter(callbacks);
    }
}

/**
 * Show table search error popup.
 * @param {Object} error - Error object with message, position, length
 */
export function showTableError(error) {
    const popup = document.getElementById('tableErrorPopup');
    const input = document.getElementById('tableSearchInput');
    if (!popup || !input) return;
    
    const expr = tableSearchState.term;
    let html = '<div class="error-message">' + escapeHtml(error.message) + ' <a href="#" class="error-help-link" onclick="window.__dashboard.showHelpModal(); return false;">Syntax help</a></div>';
    
    if (error.position !== undefined && expr) {
        const before = escapeHtml(expr.substring(0, error.position));
        const errorPart = escapeHtml(expr.substring(error.position, error.position + (error.length || 1)));
        const after = escapeHtml(expr.substring(error.position + (error.length || 1)));
        html += '<div class="error-expr"><code>' + before + '<span class="error-highlight">' + (errorPart || '▯') + '</span>' + after + '</code></div>';
    }
    
    popup.innerHTML = html;
    popup.classList.remove('hidden');
    input.classList.add('has-error');
}

/**
 * Hide table search error popup.
 */
export function hideTableError() {
    const popup = document.getElementById('tableErrorPopup');
    const input = document.getElementById('tableSearchInput');
    if (popup) popup.classList.add('hidden');
    if (input) input.classList.remove('has-error');
}

/**
 * Toggle case sensitivity.
 * @param {Object} callbacks - Callback functions
 */
export function toggleTableCaseSensitivity(callbacks = {}) {
    tableSearchState.caseSensitive = !tableSearchState.caseSensitive;
    updateTableCaseToggleUI();
    applyFilter(callbacks);
}

/**
 * Toggle regex mode.
 * @param {Object} callbacks - Callback functions
 */
export function toggleTableRegex(callbacks = {}) {
    tableSearchState.regex = !tableSearchState.regex;
    if (tableSearchState.regex && tableSearchState.bangAndPipe) {
        tableSearchState.bangAndPipe = false;
        updateTableBangPipeToggleUI();
    }
    updateTableRegexToggleUI();
    hideTableError();
    applyFilter(callbacks);
}

/**
 * Toggle Bang & Pipe mode.
 * @param {Object} callbacks - Callback functions
 */
export function toggleTableBangAndPipe(callbacks = {}) {
    tableSearchState.bangAndPipe = !tableSearchState.bangAndPipe;
    if (tableSearchState.bangAndPipe && tableSearchState.regex) {
        tableSearchState.regex = false;
        updateTableRegexToggleUI();
    }
    updateTableBangPipeToggleUI();
    
    if (tableSearchState.bangAndPipe && tableSearchState.term) {
        compileTableBangAndPipe(tableSearchState.term, callbacks);
    } else {
        tableSearchState.ast = null;
        hideTableError();
        applyFilter(callbacks);
    }
}

/**
 * Update case toggle button UI.
 */
export function updateTableCaseToggleUI() {
    const caseBtn = document.getElementById('tableCaseToggle');
    if (!caseBtn) return;
    
    if (tableSearchState.caseSensitive) {
        caseBtn.classList.add('active');
        caseBtn.title = 'Case sensitive (click to toggle)';
    } else {
        caseBtn.classList.remove('active');
        caseBtn.title = 'Case insensitive (click to toggle)';
    }
}

/**
 * Update regex toggle button UI.
 */
export function updateTableRegexToggleUI() {
    const regexBtn = document.getElementById('tableRegexToggle');
    const input = document.getElementById('tableSearchInput');
    if (!regexBtn) return;
    
    if (tableSearchState.regex) {
        regexBtn.classList.add('active');
        regexBtn.title = 'Regular expression enabled (click to toggle)';
    } else {
        regexBtn.classList.remove('active');
        regexBtn.title = 'Use Regular Expression (click to toggle)';
    }
    
    if (input) {
        if (tableSearchState.error) {
            input.classList.add('has-error');
            input.title = tableSearchState.error;
        } else {
            input.classList.remove('has-error');
            input.title = '';
        }
    }
}

/**
 * Update Bang & Pipe toggle button UI.
 */
export function updateTableBangPipeToggleUI() {
    const btn = document.getElementById('tableBangPipeToggle');
    if (!btn) return;
    
    if (tableSearchState.bangAndPipe) {
        btn.classList.add('active');
        btn.title = 'Bang & Pipe mode enabled: Use ! (not), & (and), | (or), () grouping, "" literals';
    } else {
        btn.classList.remove('active');
        btn.title = 'Bang & Pipe mode: Use !&| operators';
    }
}
