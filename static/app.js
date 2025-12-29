let eventSource = null;
let activeLogsRow = null;
let allServices = [];
let activeFilter = null;
let sortColumn = null;
let sortDirection = 'asc';

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

    // Count all services for stats (use allServices for accurate counts)
    const statsSource = updateStats ? services : allServices;
    statsSource.forEach(service => {
        if (service.state.toLowerCase() === 'running') {
            running++;
        } else {
            stopped++;
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
    }
}

function toggleFilter(filter) {
    // If clicking the same filter, clear it
    if (activeFilter === filter) {
        activeFilter = null;
    } else {
        activeFilter = filter;
    }
    
    // Update card selection state
    document.querySelectorAll('.stat-card').forEach(card => {
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

function applyFilter() {
    let services = [...allServices];
    
    // Apply filter
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
        content.appendChild(line);
        
        // Auto-scroll to bottom
        content.scrollTop = content.scrollHeight;

        // Limit lines to prevent memory issues
        while (content.children.length > 1000) {
            content.removeChild(content.firstChild);
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

// Load services on page load
document.addEventListener('DOMContentLoaded', loadServices);
