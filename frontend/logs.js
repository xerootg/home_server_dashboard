/**
 * Logs viewer functionality.
 */

import { escapeHtml } from './utils.js';
import { logsState, resetLogsState } from './state.js';
import { textMatches, evaluateAST, getSearchRegex, hasInversePrefix, findAllMatches } from './search-core.js';
import { showHelpModal } from './help.js';

/**
 * Toggle logs viewer for a service row.
 * @param {HTMLElement} row - The service table row element
 */
export function toggleLogs(row) {
    const containerName = row.dataset.container;
    const serviceName = row.dataset.service;
    const source = row.dataset.source || 'docker';
    const host = row.dataset.host || '';

    // If clicking the same row, close it
    if (logsState.activeLogsRow && logsState.activeLogsRow.dataset.container === containerName) {
        closeLogs();
        return;
    }

    // Close any existing logs
    closeLogs();

    // Mark row as selected
    row.classList.add('selected');

    // Create the logs row
    const logsRow = document.createElement('tr');
    logsRow.className = 'logs-row';
    
    const hostInfo = host ? ` (${escapeHtml(host)})` : '';
    logsRow.innerHTML = `
        <td colspan="6">
            <div class="logs-inline">
                <div class="logs-header">
                    <span class="logs-title"><i class="bi bi-journal-text"></i> Logs: ${escapeHtml(serviceName)}${hostInfo}</span>
                    <div class="logs-controls">
                        <div class="logs-search-widget-wrapper">
                            <div class="logs-search-widget">
                                <button class="logs-search-btn" id="logsModeToggle" onclick="window.__dashboard.toggleLogsSearchMode()" title="Toggle Filter/Find mode">
                                    <i class="bi bi-funnel-fill"></i>
                                </button>
                                <div class="logs-search-input-wrapper">
                                    <input type="text" id="logsSearchInput" class="logs-search-input" placeholder="Search..." oninput="window.__dashboard.onLogsSearchInput(this.value)" onkeydown="window.__dashboard.onLogsSearchKeydown(event)">
                                </div>
                                <span class="logs-match-count" id="logsMatchCount"></span>
                                <div class="logs-search-nav" id="logsSearchNav" style="display: none;">
                                    <button class="logs-search-btn" onclick="window.__dashboard.navigateMatch(-1)" title="Previous match (Shift+Enter)">
                                        <i class="bi bi-chevron-up"></i>
                                    </button>
                                    <button class="logs-search-btn" onclick="window.__dashboard.navigateMatch(1)" title="Next match (Enter)">
                                        <i class="bi bi-chevron-down"></i>
                                    </button>
                                </div>
                                <button class="logs-search-btn" id="logsCaseToggle" onclick="window.__dashboard.toggleLogsCaseSensitivity()" title="Match Case">
                                    <span class="case-icon">Aa</span>
                                </button>
                                <button class="logs-search-btn" id="logsRegexToggle" onclick="window.__dashboard.toggleLogsRegex()" title="Use Regular Expression">
                                    <span class="regex-icon">.*</span>
                                </button>
                                <button class="logs-search-btn" id="logsBangPipeToggle" onclick="window.__dashboard.toggleLogsBangAndPipe()" title="Bang &amp; Pipe mode: Use !&amp;| operators">
                                    <span class="bangpipe-icon">!&amp;|</span>
                                </button>
                                <button class="logs-search-btn logs-help-btn" id="logsHelpBtn" onclick="window.__dashboard.showHelpModal()" title="Query language help">
                                    <i class="bi bi-question-circle"></i>
                                </button>
                            </div>
                            <div class="logs-error-popup hidden" id="logsErrorPopup"></div>
                        </div>
                        <span class="logs-status" id="logsStatus">Connecting...</span>
                        <button class="btn btn-sm btn-danger" onclick="window.__dashboard.closeLogs()">
                            <i class="bi bi-x-lg"></i>
                        </button>
                    </div>
                </div>
                <div class="logs-content" id="logsContent"></div>
            </div>
        </td>
    `;

    // Insert after the clicked row
    row.after(logsRow);
    logsState.activeLogsRow = row;
    
    // Initialize the Bang & Pipe toggle button UI state
    updateBangPipeToggleUI();

    // Connect to appropriate SSE endpoint
    const content = document.getElementById('logsContent');
    const status = document.getElementById('logsStatus');

    let url;
    if (source === 'systemd') {
        url = '/api/logs/systemd?unit=' + encodeURIComponent(serviceName) + '&host=' + encodeURIComponent(host);
    } else if (source === 'traefik') {
        url = '/api/logs/traefik?service=' + encodeURIComponent(serviceName) + '&host=' + encodeURIComponent(host);
    } else if (source === 'homeassistant' || source === 'homeassistant-addon') {
        url = '/api/logs/homeassistant?service=' + encodeURIComponent(serviceName) + '&host=' + encodeURIComponent(host);
    } else {
        url = '/api/logs?container=' + encodeURIComponent(containerName) + '&service=' + encodeURIComponent(serviceName);
    }

    logsState.eventSource = new EventSource(url);

    logsState.eventSource.onopen = function() {
        status.textContent = '🟢 Connected';
        status.className = 'logs-status connected';
    };

    logsState.eventSource.onmessage = function(event) {
        const line = document.createElement('div');
        line.className = 'log-line';
        line.textContent = event.data;
        line.dataset.originalText = event.data;
        
        content.appendChild(line);
        
        // Apply current search to new line
        if (logsState.searchTerm) {
            applySearchToLine(line);
        }
        
        // Auto-scroll to bottom (only in filter mode or if not hidden)
        if (logsState.mode === 'find' || !line.classList.contains('log-line-hidden')) {
            content.scrollTop = content.scrollHeight;
        }

        // Limit lines to prevent memory issues
        while (content.children.length > 1000) {
            content.removeChild(content.firstChild);
        }
        
        // Update matches and count
        if (logsState.searchTerm) {
            updateAllMatches();
        }
    };

    logsState.eventSource.onerror = function() {
        status.textContent = '🔴 Disconnected';
        status.className = 'logs-status error';
    };
}

