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

// Table search state
let tableSearchTerm = '';
let tableSearchCaseSensitive = false;
let tableSearchRegex = false;
let tableSearchBangAndPipe = false;
let tableSearchError = '';
let tableBangAndPipeAST = null;
let tableSearchDebounceTimer = null;

// Action modal state
let actionEventSource = null;
let pendingAction = null;

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

// Scroll to a service row in the table and highlight it briefly
function scrollToService(serviceName, host) {
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

// Find a service's host_ip by name and host
function getServiceHostIP(serviceName, host) {
    const service = allServices.find(s => s.name === serviceName && s.host === host);
    return service ? service.host_ip : null;
}

function renderPorts(ports, hostIP, currentService) {
    if (!ports || ports.length === 0) {
        return '';
    }
    // Use hostIP if available, otherwise fall back to current hostname
    const targetHost = hostIP || window.location.hostname;
    const currentHost = currentService ? currentService.host : '';
    
    return ports
        .filter(port => !port.hidden) // Filter out hidden ports
        .map(port => {
            // Determine display text and styling based on port remapping
            // - source_service: port is remapped TO this service FROM source_service
            // - target_service: port is remapped FROM this service TO target_service
            let displayText;
            let titleText;
            let badgeClass = 'port-link badge bg-info text-dark me-1';
            
            if (port.label) {
                // Custom label takes precedence - use current service's host IP
                const url = `http://${targetHost}:${port.host_port}`;
                displayText = escapeHtml(port.label);
                titleText = `${escapeHtml(port.label)} - Port ${port.host_port} (${port.protocol})`;
                return `<a href="${url}" target="_blank" rel="noopener noreferrer" class="${badgeClass}" onclick="event.stopPropagation();" title="${titleText}">${displayText}</a>`;
            } else if (port.target_service) {
                // This port is remapped to another service - clicking scrolls to that service
                displayText = `<i class="bi bi-arrow-right me-1"></i>${escapeHtml(port.target_service)}:${port.host_port}`;
                titleText = `Click to go to ${escapeHtml(port.target_service)} (port ${port.host_port}, ${port.protocol})`;
                badgeClass = 'port-link-scroll badge bg-secondary text-light me-1'; // De-emphasized style
                return `<span class="${badgeClass}" onclick="event.stopPropagation(); scrollToService('${escapeHtml(port.target_service)}', '${escapeHtml(currentHost)}');" title="${titleText}" style="cursor: pointer;">${displayText}</span>`;
            } else if (port.source_service) {
                // This port comes from another service - link to the source service's IP:port
                const sourceIP = getServiceHostIP(port.source_service, currentHost) || targetHost;
                const url = `http://${sourceIP}:${port.host_port}`;
                displayText = `${escapeHtml(port.source_service)}:${port.host_port}`;
                titleText = `Open port ${port.host_port} on ${escapeHtml(port.source_service)} (${port.protocol})`;
                return `<a href="${url}" target="_blank" rel="noopener noreferrer" class="${badgeClass}" onclick="event.stopPropagation();" title="${titleText}">${displayText}</a>`;
            } else {
                const url = `http://${targetHost}:${port.host_port}`;
                displayText = `:${port.host_port}`;
                titleText = `Open port ${port.host_port} (${port.protocol})`;
                return `<a href="${url}" target="_blank" rel="noopener noreferrer" class="${badgeClass}" onclick="event.stopPropagation();" title="${titleText}">${displayText}</a>`;
            }
        }).join('');
}

function renderTraefikURLs(traefikURLs) {
    if (!traefikURLs || traefikURLs.length === 0) {
        return '';
    }
    return traefikURLs.map(url => {
        // Extract hostname from URL for display
        let hostname;
        try {
            hostname = new URL(url).hostname;
        } catch (e) {
            hostname = url;
        }
        return `<a href="${escapeHtml(url)}" target="_blank" rel="noopener noreferrer" class="traefik-link badge bg-success text-white me-1" onclick="event.stopPropagation();" title="Traefik: ${escapeHtml(hostname)}">${escapeHtml(hostname)}</a>`;
    }).join('');
}

// getSourceIcons returns HTML for source icons based on service's primary source and traefik integration
function getSourceIcons(service) {
    let icons = '';
    
    // Primary source icon
    if (service.source === 'systemd') {
        icons += '<i class="bi bi-gear-fill text-info" title="systemd"></i>';
    } else if (service.source === 'traefik') {
        icons += '<i class="bi bi-signpost-split text-warning" title="Traefik"></i>';
    } else {
        // Docker is default
        icons += '<i class="bi bi-box text-primary" title="Docker"></i>';
    }
    
    // If service has traefik integration (not pure traefik source), show traefik icon too
    if (service.source !== 'traefik' && service.traefik_urls && service.traefik_urls.length > 0) {
        icons += '<i class="bi bi-signpost-split text-warning ms-1" title="Exposed via Traefik"></i>';
    }
    
    return icons;
}

function renderServices(services, updateStats = true) {
    // Close any open logs first
    closeLogs();
    
    const tbody = document.getElementById('servicesTable');
    
    if (!services || services.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" class="text-center">No services found</td></tr>';
        return;
    }

    let running = 0;
    let stopped = 0;
    let dockerCount = 0;
    let systemdCount = 0;
    let traefikCount = 0;

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
        } else if (service.source === 'traefik') {
            traefikCount++;
        }
        // Also count services with Traefik integration (not pure traefik source)
        if (service.source !== 'traefik' && service.traefik_urls && service.traefik_urls.length > 0) {
            traefikCount++;
        }
    });

    const rows = services.map(service => {
        const statusClass = getStatusClass(service.state, service.status);
        const sourceIcons = getSourceIcons(service);
        const hostBadge = service.host ? `<span class="badge bg-secondary">${escapeHtml(service.host)}</span>` : '';
        const portsHtml = renderPorts(service.ports, service.host_ip, service);
        const traefikHtml = renderTraefikURLs(service.traefik_urls);
        const descriptionHtml = service.description ? `<div class="service-description text-muted small">${escapeHtml(service.description)}</div>` : '';
        const controlButtons = renderControlButtons(service);

        // Track if service has traefik integration for filtering
        const hasTraefikIntegration = service.traefik_urls && service.traefik_urls.length > 0;

        return `
            <tr class="service-row" data-container="${escapeHtml(service.container_name)}" data-service="${escapeHtml(service.name)}" data-source="${escapeHtml(service.source || 'docker')}" data-host="${escapeHtml(service.host || '')}" data-project="${escapeHtml(service.project || '')}" data-has-traefik="${hasTraefikIntegration}">
                <td>${sourceIcons} ${escapeHtml(service.name)} ${portsHtml} ${traefikHtml}${descriptionHtml}</td>
                <td>${escapeHtml(service.project)}</td>
                <td>${hostBadge}</td>
                <td><code class="small">${escapeHtml(service.container_name)}</code></td>
                <td><span class="badge badge-${statusClass}">${escapeHtml(service.status)}</span></td>
                <td class="image-cell" title="${escapeHtml(service.image)}">${escapeHtml(service.image)}</td>
                <td class="controls-cell">${controlButtons}</td>
            </tr>
        `;
    }).join('');

    tbody.innerHTML = rows;

    // Add click handlers for row (logs toggle)
    tbody.querySelectorAll('.service-row').forEach(row => {
        row.addEventListener('click', (e) => {
            // Don't toggle logs if clicking on control buttons
            if (e.target.closest('.service-controls')) {
                return;
            }
            toggleLogs(row);
        });
    });

    // Update stats (always show totals from all services)
    if (updateStats) {
        document.getElementById('totalCount').textContent = services.length;
        document.getElementById('runningCount').textContent = running;
        document.getElementById('stoppedCount').textContent = stopped;
        document.getElementById('dockerCount').innerHTML = '<i class="bi bi-box text-primary"></i> ' + dockerCount;
        document.getElementById('systemdCount').innerHTML = '<i class="bi bi-gear-fill text-info"></i> ' + systemdCount;
        document.getElementById('traefikCount').innerHTML = '<i class="bi bi-signpost-split text-warning"></i> ' + traefikCount;
    }
}

