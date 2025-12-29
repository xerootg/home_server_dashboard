let eventSource = null;
let activeLogsRow = null;
let allServices = [];
let activeFilter = null;
let activeSourceFilter = null;
let sortColumn = null;
let sortDirection = 'asc';
let logsSearchTerm = '';
let logsSearchCaseSensitive = false;
let logsSearchRegex = false;
let logsSearchBangAndPipe = false;
let logsSearchMode = 'filter'; // 'filter' or 'find'
let currentMatchIndex = -1;
let allMatches = [];
let logsSearchError = '';
let bangAndPipeAST = null;
let bangAndPipeDebounceTimer = null;
let helpContentCache = null;

function getStatusClass(state, status) {
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

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function renderServices(services, updateStats = true) {
    // Close any open logs first
    closeLogs();
    
    const tbody = document.getElementById('servicesTable');
    
    if (!services || services.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" class="text-center">No services found</td></tr>';
        return;
    }

    let running = 0;
    let stopped = 0;
    let dockerCount = 0;
    let systemdCount = 0;

    // Count all services for stats (use allServices for accurate counts)
    const statsSource = updateStats ? services : allServices;
    statsSource.forEach(service => {
        if (service.state.toLowerCase() === 'running') {
            running++;
        } else {
            stopped++;
        }
        if (service.source === 'docker') {
            dockerCount++;
        } else if (service.source === 'systemd') {
            systemdCount++;
        }
    });

    const rows = services.map(service => {
        const statusClass = getStatusClass(service.state, service.status);
        const sourceIcon = service.source === 'systemd' ? '<i class="bi bi-gear-fill text-info" title="systemd"></i>' : '<i class="bi bi-box text-primary" title="Docker"></i>';
        const hostBadge = service.host ? `<span class="badge bg-secondary">${escapeHtml(service.host)}</span>` : '';

        return `
            <tr class="service-row" data-container="${escapeHtml(service.container_name)}" data-service="${escapeHtml(service.name)}" data-source="${escapeHtml(service.source || 'docker')}" data-host="${escapeHtml(service.host || '')}">
                <td>${sourceIcon} ${escapeHtml(service.name)}</td>
                <td>${escapeHtml(service.project)}</td>
                <td>${hostBadge}</td>
                <td><code class="small">${escapeHtml(service.container_name)}</code></td>
                <td><span class="badge badge-${statusClass}">${escapeHtml(service.status)}</span></td>
                <td class="image-cell" title="${escapeHtml(service.image)}">${escapeHtml(service.image)}</td>
            </tr>
        `;
    }).join('');

    tbody.innerHTML = rows;

    // Add click handlers
    tbody.querySelectorAll('.service-row').forEach(row => {
        row.addEventListener('click', () => toggleLogs(row));
    });

    // Update stats (always show totals from all services)
    if (updateStats) {
        document.getElementById('totalCount').textContent = services.length;
        document.getElementById('runningCount').textContent = running;
        document.getElementById('stoppedCount').textContent = stopped;
        document.getElementById('dockerCount').innerHTML = '<i class="bi bi-box text-primary"></i> ' + dockerCount;
        document.getElementById('systemdCount').innerHTML = '<i class="bi bi-gear-fill text-info"></i> ' + systemdCount;
    }
}

function toggleFilter(filter) {
    // If clicking the same filter, clear it
    if (activeFilter === filter) {
        activeFilter = null;
    } else {
        activeFilter = filter;
    }
    
    // Update card selection state (only status filters, not source filters)
    document.querySelectorAll('.stat-card:not(.source-filter)').forEach(card => {
        card.classList.remove('active');
    });
    
    if (activeFilter) {
        const activeCard = document.querySelector(`.stat-card[data-filter="${activeFilter}"]`);
        if (activeCard) {
            activeCard.classList.add('active');
        }
    }
    
    // Apply filter
    applyFilter();
}

function toggleSourceFilter(source) {
    // If clicking the same filter, clear it
    if (activeSourceFilter === source) {
        activeSourceFilter = null;
    } else {
        activeSourceFilter = source;
    }
    
    // Update source filter card selection state
    document.querySelectorAll('.stat-card.source-filter').forEach(card => {
        card.classList.remove('active');
    });
    
    if (activeSourceFilter) {
        const activeCard = document.querySelector(`.stat-card[data-source-filter="${activeSourceFilter}"]`);
        if (activeCard) {
            activeCard.classList.add('active');
        }
    }
    
    // Apply filter
    applyFilter();
}

function applyFilter() {
    let services = [...allServices];
    
    // Apply status filter
    if (activeFilter && activeFilter !== 'all') {
        services = services.filter(service => {
            const isRunning = service.state.toLowerCase() === 'running';
            if (activeFilter === 'running') {
                return isRunning;
            } else if (activeFilter === 'stopped') {
                return !isRunning;
            }
            return true;
        });
    }
    
    // Apply source filter
    if (activeSourceFilter) {
        services = services.filter(service => service.source === activeSourceFilter);
    }
    
    // Apply sort
    if (sortColumn) {
        services = sortServices(services, sortColumn, sortDirection);
    }
    
    renderServices(services, false);
}

function sortServices(services, column, direction) {
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

function toggleSort(column) {
    if (sortColumn === column) {
        // Same column: toggle direction
        sortDirection = sortDirection === 'asc' ? 'desc' : 'asc';
    } else {
        // New column: set as sort column with ascending order
        sortColumn = column;
        sortDirection = 'asc';
    }
    
    updateSortIndicators();
    applyFilter();
}

function updateSortIndicators() {
    // Remove all existing indicators
    document.querySelectorAll('th[data-sort] .sort-indicator').forEach(el => {
        el.textContent = '';
    });
    
    // Add indicator to current sort column
    if (sortColumn) {
        const th = document.querySelector(`th[data-sort="${sortColumn}"] .sort-indicator`);
        if (th) {
            th.textContent = sortDirection === 'asc' ? ' ▲' : ' ▼';
        }
    }
}

async function loadServices() {
    try {
        const response = await fetch('/api/services');
        if (!response.ok) {
            throw new Error('Failed to fetch services');
        }
        allServices = await response.json();
        renderServices(allServices);
        
        // Re-apply filter if one is active
        if (activeFilter) {
            applyFilter();
        }
    } catch (error) {
        console.error('Error loading services:', error);
        document.getElementById('servicesTable').innerHTML = 
            '<tr><td colspan="6" class="text-center text-danger">Error loading services</td></tr>';
    }
}

function toggleLogs(row) {
    const containerName = row.dataset.container;
    const serviceName = row.dataset.service;
    const source = row.dataset.source || 'docker';
    const host = row.dataset.host || '';

    // If clicking the same row, close it
    if (activeLogsRow && activeLogsRow.dataset.container === containerName) {
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
    
    // Build logs row HTML (same for both Docker and systemd now)
    const hostInfo = host ? ` (${escapeHtml(host)})` : '';
    logsRow.innerHTML = `
        <td colspan="6">
            <div class="logs-inline">
                <div class="logs-header">
                    <span class="logs-title"><i class="bi bi-journal-text"></i> Logs: ${escapeHtml(serviceName)}${hostInfo}</span>
                    <div class="logs-controls">
                        <div class="logs-search-widget-wrapper">
                            <div class="logs-search-widget">
                                <button class="logs-search-btn" id="logsModeToggle" onclick="toggleLogsSearchMode()" title="Toggle Filter/Find mode">
                                    <i class="bi bi-funnel-fill"></i>
                                </button>
                                <div class="logs-search-input-wrapper">
                                    <input type="text" id="logsSearchInput" class="logs-search-input" placeholder="Search..." oninput="onLogsSearchInput(this.value)" onkeydown="onLogsSearchKeydown(event)">
                                </div>
                                <span class="logs-match-count" id="logsMatchCount"></span>
                                <div class="logs-search-nav" id="logsSearchNav" style="display: none;">
                                    <button class="logs-search-btn" onclick="navigateMatch(-1)" title="Previous match (Shift+Enter)">
                                        <i class="bi bi-chevron-up"></i>
                                    </button>
                                    <button class="logs-search-btn" onclick="navigateMatch(1)" title="Next match (Enter)">
                                        <i class="bi bi-chevron-down"></i>
                                    </button>
                                </div>
                                <button class="logs-search-btn" id="logsCaseToggle" onclick="toggleLogsCaseSensitivity()" title="Match Case">
                                    <span class="case-icon">Aa</span>
                                </button>
                                <button class="logs-search-btn" id="logsRegexToggle" onclick="toggleLogsRegex()" title="Use Regular Expression">
                                    <span class="regex-icon">.*</span>
                                </button>
                                <button class="logs-search-btn" id="logsBangPipeToggle" onclick="toggleLogsBangAndPipe()" title="Bang &amp; Pipe mode: Use !&amp;| operators">
                                    <span class="bangpipe-icon">!&amp;|</span>
                                </button>
                                <button class="logs-search-btn logs-help-btn" id="logsHelpBtn" onclick="showHelpModal()" title="Query language help">
                                    <i class="bi bi-question-circle"></i>
                                </button>
                            </div>
                            <div class="logs-error-popup hidden" id="logsErrorPopup"></div>
                        </div>
                        <span class="logs-status" id="logsStatus">Connecting...</span>
                        <button class="btn btn-sm btn-danger" onclick="closeLogs()">
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
    activeLogsRow = row;

    // Connect to appropriate SSE endpoint
    const content = document.getElementById('logsContent');
    const status = document.getElementById('logsStatus');

    let url;
    if (source === 'systemd') {
        url = '/api/logs/systemd?unit=' + encodeURIComponent(serviceName) + '&host=' + encodeURIComponent(host);
    } else {
        url = '/api/logs?container=' + encodeURIComponent(containerName);
    }

    eventSource = new EventSource(url);

    eventSource.onopen = function() {
        status.textContent = '🟢 Connected';
        status.className = 'logs-status connected';
    };

    eventSource.onmessage = function(event) {
        const line = document.createElement('div');
        line.className = 'log-line';
        line.textContent = event.data;
        line.dataset.originalText = event.data;
        
        content.appendChild(line);
        
        // Apply current search to new line
        if (logsSearchTerm) {
            applySearchToLine(line);
        }
        
        // Auto-scroll to bottom (only in filter mode or if not hidden)
        if (logsSearchMode === 'find' || !line.classList.contains('log-line-hidden')) {
            content.scrollTop = content.scrollHeight;
        }

        // Limit lines to prevent memory issues
        while (content.children.length > 1000) {
            content.removeChild(content.firstChild);
        }
        
        // Update matches and count
        if (logsSearchTerm) {
            updateAllMatches();
        }
    };

    eventSource.onerror = function() {
        status.textContent = '🔴 Disconnected';
        status.className = 'logs-status error';
    };
}

function closeLogs() {
    if (eventSource) {
        eventSource.close();
        eventSource = null;
    }
    
    // Reset search state
    logsSearchTerm = '';
    logsSearchCaseSensitive = false;
    logsSearchRegex = false;
    logsSearchBangAndPipe = false;
    logsSearchMode = 'filter';
    currentMatchIndex = -1;
    allMatches = [];
    logsSearchError = '';
    bangAndPipeAST = null;
    if (bangAndPipeDebounceTimer) {
        clearTimeout(bangAndPipeDebounceTimer);
        bangAndPipeDebounceTimer = null;
    }
    
    // Remove any existing logs row
    const existingLogsRow = document.querySelector('.logs-row');
    if (existingLogsRow) {
        existingLogsRow.remove();
    }
    
    // Remove selected state from all rows
    document.querySelectorAll('.service-row.selected').forEach(row => {
        row.classList.remove('selected');
    });
    
    activeLogsRow = null;
}

function onLogsSearchInput(searchTerm) {
    logsSearchTerm = searchTerm;
    currentMatchIndex = -1;
    
    if (logsSearchBangAndPipe && searchTerm) {
        // Debounce the API call for bang-and-pipe mode
        if (bangAndPipeDebounceTimer) {
            clearTimeout(bangAndPipeDebounceTimer);
        }
        bangAndPipeDebounceTimer = setTimeout(() => {
            compileBangAndPipe(searchTerm);
        }, 150);
    } else {
        bangAndPipeAST = null;
        hideLogsError();
        applySearch();
    }
}

async function compileBangAndPipe(expr) {
    try {
        const response = await fetch('/api/bangAndPipeToRegex?expr=' + encodeURIComponent(expr));
        const result = await response.json();
        
        if (result.valid) {
            bangAndPipeAST = result.ast;
            logsSearchError = '';
            hideLogsError();
        } else {
            bangAndPipeAST = null;
            logsSearchError = result.error.message;
            showLogsError(result.error);
        }
        
        applySearch();
    } catch (e) {
        console.error('Error compiling bang-and-pipe expression:', e);
        bangAndPipeAST = null;
        logsSearchError = 'Failed to compile expression';
        applySearch();
    }
}

function showLogsError(error) {
    const popup = document.getElementById('logsErrorPopup');
    const input = document.getElementById('logsSearchInput');
    if (!popup || !input) return;
    
    // Build error message with position indicator
    const expr = logsSearchTerm;
    let html = '<div class="error-message">' + escapeHtml(error.message) + ' <a href="#" class="error-help-link" onclick="showHelpModal(); return false;">Syntax help</a></div>';
    
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

function hideLogsError() {
    const popup = document.getElementById('logsErrorPopup');
    const input = document.getElementById('logsSearchInput');
    if (popup) popup.classList.add('hidden');
    if (input) input.classList.remove('has-error');
}

async function showHelpModal() {
    const modal = new bootstrap.Modal(document.getElementById('helpModal'));
    const body = document.getElementById('helpModalBody');
    
    modal.show();
    
    // Load content if not cached
    if (!helpContentCache) {
        try {
            const response = await fetch('/api/docs/bangandpipe');
            if (!response.ok) throw new Error('Failed to load documentation');
            helpContentCache = await response.text();
        } catch (e) {
            body.innerHTML = '<div class="alert alert-danger">Failed to load documentation: ' + escapeHtml(e.message) + '</div>';
            return;
        }
    }
    
    body.innerHTML = '<div class="help-content">' + helpContentCache + '</div>';
}

function onLogsSearchKeydown(event) {
    if (event.key === 'Enter' && logsSearchMode === 'find') {
        event.preventDefault();
        if (event.shiftKey) {
            navigateMatch(-1);
        } else {
            navigateMatch(1);
        }
    }
}

function toggleLogsSearchMode() {
    logsSearchMode = logsSearchMode === 'filter' ? 'find' : 'filter';
    updateModeToggleUI();
    currentMatchIndex = -1;
    applySearch();
}

function toggleLogsCaseSensitivity() {
    logsSearchCaseSensitive = !logsSearchCaseSensitive;
    updateCaseToggleUI();
    currentMatchIndex = -1;
    applySearch();
}

function toggleLogsRegex() {
    logsSearchRegex = !logsSearchRegex;
    // Disable bang-and-pipe if enabling regex (they're mutually exclusive)
    if (logsSearchRegex && logsSearchBangAndPipe) {
        logsSearchBangAndPipe = false;
        updateBangPipeToggleUI();
    }
    updateRegexToggleUI();
    currentMatchIndex = -1;
    hideLogsError();
    applySearch();
}

function toggleLogsBangAndPipe() {
    logsSearchBangAndPipe = !logsSearchBangAndPipe;
    // Disable regex if enabling bang-and-pipe (they're mutually exclusive)
    if (logsSearchBangAndPipe && logsSearchRegex) {
        logsSearchRegex = false;
        updateRegexToggleUI();
    }
    updateBangPipeToggleUI();
    currentMatchIndex = -1;
    
    if (logsSearchBangAndPipe && logsSearchTerm) {
        // Compile the current expression
        compileBangAndPipe(logsSearchTerm);
    } else {
        bangAndPipeAST = null;
        hideLogsError();
        applySearch();
    }
}

function updateBangPipeToggleUI() {
    const btn = document.getElementById('logsBangPipeToggle');
    if (!btn) return;
    
    if (logsSearchBangAndPipe) {
        btn.classList.add('active');
        btn.title = 'Bang & Pipe mode enabled: Use ! (not), & (and), | (or), () grouping, "" literals';
    } else {
        btn.classList.remove('active');
        btn.title = 'Bang & Pipe mode: Use !&| operators';
    }
}

function updateRegexToggleUI() {
    const regexBtn = document.getElementById('logsRegexToggle');
    const input = document.getElementById('logsSearchInput');
    if (!regexBtn) return;
    
    if (logsSearchRegex) {
        regexBtn.classList.add('active');
        regexBtn.title = 'Regular expression enabled (click to toggle)';
    } else {
        regexBtn.classList.remove('active');
        regexBtn.title = 'Use Regular Expression (click to toggle)';
    }
    
    // Update input styling for regex errors
    if (input) {
        if (logsSearchError) {
            input.classList.add('has-error');
            input.title = logsSearchError;
        } else {
            input.classList.remove('has-error');
            input.title = '';
        }
    }
}

function updateModeToggleUI() {
    const modeBtn = document.getElementById('logsModeToggle');
    const navBtns = document.getElementById('logsSearchNav');
    const input = document.getElementById('logsSearchInput');
    
    if (!modeBtn) return;
    
    if (logsSearchMode === 'filter') {
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

function updateCaseToggleUI() {
    const caseBtn = document.getElementById('logsCaseToggle');
    if (!caseBtn) return;
    
    if (logsSearchCaseSensitive) {
        caseBtn.classList.add('active');
        caseBtn.title = 'Case sensitive (click to toggle)';
    } else {
        caseBtn.classList.remove('active');
        caseBtn.title = 'Case insensitive (click to toggle)';
    }
}

function applySearch() {
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
    
    if (!logsSearchTerm) {
        allMatches = [];
        updateMatchCountUI();
        return;
    }
    
    // Apply search to each line
    lines.forEach(line => applySearchToLine(line));
    
    // Collect all matches for find mode navigation
    updateAllMatches();
    
    // In find mode, jump to first match
    if (logsSearchMode === 'find' && allMatches.length > 0 && currentMatchIndex === -1) {
        currentMatchIndex = 0;
        highlightCurrentMatch();
    }
}

function applySearchToLine(line) {
    const originalText = line.dataset.originalText || line.textContent;
    line.dataset.originalText = originalText;
    
    const matches = textMatches(originalText, logsSearchTerm);
    
    // Check if we can highlight (not bang-and-pipe mode, or simple pattern)
    const canHighlight = !logsSearchBangAndPipe && !hasInversePrefix();
    
    if (logsSearchMode === 'filter') {
        // Filter mode: hide non-matching lines
        if (!matches) {
            line.classList.add('log-line-hidden');
            line.innerHTML = '';
            line.textContent = originalText;
        } else {
            line.classList.remove('log-line-hidden');
            if (canHighlight) {
                highlightAllInLine(line, logsSearchTerm);
            } else {
                line.innerHTML = '';
                line.textContent = originalText;
            }
        }
    } else {
        // Find mode: show all lines, highlight matches
        line.classList.remove('log-line-hidden');
        if (matches && canHighlight) {
            highlightAllInLine(line, logsSearchTerm);
        }
    }
}

function hasInversePrefix() {
    if (!logsSearchRegex) return false;
    if (logsSearchTerm.startsWith('\\!')) return false;
    return logsSearchTerm.startsWith('!');
}

function textMatches(text, searchTerm) {
    if (!searchTerm) return false;
    
    // Bang-and-pipe mode: use AST evaluation
    if (logsSearchBangAndPipe) {
        if (!bangAndPipeAST) return false;
        return evaluateAST(bangAndPipeAST, text);
    }
    
    // Regex mode with ! prefix for inverse
    if (logsSearchRegex && searchTerm.startsWith('!') && !searchTerm.startsWith('\\!')) {
        const pattern = searchTerm.slice(1);
        if (!pattern) return true; // !empty matches all
        try {
            const flags = logsSearchCaseSensitive ? '' : 'i';
            const regex = new RegExp(pattern, flags);
            return !regex.test(text);
        } catch (e) {
            return false;
        }
    }
    
    // Regex mode with escaped \! 
    let effectivePattern = searchTerm;
    if (logsSearchRegex && searchTerm.startsWith('\\!')) {
        effectivePattern = searchTerm.slice(2);
    }
    
    // Standard regex mode
    if (logsSearchRegex) {
        try {
            const flags = logsSearchCaseSensitive ? '' : 'i';
            const regex = new RegExp(effectivePattern, flags);
            return regex.test(text);
        } catch (e) {
            return false;
        }
    }
    
    // Plain text mode
    if (logsSearchCaseSensitive) {
        return text.includes(searchTerm);
    }
    return text.toLowerCase().includes(searchTerm.toLowerCase());
}

function evaluateAST(ast, text) {
    if (!ast) return false;
    
    switch (ast.type) {
        case 'pattern':
            // Use the regex field from the AST
            try {
                const flags = logsSearchCaseSensitive ? '' : 'i';
                const regex = new RegExp(ast.regex, flags);
                return regex.test(text);
            } catch (e) {
                return false;
            }
        case 'or':
            return ast.children.some(child => evaluateAST(child, text));
        case 'and':
            return ast.children.every(child => evaluateAST(child, text));
        case 'not':
            return !evaluateAST(ast.child, text);
        default:
            return false;
    }
}

function getSearchRegex(searchTerm) {
    if (!searchTerm) return null;
    
    // For bang-and-pipe mode, we don't use regex highlighting
    if (logsSearchBangAndPipe) return null;
    
    // Handle ! prefix in regex mode
    let pattern = searchTerm;
    if (logsSearchRegex) {
        if (searchTerm.startsWith('\\!')) {
            pattern = searchTerm.slice(2);
        } else if (searchTerm.startsWith('!')) {
            pattern = searchTerm.slice(1);
        }
    }
    if (!pattern) return null;
    
    try {
        const flags = logsSearchCaseSensitive ? 'g' : 'gi';
        if (logsSearchRegex) {
            return new RegExp(`(${pattern})`, flags);
        } else {
            const escapedTerm = pattern.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
            return new RegExp(`(${escapedTerm})`, flags);
        }
    } catch (e) {
        return null;
    }
}

function highlightAllInLine(lineElement, searchTerm) {
    const originalText = lineElement.dataset.originalText || lineElement.textContent;
    
    if (!searchTerm) {
        lineElement.textContent = originalText;
        return;
    }
    
    const regex = getSearchRegex(searchTerm);
    if (!regex) {
        lineElement.textContent = originalText;
        return;
    }
    
    const highlighted = escapeHtml(originalText).replace(regex, '<mark class="log-highlight">$1</mark>');
    lineElement.innerHTML = highlighted;
}

function updateAllMatches() {
    allMatches = [];
    logsSearchError = '';
    const content = document.getElementById('logsContent');
    if (!content || !logsSearchTerm) {
        updateMatchCountUI();
        updateRegexToggleUI();
        updateBangPipeToggleUI();
        return;
    }
    
    const lines = content.querySelectorAll('.log-line');
    
    if (logsSearchBangAndPipe) {
        // Bang-and-pipe mode: each matching line is one "match"
        if (!bangAndPipeAST) {
            // No valid AST (syntax error or empty)
            updateMatchCountUI();
            updateRegexToggleUI();
            updateBangPipeToggleUI();
            return;
        }
        
        lines.forEach((line, lineIndex) => {
            const originalText = line.dataset.originalText || line.textContent;
            if (evaluateAST(bangAndPipeAST, originalText)) {
                allMatches.push({ lineIndex, position: 0, length: originalText.length, lineElement: line, isLineMatch: true });
            }
        });
    } else if (logsSearchRegex) {
        // Regex mode
        const hasInverse = hasInversePrefix(logsSearchTerm);
        const pattern = hasInverse ? logsSearchTerm.substring(1) : logsSearchTerm;
        
        if (hasInverse) {
            // Inverse mode: each non-matching line is one "match"
            let regex;
            if (pattern) {
                try {
                    const flags = logsSearchCaseSensitive ? '' : 'i';
                    regex = new RegExp(pattern, flags);
                } catch (e) {
                    logsSearchError = 'Invalid regex: ' + e.message;
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
                    allMatches.push({ lineIndex, position: 0, length: originalText.length, lineElement: line, isLineMatch: true });
                }
            });
        } else {
            // Normal regex mode
            let regex;
            try {
                const flags = logsSearchCaseSensitive ? 'g' : 'gi';
                regex = new RegExp(pattern, flags);
            } catch (e) {
                logsSearchError = 'Invalid regex: ' + e.message;
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
                    allMatches.push({ lineIndex, position: match.index, length: match[0].length, lineElement: line });
                    if (match[0].length === 0) regex.lastIndex++;
                }
            });
        }
    } else {
        // Plain text mode
        lines.forEach((line, lineIndex) => {
            const originalText = line.dataset.originalText || line.textContent;
            const searchStr = logsSearchCaseSensitive ? logsSearchTerm : logsSearchTerm.toLowerCase();
            const textStr = logsSearchCaseSensitive ? originalText : originalText.toLowerCase();
            
            let pos = 0;
            while ((pos = textStr.indexOf(searchStr, pos)) !== -1) {
                allMatches.push({ lineIndex, position: pos, length: searchStr.length, lineElement: line });
                pos += searchStr.length;
            }
        });
    }
    
    // Ensure currentMatchIndex is valid
    if (allMatches.length > 0 && currentMatchIndex >= allMatches.length) {
        currentMatchIndex = allMatches.length - 1;
    }
    
    updateMatchCountUI();
    updateRegexToggleUI();
}

function updateMatchCountUI() {
    const countEl = document.getElementById('logsMatchCount');
    if (!countEl) return;
    
    if (!logsSearchTerm) {
        countEl.textContent = '';
        countEl.classList.remove('no-matches');
        return;
    }
    
    if (logsSearchError) {
        countEl.textContent = 'Invalid regex';
        countEl.classList.add('no-matches');
        return;
    }
    
    if (allMatches.length === 0) {
        countEl.textContent = 'No results';
        countEl.classList.add('no-matches');
    } else if (logsSearchMode === 'find') {
        countEl.textContent = `${currentMatchIndex + 1} of ${allMatches.length}`;
        countEl.classList.remove('no-matches');
    } else {
        // Filter mode - show count of matching lines
        const content = document.getElementById('logsContent');
        const visibleLines = content ? content.querySelectorAll('.log-line:not(.log-line-hidden)').length : 0;
        const totalLines = content ? content.querySelectorAll('.log-line').length : 0;
        countEl.textContent = `${visibleLines} of ${totalLines} lines`;
        countEl.classList.remove('no-matches');
    }
}

function navigateMatch(direction) {
    if (allMatches.length === 0) return;
    
    // Clear current highlight
    document.querySelectorAll('.log-line-current-match').forEach(el => {
        el.classList.remove('log-line-current-match');
    });
    document.querySelectorAll('.log-highlight-current').forEach(el => {
        el.classList.remove('log-highlight-current');
    });
    
    // Update index with wrapping
    currentMatchIndex += direction;
    if (currentMatchIndex < 0) {
        currentMatchIndex = allMatches.length - 1;
    } else if (currentMatchIndex >= allMatches.length) {
        currentMatchIndex = 0;
    }
    
    highlightCurrentMatch();
    updateMatchCountUI();
}

function highlightCurrentMatch() {
    if (currentMatchIndex < 0 || currentMatchIndex >= allMatches.length) return;
    
    const match = allMatches[currentMatchIndex];
    const line = match.lineElement;
    
    // Add current match styling to line
    line.classList.add('log-line-current-match');
    
    // Find and highlight the specific match within the line
    const highlights = line.querySelectorAll('.log-highlight');
    let matchCount = 0;
    const originalText = line.dataset.originalText || '';
    const searchStr = logsSearchCaseSensitive ? logsSearchTerm : logsSearchTerm.toLowerCase();
    const textStr = logsSearchCaseSensitive ? originalText : originalText.toLowerCase();
    
    // Count which occurrence this is in the line
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
    
    // Scroll the line into view
    line.scrollIntoView({ behavior: 'smooth', block: 'center' });
}

// Load services on page load
document.addEventListener('DOMContentLoaded', loadServices);