/**
 * Close the logs viewer.
 */
export function closeLogs() {
    if (logsState.eventSource) {
        logsState.eventSource.close();
    }
    
    resetLogsState();
    
    // Remove any existing logs row
    const existingLogsRow = document.querySelector('.logs-row');
    if (existingLogsRow) {
        existingLogsRow.remove();
    }
    
    // Remove selected state from all rows
    document.querySelectorAll('.service-row.selected').forEach(row => {
        row.classList.remove('selected');
    });
}

/**
 * Handle logs search input.
 * @param {string} searchTerm - The search term
 */
export function onLogsSearchInput(searchTerm) {
    logsState.searchTerm = searchTerm;
    logsState.currentMatchIndex = -1;
    
    if (logsState.bangAndPipe && searchTerm) {
        // Debounce the API call for bang-and-pipe mode
        if (logsState.debounceTimer) {
            clearTimeout(logsState.debounceTimer);
        }
        logsState.debounceTimer = setTimeout(() => {
            compileBangAndPipe(searchTerm);
        }, 150);
    } else {
        logsState.ast = null;
        hideLogsError();
        applySearch();
    }
}

/**
 * Compile Bang & Pipe expression via API.
 * @param {string} expr - The expression to compile
 */
export async function compileBangAndPipe(expr) {
    try {
        const response = await fetch('/api/bangAndPipeToRegex?expr=' + encodeURIComponent(expr));
        const result = await response.json();
        
        if (result.valid) {
            logsState.ast = result.ast;
            logsState.error = '';
            hideLogsError();
        } else {
            logsState.ast = null;
            logsState.error = result.error.message;
            showLogsError(result.error);
        }
        
        applySearch();
    } catch (e) {
        console.error('Error compiling bang-and-pipe expression:', e);
        logsState.ast = null;
        logsState.error = 'Failed to compile expression';
        applySearch();
    }
}

/**
 * Show logs error popup.
 * @param {Object} error - Error object with message, position, length
 */