function renderControlButtons(service) {
    const isRunning = service.state.toLowerCase() === 'running';
    const containerName = escapeHtml(service.container_name);
    const serviceName = escapeHtml(service.name);
    const source = escapeHtml(service.source || 'docker');
    const host = escapeHtml(service.host || '');
    const project = escapeHtml(service.project || '');
    
    let buttons = '<div class="service-controls">';
    
    if (!isRunning) {
        buttons += `<button class="service-control-btn btn-start" onclick="confirmServiceAction(event, 'start', '${containerName}', '${serviceName}', '${source}', '${host}', '${project}')" title="Start service"><i class="bi bi-play-fill"></i></button>`;
    }
    
    if (isRunning) {
        buttons += `<button class="service-control-btn btn-stop" onclick="confirmServiceAction(event, 'stop', '${containerName}', '${serviceName}', '${source}', '${host}', '${project}')" title="Stop service"><i class="bi bi-stop-fill"></i></button>`;
    }
    
    buttons += `<button class="service-control-btn btn-restart" onclick="confirmServiceAction(event, 'restart', '${containerName}', '${serviceName}', '${source}', '${host}', '${project}')" title="Restart service"><i class="bi bi-arrow-clockwise"></i></button>`;
    
    buttons += '</div>';
    return buttons;
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
    const totalBeforeSearch = services.length;
    
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
        services = services.filter(service => {
            // For traefik filter, include services that either:
            // 1. Have traefik as primary source
            // 2. Have traefik integration (non-empty traefik_urls)
            if (activeSourceFilter === 'traefik') {
                return service.source === 'traefik' || 
                       (service.traefik_urls && service.traefik_urls.length > 0);
            }
            // For docker/systemd, just match by primary source
            return service.source === activeSourceFilter;
        });
    }
    
    // Apply table search filter
    const servicesBeforeSearch = services.length;
    if (tableSearchTerm) {
        services = services.filter(service => serviceMatchesTableSearch(service));
    }
    
    // Update match count UI
    updateTableMatchCountUI(services.length, servicesBeforeSearch);
    
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
        if (response.status === 401) {
            handleUnauthorized();
            return;
        }
        if (!response.ok) {
            throw new Error('Failed to fetch services');
        }
        const rawServices = await response.json();
        // Filter out hidden services
        allServices = rawServices.filter(service => !service.hidden);
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
    } else if (source === 'traefik') {
        url = '/api/logs/traefik?service=' + encodeURIComponent(serviceName) + '&host=' + encodeURIComponent(host);
    } else {
        url = '/api/logs?container=' + encodeURIComponent(containerName) + '&service=' + encodeURIComponent(serviceName);
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

// ============================================================================
// Table Search Functions
// ============================================================================

function onTableSearchInput(searchTerm) {
    tableSearchTerm = searchTerm;
    
    // Show/hide clear button
    const clearBtn = document.getElementById('tableClearBtn');
    if (clearBtn) {
        clearBtn.style.display = searchTerm ? 'flex' : 'none';
    }
    
    if (tableSearchBangAndPipe && searchTerm) {
        // Debounce the API call for bang-and-pipe mode
        if (tableSearchDebounceTimer) {
            clearTimeout(tableSearchDebounceTimer);
        }
        tableSearchDebounceTimer = setTimeout(() => {
            compileTableBangAndPipe(searchTerm);
        }, 150);
    } else {
        tableBangAndPipeAST = null;
        hideTableError();
        applyFilter();
    }
}

function onTableSearchKeydown(event) {
    if (event.key === 'Escape') {
        clearTableSearch();
    }
}

function clearTableSearch() {
    tableSearchTerm = '';
    tableBangAndPipeAST = null;
    tableSearchError = '';
    
    const input = document.getElementById('tableSearchInput');
    if (input) input.value = '';
    
    const clearBtn = document.getElementById('tableClearBtn');
    if (clearBtn) clearBtn.style.display = 'none';
    
    hideTableError();
    applyFilter();
}

async function compileTableBangAndPipe(expr) {
    try {
        const response = await fetch('/api/bangAndPipeToRegex?expr=' + encodeURIComponent(expr));
        const result = await response.json();
        
        if (result.valid) {
            tableBangAndPipeAST = result.ast;
            tableSearchError = '';
            hideTableError();
        } else {
            tableBangAndPipeAST = null;
            tableSearchError = result.error.message;
            showTableError(result.error);
        }
        
        applyFilter();
    } catch (e) {
        console.error('Error compiling bang-and-pipe expression:', e);
        tableBangAndPipeAST = null;
        tableSearchError = 'Failed to compile expression';
        applyFilter();
    }
}

function showTableError(error) {
    const popup = document.getElementById('tableErrorPopup');
    const input = document.getElementById('tableSearchInput');
    if (!popup || !input) return;
    
    // Build error message with position indicator
    const expr = tableSearchTerm;
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

function hideTableError() {
    const popup = document.getElementById('tableErrorPopup');
    const input = document.getElementById('tableSearchInput');
    if (popup) popup.classList.add('hidden');
    if (input) input.classList.remove('has-error');
}

function toggleTableCaseSensitivity() {
    tableSearchCaseSensitive = !tableSearchCaseSensitive;
    updateTableCaseToggleUI();
    applyFilter();
}

function toggleTableRegex() {
    tableSearchRegex = !tableSearchRegex;
    // Disable bang-and-pipe if enabling regex (they're mutually exclusive)
    if (tableSearchRegex && tableSearchBangAndPipe) {
        tableSearchBangAndPipe = false;
        updateTableBangPipeToggleUI();
    }
    updateTableRegexToggleUI();
    hideTableError();
    applyFilter();
}

function toggleTableBangAndPipe() {
    tableSearchBangAndPipe = !tableSearchBangAndPipe;
    // Disable regex if enabling bang-and-pipe (they're mutually exclusive)
    if (tableSearchBangAndPipe && tableSearchRegex) {
        tableSearchRegex = false;
        updateTableRegexToggleUI();
    }
    updateTableBangPipeToggleUI();
    
    if (tableSearchBangAndPipe && tableSearchTerm) {
        // Compile the current expression
        compileTableBangAndPipe(tableSearchTerm);
    } else {
        tableBangAndPipeAST = null;
        hideTableError();
        applyFilter();
    }
}

function updateTableCaseToggleUI() {
    const caseBtn = document.getElementById('tableCaseToggle');
    if (!caseBtn) return;
    
    if (tableSearchCaseSensitive) {
        caseBtn.classList.add('active');
        caseBtn.title = 'Case sensitive (click to toggle)';
    } else {
        caseBtn.classList.remove('active');
        caseBtn.title = 'Case insensitive (click to toggle)';
    }
}

function updateTableRegexToggleUI() {
    const regexBtn = document.getElementById('tableRegexToggle');
    const input = document.getElementById('tableSearchInput');
    if (!regexBtn) return;
    
    if (tableSearchRegex) {
        regexBtn.classList.add('active');
        regexBtn.title = 'Regular expression enabled (click to toggle)';
    } else {
        regexBtn.classList.remove('active');
        regexBtn.title = 'Use Regular Expression (click to toggle)';
    }
    
    // Update input styling for regex errors
    if (input) {
        if (tableSearchError) {
            input.classList.add('has-error');
            input.title = tableSearchError;
        } else {
            input.classList.remove('has-error');
            input.title = '';
        }
    }
}

function updateTableBangPipeToggleUI() {
    const btn = document.getElementById('tableBangPipeToggle');
    if (!btn) return;
    
    if (tableSearchBangAndPipe) {
        btn.classList.add('active');
        btn.title = 'Bang & Pipe mode enabled: Use ! (not), & (and), | (or), () grouping, "" literals';
    } else {
        btn.classList.remove('active');
        btn.title = 'Bang & Pipe mode: Use !&| operators';
    }
}

function updateTableMatchCountUI(matchCount, totalCount) {
    const countEl = document.getElementById('tableMatchCount');
    if (!countEl) return;
    
    if (!tableSearchTerm) {
        countEl.textContent = '';
        countEl.classList.remove('no-matches');
        return;
    }
    
    if (tableSearchError) {
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
 * Check if a service matches the table search term.
 * Searches across all relevant fields: name, project, host, container_name, status, image, source
 * @param {Object} service - The service object
 * @returns {boolean} - True if the service matches the search
 */
function serviceMatchesTableSearch(service) {
    if (!tableSearchTerm) return true;
    
    // Combine all searchable fields into a single string for matching
    // This makes the search work across any column and is extensible
    const searchableText = [
        service.name || '',
        service.project || '',
        service.host || '',
        service.container_name || '',
        service.status || '',
        service.state || '',
        service.image || '',
        service.source || '',
        // Include ports for searching by port number
        ...(service.ports || []).map(p => String(p.host_port)),
        // Include traefik URLs for searching by hostname
        ...(service.traefik_urls || []).map(url => {
            try {
                return new URL(url).hostname;
            } catch (e) {
                return url;
            }
        })
    ].join(' ');
    
    return tableTextMatches(searchableText, tableSearchTerm);
}

/**
 * Check if text matches the table search term using current search settings
 * @param {string} text - The text to search in
 * @param {string} searchTerm - The search term
 * @returns {boolean} - True if the text matches
 */
function tableTextMatches(text, searchTerm) {
    if (!searchTerm) return true;
    
    // Bang-and-pipe mode: use AST evaluation
    if (tableSearchBangAndPipe) {
        if (!tableBangAndPipeAST) return false;
        return evaluateTableAST(tableBangAndPipeAST, text);
    }
    
    // Regex mode with ! prefix for inverse
    if (tableSearchRegex && searchTerm.startsWith('!') && !searchTerm.startsWith('\\!')) {
        const pattern = searchTerm.slice(1);
        if (!pattern) return true; // !empty matches all
        try {
            const flags = tableSearchCaseSensitive ? '' : 'i';
            const regex = new RegExp(pattern, flags);
            return !regex.test(text);
        } catch (e) {
            tableSearchError = 'Invalid regex: ' + e.message;
            return false;
        }
    }
    
    // Regex mode with escaped \!
    let effectivePattern = searchTerm;
    if (tableSearchRegex && searchTerm.startsWith('\\!')) {
        effectivePattern = searchTerm.slice(2);
    }
    
    // Standard regex mode
    if (tableSearchRegex) {
        try {
            const flags = tableSearchCaseSensitive ? '' : 'i';
            const regex = new RegExp(effectivePattern, flags);
            return regex.test(text);
        } catch (e) {
            tableSearchError = 'Invalid regex: ' + e.message;
            return false;
        }
    }
    
    // Plain text mode
    if (tableSearchCaseSensitive) {
        return text.includes(searchTerm);
    }
    return text.toLowerCase().includes(searchTerm.toLowerCase());
}

/**
 * Evaluate a Bang & Pipe AST against text for table search
 * @param {Object} ast - The AST node
 * @param {string} text - The text to evaluate against
 * @returns {boolean} - True if the AST matches the text
 */
function evaluateTableAST(ast, text) {
    if (!ast) return false;
    
    switch (ast.type) {
        case 'pattern':
            // Use the regex field from the AST
            try {
                const flags = tableSearchCaseSensitive ? '' : 'i';
                const regex = new RegExp(ast.regex, flags);
                return regex.test(text);
            } catch (e) {
                return false;
            }
        case 'or':
            return ast.children.some(child => evaluateTableAST(child, text));
        case 'and':
            return ast.children.every(child => evaluateTableAST(child, text));
        case 'not':
            return !evaluateTableAST(ast.child, text);
        default:
            return false;
    }
}

// ============================================================================
// Service Action Functions
// ============================================================================

/**
 * Show confirmation modal for a service action
 * @param {Event} event - The click event
 * @param {string} action - The action (start, stop, restart)
 * @param {string} containerName - The container/unit name
 * @param {string} serviceName - The display service name
 * @param {string} source - The service source (docker, systemd)
 * @param {string} host - The host name
 * @param {string} project - The project name (for docker-compose)
 */
function confirmServiceAction(event, action, containerName, serviceName, source, host, project) {
    event.stopPropagation();
    
    // Store pending action
    pendingAction = {
        action,
        containerName,
        serviceName,
        source,
        host,
        project
    };
    
    // Update modal content
    const actionText = action.charAt(0).toUpperCase() + action.slice(1);
    const sourceIcon = source === 'systemd' ? '<i class="bi bi-gear-fill text-info"></i>' : '<i class="bi bi-box text-primary"></i>';
    
    document.getElementById('actionModalLabel').innerHTML = `<i class="bi bi-exclamation-triangle-fill text-warning"></i> Confirm ${actionText}`;
    document.getElementById('actionModalMessage').innerHTML = `
        Are you sure you want to <strong>${action}</strong> the service?<br>
        <br>
        ${sourceIcon} <strong>${escapeHtml(serviceName)}</strong>
        ${host ? `<span class="badge bg-secondary ms-2">${escapeHtml(host)}</span>` : ''}
        ${source === 'docker' && action === 'restart' ? '<br><small class="text-muted mt-2 d-block">Docker restart uses compose down/up</small>' : ''}
    `;
    
    // Reset modal state
    document.getElementById('actionModalStatus').style.display = 'none';
    document.getElementById('actionStatusLog').innerHTML = '';
    document.getElementById('actionCountdown').style.display = 'none';
    document.getElementById('actionModalFooter').style.display = 'flex';
    document.getElementById('actionSpinner').style.display = 'none';
    
    // Set up confirm button
    const confirmBtn = document.getElementById('actionModalConfirm');
    confirmBtn.className = 'btn btn-primary';
    confirmBtn.textContent = 'Yes, proceed';
    confirmBtn.disabled = false;
    confirmBtn.onclick = executeServiceAction;
    
    // Show modal
    const modal = new bootstrap.Modal(document.getElementById('actionModal'));
    modal.show();
}

/**
 * Execute the pending service action with SSE status updates
 */
function executeServiceAction() {
    if (!pendingAction) return;
    
    const { action, containerName, serviceName, source, host, project } = pendingAction;
    
    // Update UI to show progress
    document.getElementById('actionModalStatus').style.display = 'block';
    document.getElementById('actionModalFooter').style.display = 'none';
    document.getElementById('actionSpinner').style.display = 'inline-block';
    
    const statusLog = document.getElementById('actionStatusLog');
    statusLog.innerHTML = '';
    addActionLogLine('Initiating ' + action + '...', 'status');
    
    // Make POST request with SSE response
    const requestBody = {
        container_name: containerName,
        service_name: serviceName,
        source: source,
        host: host,
        project: project
    };
    
    // Use fetch with ReadableStream to handle SSE from POST
    fetch(`/api/services/${action}`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(requestBody)
    }).then(response => {
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        
        function processSSE(text) {
            buffer += text;
            const lines = buffer.split('\n');
            buffer = lines.pop() || ''; // Keep incomplete line in buffer
            
            let currentEvent = null;
            for (const line of lines) {
                if (line.startsWith('event: ')) {
                    currentEvent = line.substring(7).trim();
                } else if (line.startsWith('data: ')) {
                    const data = line.substring(6);
                    handleActionEvent(currentEvent || 'message', data);
                }
            }
        }
        
        function read() {
            reader.read().then(({ done, value }) => {
                if (done) {
                    // Process any remaining buffer
                    if (buffer.trim()) {
                        processSSE('\n');
                    }
                    return;
                }
                processSSE(decoder.decode(value, { stream: true }));
                read();
            }).catch(err => {
                addActionLogLine('Connection error: ' + err.message, 'error');
                document.getElementById('actionSpinner').style.display = 'none';
            });
        }
        
        read();
    }).catch(err => {
        addActionLogLine('Request failed: ' + err.message, 'error');
        document.getElementById('actionSpinner').style.display = 'none';
        showActionRetry();
    });
}

/**
 * Handle SSE events from service action
 * @param {string} eventType - The event type
 * @param {string} data - The event data
 */
function handleActionEvent(eventType, data) {
    switch (eventType) {
        case 'status':
            addActionLogLine(data, 'status');
            break;
        case 'error':
            addActionLogLine('Error: ' + data, 'error');
            break;
        case 'complete':
            document.getElementById('actionSpinner').style.display = 'none';
            if (data === 'success') {
                addActionLogLine('✓ Action completed successfully', 'success');
                startCountdown();
            } else {
                addActionLogLine('✗ Action failed', 'error');
                showActionRetry();
            }
            break;
    }
    
    // Auto-scroll log
    const statusLog = document.getElementById('actionStatusLog');
    statusLog.scrollTop = statusLog.scrollHeight;
}

/**
 * Add a line to the action status log
 * @param {string} message - The message to add
 * @param {string} className - CSS class for styling (status, error, success)
 */
function addActionLogLine(message, className) {
    const statusLog = document.getElementById('actionStatusLog');
    const line = document.createElement('div');
    line.className = 'action-log-line ' + (className || '');
    line.textContent = message;
    statusLog.appendChild(line);
}

/**
 * Show retry/close buttons after action failure
 */
function showActionRetry() {
    const footer = document.getElementById('actionModalFooter');
    footer.style.display = 'flex';
    footer.innerHTML = `
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Close</button>
        <button type="button" class="btn btn-primary" onclick="executeServiceAction()">Retry</button>
    `;
}

/**
 * Start countdown and then refresh services
 */
function startCountdown() {
    const countdownEl = document.getElementById('actionCountdown');
    const valueEl = document.getElementById('countdownValue');
    countdownEl.style.display = 'block';
    
    let count = 5;
    valueEl.textContent = count;
    
    const interval = setInterval(() => {
        count--;
        valueEl.textContent = count;
        
        if (count <= 0) {
            clearInterval(interval);
            closeActionModalAndRefresh();
        }
    }, 1000);
}

/**
 * Close the action modal and refresh services data
 */
function closeActionModalAndRefresh() {
    // Close modal
    const modalEl = document.getElementById('actionModal');
    const modal = bootstrap.Modal.getInstance(modalEl);
    if (modal) {
        modal.hide();
    }
    
    // Clear pending action
    pendingAction = null;
    
    // Refresh services data without page reload
    loadServices();
}

// Auth state
let authStatus = null;

/**
 * Check authentication status
 */
async function checkAuthStatus() {
    try {
        const response = await fetch('/auth/status');
        if (!response.ok) {
            throw new Error('Failed to check auth status');
        }
        authStatus = await response.json();
        updateAuthUI();
        return authStatus;
    } catch (error) {
        console.error('Auth status check failed:', error);
        // Assume not authenticated if we can't check
        authStatus = { authenticated: false, oidc_enabled: false };
        updateAuthUI();
        return authStatus;
    }
}

/**
 * Update the UI based on authentication status
 */
function updateAuthUI() {
    const authControls = document.getElementById('authControls');
    const userInfo = document.getElementById('userInfo');
    
    if (!authStatus) {
        authControls.style.display = 'none';
        return;
    }
    
    if (authStatus.oidc_enabled && authStatus.authenticated && authStatus.user) {
        authControls.style.display = 'flex';
        const displayName = authStatus.user.name || authStatus.user.email || 'User';
        userInfo.innerHTML = `<i class="bi bi-person-circle"></i> ${escapeHtml(displayName)}`;
    } else if (authStatus.oidc_enabled && !authStatus.authenticated) {
        // Will be redirected by the server, but show nothing in the meantime
        authControls.style.display = 'none';
    } else {
        // OIDC not enabled, no auth controls needed
        authControls.style.display = 'none';
    }
}

/**
 * Handle logout
 */
function logout() {
    window.location.href = '/logout';
}

/**
 * Handle 401 Unauthorized responses
 */
function handleUnauthorized() {
    if (authStatus && authStatus.oidc_enabled) {
        // Redirect to login
        window.location.href = '/login?redirect=' + encodeURIComponent(window.location.pathname);
    }
}

/**
 * Wrap fetch to handle auth errors
 */
async function authFetch(url, options = {}) {
    const response = await fetch(url, options);
    if (response.status === 401) {
        handleUnauthorized();
        throw new Error('Authentication required');
    }
    return response;
}

// Load services on page load, but check auth first
document.addEventListener('DOMContentLoaded', async function() {
    await checkAuthStatus();
    loadServices();
});
