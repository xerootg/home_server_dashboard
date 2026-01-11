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
 * @param {Object} callbacks - Callback functions
 */
export function onTableSearchKeydown(event, callbacks = {}) {
    if (event.key === 'Escape') {
        clearTableSearch(callbacks);
    } else if (event.key === 'Enter' && tableSearchState.mode === 'find') {
        event.preventDefault();
        if (event.shiftKey) {
            navigateTableMatch(-1);
        } else {
            navigateTableMatch(1);
        }
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
    tableSearchState.currentMatchIndex = -1;
    tableSearchState.allMatches = [];
    
    const input = document.getElementById('tableSearchInput');
    if (input) input.value = '';
    
    const clearBtn = document.getElementById('tableClearBtn');
    if (clearBtn) clearBtn.style.display = 'none';
    
    clearCurrentMatchHighlight();
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

/**
 * Toggle between filter and find mode.
 * Preserves scroll position to avoid jarring jumps.
 * @param {Object} callbacks - Callback functions
 */
export function toggleTableSearchMode(callbacks = {}) {
    // Store scroll position before mode change
    const scrollY = typeof window !== 'undefined' ? window.scrollY : 0;
    
    tableSearchState.mode = tableSearchState.mode === 'filter' ? 'find' : 'filter';
    tableSearchState.currentMatchIndex = -1;
    updateTableModeToggleUI();
    applyFilter(callbacks);
    
    // Restore scroll position after DOM updates
    if (typeof window !== 'undefined' && typeof document !== 'undefined') {
        // Use requestAnimationFrame to ensure DOM has updated
        requestAnimationFrame(() => {
            const newDocumentHeight = document.documentElement.scrollHeight;
            // If we would be past the new bottom, scroll to bottom instead
            const maxScroll = newDocumentHeight - window.innerHeight;
            const targetScroll = Math.min(scrollY, Math.max(0, maxScroll));
            window.scrollTo(0, targetScroll);
        });
    }
}

/**
 * Update mode toggle button UI.
 */
export function updateTableModeToggleUI() {
    const modeBtn = document.getElementById('tableModeToggle');
    const navBtns = document.getElementById('tableSearchNav');
    const input = document.getElementById('tableSearchInput');
    
    if (!modeBtn) return;
    
    if (tableSearchState.mode === 'filter') {
        modeBtn.innerHTML = '<i class="bi bi-funnel-fill"></i>';
        modeBtn.title = 'Filter mode - hiding non-matching services. Click to switch to Find mode.';
        modeBtn.classList.remove('active');
        if (navBtns) navBtns.style.display = 'none';
        if (input) input.placeholder = 'Search services...';
    } else {
        modeBtn.innerHTML = '<i class="bi bi-search"></i>';
        modeBtn.title = 'Find mode - jump between matches. Click to switch to Filter mode.';
        modeBtn.classList.add('active');
        if (navBtns) navBtns.style.display = 'flex';
        if (input) input.placeholder = 'Find services...';
    }
}

/**
 * Navigate to next/previous match in find mode.
 * @param {number} direction - 1 for next, -1 for previous
 */
export function navigateTableMatch(direction) {
    if (tableSearchState.allMatches.length === 0) return;
    
    // Clear current highlight
    clearCurrentMatchHighlight();
    
    // Update index with wrapping
    tableSearchState.currentMatchIndex += direction;
    if (tableSearchState.currentMatchIndex < 0) {
        tableSearchState.currentMatchIndex = tableSearchState.allMatches.length - 1;
    } else if (tableSearchState.currentMatchIndex >= tableSearchState.allMatches.length) {
        tableSearchState.currentMatchIndex = 0;
    }
    
    highlightCurrentTableMatch();
    updateTableMatchCountUI();
}

/**
 * Clear current match highlight from service rows.
 */
export function clearCurrentMatchHighlight() {
    if (typeof document === 'undefined') return;
    document.querySelectorAll('.service-row.current-match').forEach(el => {
        el.classList.remove('current-match');
    });
}

/**
 * Highlight the current match and scroll to it.
 */
export function highlightCurrentTableMatch() {
    if (tableSearchState.currentMatchIndex < 0 || 
        tableSearchState.currentMatchIndex >= tableSearchState.allMatches.length) return;
    
    const match = tableSearchState.allMatches[tableSearchState.currentMatchIndex];
    if (!match || !match.row) return;
    
    match.row.classList.add('current-match');
    match.row.scrollIntoView({ behavior: 'smooth', block: 'center' });
}

/**
 * Update match count display for find mode.
 */
export function updateTableMatchCountUI() {
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
    
    if (tableSearchState.mode === 'find') {
        if (tableSearchState.allMatches.length === 0) {
            countEl.textContent = 'No matches';
            countEl.classList.add('no-matches');
        } else {
            countEl.textContent = `${tableSearchState.currentMatchIndex + 1} of ${tableSearchState.allMatches.length}`;
            countEl.classList.remove('no-matches');
        }
    }
    // Filter mode count is handled by filter.js updateTableMatchCountUI
}