export function showLogsError(error) {
    const popup = document.getElementById('logsErrorPopup');
    const input = document.getElementById('logsSearchInput');
    if (!popup || !input) return;
    
    const expr = logsState.searchTerm;
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
 * Hide logs error popup.
 */
export function hideLogsError() {
    const popup = document.getElementById('logsErrorPopup');
    const input = document.getElementById('logsSearchInput');
    if (popup) popup.classList.add('hidden');
    if (input) input.classList.remove('has-error');
}

/**
 * Handle keydown in logs search input.
 * @param {KeyboardEvent} event - The keyboard event
 */
export function onLogsSearchKeydown(event) {
    if (event.key === 'Enter' && logsState.mode === 'find') {
        event.preventDefault();
        if (event.shiftKey) {
            navigateMatch(-1);
        } else {
            navigateMatch(1);
        }
    }
}

/**
 * Toggle between filter and find mode.
 */
export function toggleLogsSearchMode() {
    logsState.mode = logsState.mode === 'filter' ? 'find' : 'filter';
    updateModeToggleUI();
    logsState.currentMatchIndex = -1;
    applySearch();
}

/**
 * Toggle case sensitivity.
 */
export function toggleLogsCaseSensitivity() {
    logsState.caseSensitive = !logsState.caseSensitive;
    updateCaseToggleUI();
    logsState.currentMatchIndex = -1;
    applySearch();
}

/**
 * Toggle regex mode.
 */
export function toggleLogsRegex() {
    logsState.regex = !logsState.regex;
    if (logsState.regex && logsState.bangAndPipe) {
        logsState.bangAndPipe = false;
        updateBangPipeToggleUI();
    }
    updateRegexToggleUI();
    logsState.currentMatchIndex = -1;
    hideLogsError();
    applySearch();
}

/**
 * Toggle Bang & Pipe mode.
 */
export function toggleLogsBangAndPipe() {
    logsState.bangAndPipe = !logsState.bangAndPipe;
    if (logsState.bangAndPipe && logsState.regex) {
        logsState.regex = false;
        updateRegexToggleUI();
    }
    updateBangPipeToggleUI();
    logsState.currentMatchIndex = -1;
    
    if (logsState.bangAndPipe && logsState.searchTerm) {
        compileBangAndPipe(logsState.searchTerm);
    } else {
        logsState.ast = null;
        hideLogsError();
        applySearch();
    }
}

/**
 * Update Bang & Pipe toggle button UI.
 */
export function updateBangPipeToggleUI() {
    const btn = document.getElementById('logsBangPipeToggle');
    if (!btn) return;
    
    if (logsState.bangAndPipe) {
        btn.classList.add('active');
        btn.title = 'Bang & Pipe mode enabled: Use ! (not), & (and), | (or), () grouping, "" literals';
    } else {
        btn.classList.remove('active');
        btn.title = 'Bang & Pipe mode: Use !&| operators';
    }
}

/**
 * Update regex toggle button UI.
 */
export function updateRegexToggleUI() {
    const regexBtn = document.getElementById('logsRegexToggle');
    const input = document.getElementById('logsSearchInput');
    if (!regexBtn) return;
    
    if (logsState.regex) {
        regexBtn.classList.add('active');
        regexBtn.title = 'Regular expression enabled (click to toggle)';
    } else {
        regexBtn.classList.remove('active');
        regexBtn.title = 'Use Regular Expression (click to toggle)';
    }
    
    if (input) {
        if (logsState.error) {
            input.classList.add('has-error');
            input.title = logsState.error;
        } else {
            input.classList.remove('has-error');
            input.title = '';
        }
    }
}

/**
 * Update mode toggle button UI.
 */
export function updateModeToggleUI() {
    const modeBtn = document.getElementById('logsModeToggle');
    const navBtns = document.getElementById('logsSearchNav');
    const input = document.getElementById('logsSearchInput');
    
    if (!modeBtn) return;
    
    if (logsState.mode === 'filter') {
        modeBtn.innerHTML = '<i class="bi bi-funnel-fill"></i>';
        modeBtn.title = 'Filter mode - hiding non-matching lines. Click to switch to Find mode.';
        modeBtn.classList.remove('active');
        if (navBtns) navBtns.style.display = 'none';
        if (input) input.placeholder = 'Filter...';
    } else {
        modeBtn.innerHTML = '<i class="bi bi-search"></i>';
        modeBtn.title = 'Find mode - jump between matches. Click to switch to Filter mode.';
        modeBtn.classList.add('active');
        if (navBtns) navBtns.style.display = 'flex';
        if (input) input.placeholder = 'Find...';
    }
}

/**
 * Update case toggle button UI.
 */
export function updateCaseToggleUI() {
    const caseBtn = document.getElementById('logsCaseToggle');
    if (!caseBtn) return;
    
    if (logsState.caseSensitive) {
        caseBtn.classList.add('active');
        caseBtn.title = 'Case sensitive (click to toggle)';
    } else {
        caseBtn.classList.remove('active');
        caseBtn.title = 'Case insensitive (click to toggle)';
    }
}

/**
 * Apply search to all log lines.
 */
export function applySearch() {
    const content = document.getElementById('logsContent');
    if (!content) return;
    
    const lines = content.querySelectorAll('.log-line');
    
    // Clear all highlights and visibility first
    lines.forEach(line => {
        line.classList.remove('log-line-hidden', 'log-line-current-match');
        const originalText = line.dataset.originalText || line.textContent;
        line.dataset.originalText = originalText;
        line.innerHTML = '';
        line.textContent = originalText;
    });
    
    if (!logsState.searchTerm) {
        logsState.allMatches = [];
        updateMatchCountUI();
        return;
    }
    
    // Apply search to each line
    lines.forEach(line => applySearchToLine(line));
    
    // Collect all matches for find mode navigation
    updateAllMatches();
    
    // In find mode, jump to first match
    if (logsState.mode === 'find' && logsState.allMatches.length > 0 && logsState.currentMatchIndex === -1) {
        logsState.currentMatchIndex = 0;
        highlightCurrentMatch();
    }
}

/**
 * Apply search to a single log line.
 * @param {HTMLElement} line - The log line element
 */
export function applySearchToLine(line) {
    const originalText = line.dataset.originalText || line.textContent;
    line.dataset.originalText = originalText;
    
    const matches = textMatches(originalText, logsState.searchTerm, {
        caseSensitive: logsState.caseSensitive,
        regex: logsState.regex,
        bangAndPipe: logsState.bangAndPipe,
        ast: logsState.ast
    });
    
    // Check if we can highlight (not bang-and-pipe mode, or simple pattern)
    const canHighlight = !logsState.bangAndPipe && !hasInversePrefix(logsState.searchTerm, logsState.regex);
    
    if (logsState.mode === 'filter') {
        // Filter mode: hide non-matching lines
        if (!matches) {
            line.classList.add('log-line-hidden');
            line.innerHTML = '';
            line.textContent = originalText;
        } else {
            line.classList.remove('log-line-hidden');
            if (canHighlight) {
                highlightAllInLine(line, logsState.searchTerm);
            } else {
                line.innerHTML = '';
                line.textContent = originalText;
            }
        }
    } else {
        // Find mode: show all lines, highlight matches
        line.classList.remove('log-line-hidden');
        if (matches && canHighlight) {
            highlightAllInLine(line, logsState.searchTerm);
        }
    }
}

/**
 * Highlight all matches in a line.
 * @param {HTMLElement} lineElement - The log line element
 * @param {string} searchTerm - The search term
 */
export function highlightAllInLine(lineElement, searchTerm) {
    const originalText = lineElement.dataset.originalText || lineElement.textContent;
    
    if (!searchTerm) {
        lineElement.textContent = originalText;
        return;
    }
    
    const regex = getSearchRegex(searchTerm, {
        caseSensitive: logsState.caseSensitive,
        regex: logsState.regex,
        bangAndPipe: logsState.bangAndPipe
    });
    
    if (!regex) {
        lineElement.textContent = originalText;
        return;
    }
    
    const highlighted = escapeHtml(originalText).replace(regex, '<mark class="log-highlight">$1</mark>');
    lineElement.innerHTML = highlighted;
}

/**
 * Update all matches array for navigation.
 */
export function updateAllMatches() {
    logsState.allMatches = [];
    logsState.error = '';
    const content = document.getElementById('logsContent');
    if (!content || !logsState.searchTerm) {
        updateMatchCountUI();
        updateRegexToggleUI();
        updateBangPipeToggleUI();
        return;
    }
    
    const lines = content.querySelectorAll('.log-line');
    
    if (logsState.bangAndPipe) {
        if (!logsState.ast) {
            updateMatchCountUI();
            updateRegexToggleUI();
            updateBangPipeToggleUI();
            return;
        }
        
        lines.forEach((line, lineIndex) => {
            const originalText = line.dataset.originalText || line.textContent;
            if (evaluateAST(logsState.ast, originalText, logsState.caseSensitive)) {
                logsState.allMatches.push({ lineIndex, position: 0, length: originalText.length, lineElement: line, isLineMatch: true });
            }
        });
    } else if (logsState.regex) {
        const hasInverse = hasInversePrefix(logsState.searchTerm, logsState.regex);
        const pattern = hasInverse ? logsState.searchTerm.substring(1) : logsState.searchTerm;
        
        if (hasInverse) {
            let regex;
            if (pattern) {
                try {
                    const flags = logsState.caseSensitive ? '' : 'i';
                    regex = new RegExp(pattern, flags);
                } catch (e) {
                    logsState.error = 'Invalid regex: ' + e.message;
                    updateMatchCountUI();
                    updateRegexToggleUI();
                    updateBangPipeToggleUI();
                    return;
                }
            }
            
            lines.forEach((line, lineIndex) => {
                const originalText = line.dataset.originalText || line.textContent;
                const matches = pattern ? regex.test(originalText) : false;
                if (!matches) {
                    logsState.allMatches.push({ lineIndex, position: 0, length: originalText.length, lineElement: line, isLineMatch: true });
                }
            });
        } else {
            let regex;
            try {
                const flags = logsState.caseSensitive ? 'g' : 'gi';
                regex = new RegExp(pattern, flags);
            } catch (e) {
                logsState.error = 'Invalid regex: ' + e.message;
                updateMatchCountUI();
                updateRegexToggleUI();
                updateBangPipeToggleUI();
                return;
            }
            
            lines.forEach((line, lineIndex) => {
                const originalText = line.dataset.originalText || line.textContent;
                let match;
                regex.lastIndex = 0;
                while ((match = regex.exec(originalText)) !== null) {
                    logsState.allMatches.push({ lineIndex, position: match.index, length: match[0].length, lineElement: line });
                    if (match[0].length === 0) regex.lastIndex++;
                }
            });
        }
    } else {
        lines.forEach((line, lineIndex) => {
            const originalText = line.dataset.originalText || line.textContent;
            const searchStr = logsState.caseSensitive ? logsState.searchTerm : logsState.searchTerm.toLowerCase();
            const textStr = logsState.caseSensitive ? originalText : originalText.toLowerCase();
            
            let pos = 0;
            while ((pos = textStr.indexOf(searchStr, pos)) !== -1) {
                logsState.allMatches.push({ lineIndex, position: pos, length: searchStr.length, lineElement: line });
                pos += searchStr.length;
            }
        });
    }
    
    // Ensure currentMatchIndex is valid
    if (logsState.allMatches.length > 0 && logsState.currentMatchIndex >= logsState.allMatches.length) {
        logsState.currentMatchIndex = logsState.allMatches.length - 1;
    }
    
    updateMatchCountUI();
    updateRegexToggleUI();
}

/**
 * Update match count display.
 */
export function updateMatchCountUI() {
    const countEl = document.getElementById('logsMatchCount');
    if (!countEl) return;
    
    if (!logsState.searchTerm) {
        countEl.textContent = '';
        countEl.classList.remove('no-matches');
        return;
    }
    
    if (logsState.error) {
        countEl.textContent = 'Invalid regex';
        countEl.classList.add('no-matches');
        return;
    }
    
    if (logsState.allMatches.length === 0) {
        countEl.textContent = 'No results';
        countEl.classList.add('no-matches');
    } else if (logsState.mode === 'find') {
        countEl.textContent = `${logsState.currentMatchIndex + 1} of ${logsState.allMatches.length}`;
        countEl.classList.remove('no-matches');
    } else {
        const content = document.getElementById('logsContent');
        const visibleLines = content ? content.querySelectorAll('.log-line:not(.log-line-hidden)').length : 0;
        const totalLines = content ? content.querySelectorAll('.log-line').length : 0;
        countEl.textContent = `${visibleLines} of ${totalLines} lines`;
        countEl.classList.remove('no-matches');
    }
}

/**
 * Navigate to next/previous match.
 * @param {number} direction - 1 for next, -1 for previous
 */
export function navigateMatch(direction) {
    if (logsState.allMatches.length === 0) return;
    
    // Clear current highlight
    document.querySelectorAll('.log-line-current-match').forEach(el => {
        el.classList.remove('log-line-current-match');
    });
    document.querySelectorAll('.log-highlight-current').forEach(el => {
        el.classList.remove('log-highlight-current');
    });
    
    // Update index with wrapping
    logsState.currentMatchIndex += direction;
    if (logsState.currentMatchIndex < 0) {
        logsState.currentMatchIndex = logsState.allMatches.length - 1;
    } else if (logsState.currentMatchIndex >= logsState.allMatches.length) {
        logsState.currentMatchIndex = 0;
    }
    
    highlightCurrentMatch();
    updateMatchCountUI();
}

/**
 * Highlight the current match.
 */
export function highlightCurrentMatch() {
    if (logsState.currentMatchIndex < 0 || logsState.currentMatchIndex >= logsState.allMatches.length) return;
    
    const match = logsState.allMatches[logsState.currentMatchIndex];
    const line = match.lineElement;
    
    line.classList.add('log-line-current-match');
    
    const highlights = line.querySelectorAll('.log-highlight');
    const originalText = line.dataset.originalText || '';
    const searchStr = logsState.caseSensitive ? logsState.searchTerm : logsState.searchTerm.toLowerCase();
    const textStr = logsState.caseSensitive ? originalText : originalText.toLowerCase();
    
    let pos = 0;
    let occurrenceIndex = 0;
    while ((pos = textStr.indexOf(searchStr, pos)) !== -1) {
        if (pos === match.position) {
            break;
        }
        occurrenceIndex++;
        pos += searchStr.length;
    }
    
    if (highlights[occurrenceIndex]) {
        highlights[occurrenceIndex].classList.add('log-highlight-current');
    }
    
    line.scrollIntoView({ behavior: 'smooth', block: 'center' });
}
