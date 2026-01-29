/**
 * Service rendering functions.
 */

import { escapeHtml, getStatusClass, formatLogSize } from './utils.js';
import { getServiceHostIP, scrollToService } from './services.js';
import { authState } from './state.js';
import { getVisibleColumns, renderTableHeader as renderColumnsHeader } from './columns.js';

/** Toast timeout handle */
let toastTimeout = null;

/**
 * Show a status toast message (for mobile).
 * @param {string} message - Message to display
 * @param {string} statusClass - Status class (running, stopped, unhealthy)
 */
export function showStatusToast(message, statusClass) {
    if (typeof document === 'undefined') return;
    
    const toast = document.getElementById('statusToast');
    if (!toast) return;
    
    // Clear any existing timeout
    if (toastTimeout) {
        clearTimeout(toastTimeout);
    }
    
    // Set content and show
    toast.textContent = message;
    toast.className = 'status-toast show ' + statusClass;
    
    // Auto-hide after 2 seconds
    toastTimeout = setTimeout(() => {
        toast.classList.remove('show');
    }, 2000);
}

/**
 * Render port badges for a service.
 * @param {Array} ports - Array of port objects
 * @param {string} hostIP - The host IP address
 * @param {Object} currentService - The current service object
 * @returns {string} HTML string of port badges
 */
export function renderPorts(ports, hostIP, currentService) {
    if (!ports || ports.length === 0) {
        return '';
    }
    // Use hostIP if available, otherwise fall back to current hostname
    const targetHost = hostIP || (typeof window !== 'undefined' ? window.location.hostname : 'localhost');
    const currentHost = currentService ? currentService.host : '';
    
    return ports
        .filter(port => !port.hidden)
        .map(port => {
            let displayText;
            let titleText;
            let badgeClass = 'port-link badge bg-info text-dark me-1';
            // Use custom protocol if specified, otherwise default to http
            const urlProtocol = port.url_protocol || 'http';
            
            if (port.label) {
                const url = `${urlProtocol}://${targetHost}:${port.host_port}`;
                displayText = escapeHtml(port.label);
                titleText = `${escapeHtml(port.label)} - Port ${port.host_port} (${port.protocol})`;
                return `<a href="${url}" target="_blank" rel="noopener noreferrer" class="${badgeClass}" onclick="event.stopPropagation();" title="${titleText}">${displayText}</a>`;
            } else if (port.target_service) {
                displayText = `<i class="bi bi-arrow-right me-1"></i>${escapeHtml(port.target_service)}:${port.host_port}`;
                titleText = `Click to go to ${escapeHtml(port.target_service)} (port ${port.host_port}, ${port.protocol})`;
                badgeClass = 'port-link-scroll badge bg-secondary text-light me-1';
                return `<span class="${badgeClass}" onclick="event.stopPropagation(); window.__dashboard.scrollToService('${escapeHtml(port.target_service)}', '${escapeHtml(currentHost)}');" title="${titleText}" style="cursor: pointer;">${displayText}</span>`;
            } else if (port.source_service) {
                const sourceIP = getServiceHostIP(port.source_service, currentHost) || targetHost;
                const url = `${urlProtocol}://${sourceIP}:${port.host_port}`;
                displayText = `${escapeHtml(port.source_service)}:${port.host_port}`;
                titleText = `Open port ${port.host_port} on ${escapeHtml(port.source_service)} (${port.protocol})`;
                return `<a href="${url}" target="_blank" rel="noopener noreferrer" class="${badgeClass}" onclick="event.stopPropagation();" title="${titleText}">${displayText}</a>`;
            } else {
                const url = `${urlProtocol}://${targetHost}:${port.host_port}`;
                displayText = `:${port.host_port}`;
                titleText = `Open port ${port.host_port} (${port.protocol})`;
                return `<a href="${url}" target="_blank" rel="noopener noreferrer" class="${badgeClass}" onclick="event.stopPropagation();" title="${titleText}">${displayText}</a>`;
            }
        }).join('');
}

/**
 * Render Traefik URL badges for a service.
 * @param {Array} traefikURLs - Array of Traefik URLs
 * @returns {string} HTML string of Traefik badges
 */
export function renderTraefikURLs(traefikURLs) {
    if (!traefikURLs || traefikURLs.length === 0) {
        return '';
    }
    return traefikURLs.map(url => {
        let hostname;
        try {
            hostname = new URL(url).hostname;
        } catch (e) {
            hostname = url;
        }
        return `<a href="${escapeHtml(url)}" target="_blank" rel="noopener noreferrer" class="traefik-link badge bg-success text-white me-1" onclick="event.stopPropagation();" title="Traefik: ${escapeHtml(hostname)}">${escapeHtml(hostname)}</a>`;
    }).join('');
}

/**
 * Get source icons HTML for a service.
 * @param {Object} service - The service object
 * @returns {string} HTML string of source icons
 */
export function getSourceIcons(service) {
    let icons = '';
    
    if (service.source === 'systemd') {
        icons += '<i class="bi bi-gear-fill text-info" title="systemd"></i>';
    } else if (service.source === 'traefik') {
        icons += '<i class="bi bi-signpost-split text-warning" title="Traefik"></i>';
    } else if (service.source === 'homeassistant') {
        icons += '<i class="bi bi-house-heart-fill text-primary" title="Home Assistant"></i>';
    } else if (service.source === 'homeassistant-addon') {
        icons += '<i class="bi bi-puzzle-fill text-info" title="Home Assistant Addon"></i>';
    } else {
        icons += '<i class="bi bi-box text-primary" title="Docker"></i>';
    }
    
    if (service.source !== 'traefik' && service.traefik_urls && service.traefik_urls.length > 0) {
        icons += '<i class="bi bi-signpost-split text-warning ms-1" title="Exposed via Traefik"></i>';
    }
    
    return icons;
}

/**
 * Check if the current user is an admin.
 * @returns {boolean} True if user is admin
 */
export function isAdmin() {
    return authState.status?.user?.is_admin === true;
}

/**
 * Render control buttons for a service.
 * @param {Object} service - The service object
 * @returns {string} HTML string of control buttons
 */
export function renderControlButtons(service) {
    // Read-only services show no control buttons
    if (service.readonly) {
        return '<div class="service-controls"><span class="text-muted small" title="This service is read-only"><i class="bi bi-lock"></i></span></div>';
    }
    
    const isRunning = service.state.toLowerCase() === 'running';
    const containerName = escapeHtml(service.container_name);
    const serviceName = escapeHtml(service.name);
    const source = escapeHtml(service.source || 'docker');
    const host = escapeHtml(service.host || '');
    const project = escapeHtml(service.project || '');
    
    let buttons = '<div class="service-controls">';
    
    if (!isRunning) {
        buttons += `<button class="service-control-btn btn-start" onclick="window.__dashboard.confirmServiceAction(event, 'start', '${containerName}', '${serviceName}', '${source}', '${host}', '${project}')" title="Start service"><i class="bi bi-play-fill"></i></button>`;
    }
    
    if (isRunning) {
        buttons += `<button class="service-control-btn btn-stop" onclick="window.__dashboard.confirmServiceAction(event, 'stop', '${containerName}', '${serviceName}', '${source}', '${host}', '${project}')" title="Stop service"><i class="bi bi-stop-fill"></i></button>`;
    }
    
    buttons += `<button class="service-control-btn btn-restart" onclick="window.__dashboard.confirmServiceAction(event, 'restart', '${containerName}', '${serviceName}', '${source}', '${host}', '${project}')" title="Restart service"><i class="bi bi-arrow-clockwise"></i></button>`;
    
    buttons += '</div>';
    return buttons;
}

/**
 * Render the log size column for a service.
 * @param {Object} service - The service object
 * @returns {string} HTML string for log size column
 */
export function renderLogSize(service) {
    const containerName = escapeHtml(service.container_name);
    const serviceName = escapeHtml(service.name);
    const host = escapeHtml(service.host || '');
    const source = service.source || 'docker';
    
    // Only Docker services have log sizes
    if (source !== 'docker' || !service.log_size || service.log_size <= 0) {
        return '<span class="text-muted">-</span>';
    }
    
    const logSizeFormatted = formatLogSize(service.log_size);
    const userIsAdmin = isAdmin();
    
    if (userIsAdmin) {
        // Admin users get clickable flush button
        return `<button class="btn btn-sm btn-logs" onclick="window.__dashboard.confirmLogFlush(event, '${containerName}', '${serviceName}', '${host}')" title="Flush logs (${logSizeFormatted})"><i class="bi bi-file-earmark-x me-1"></i>${logSizeFormatted}</button>`;
    } else {
        // Non-admin users see log size but can't flush
        return `<span class="btn btn-sm btn-logs btn-logs-readonly" title="Log size: ${logSizeFormatted}"><i class="bi bi-file-earmark-text me-1"></i>${logSizeFormatted}</span>`;
    }
}

/**
 * Render the services table body.
 * @param {Array} services - Array of service objects
 * @param {boolean} updateStats - Whether to update stats counters
 * @param {Object} callbacks - Callback functions
 * @param {Function} callbacks.onRowClick - Called when a row is clicked
 */
export function renderServices(services, updateStats = true, callbacks = {}) {
    if (typeof document === 'undefined') return;
    
    const { closeLogs } = callbacks;
    
    // Close any open logs first
    if (closeLogs) closeLogs();
    
    const tbody = document.getElementById('servicesTable');
    
    // Get visible columns
    const visibleColumns = getVisibleColumns();
    
    // Render table header with current column configuration
    renderColumnsHeader();
    
    if (!services || services.length === 0) {
        tbody.innerHTML = `<tr><td colspan="${visibleColumns.length}" class="text-center">No services found</td></tr>`;
        return;
    }

    let running = 0;
    let stopped = 0;
    let dockerCount = 0;
    let systemdCount = 0;
    let traefikCount = 0;
    let homeassistantCount = 0;

    services.forEach(service => {
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
        } else if (service.source === 'homeassistant' || service.source === 'homeassistant-addon') {
            homeassistantCount++;
        }
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
        const logSizeHtml = renderLogSize(service);
        const hasTraefikIntegration = service.traefik_urls && service.traefik_urls.length > 0;

        // Build cell content map
        const cellContent = {
            name: `${sourceIcons} ${escapeHtml(service.name)} ${portsHtml} ${traefikHtml}${descriptionHtml}`,
            project: escapeHtml(service.project),
            host: hostBadge,
            container: `<code class="small">${escapeHtml(service.container_name)}</code>`,
            status: `<span class="badge badge-${statusClass} status-badge" title="${escapeHtml(service.status)}" onclick="event.stopPropagation(); window.__dashboard.showStatusToast('${escapeHtml(service.status).replace(/'/g, "\\'")}', '${statusClass}')"><span class="status-text">${escapeHtml(service.status)}</span></span>`,
            image: escapeHtml(service.image),
            log_size: logSizeHtml,
            actions: controlButtons
        };
        
        // Build cells based on visible columns
        const cells = visibleColumns.map(col => {
            let cellClass = '';
            let cellAttrs = '';
            
            if (col.id === 'name') {
                cellClass = 'class="service-cell"';
            } else if (col.id === 'status') {
                cellClass = 'class="status-cell"';
            } else if (col.id === 'image') {
                cellClass = 'class="image-cell"';
                cellAttrs = `title="${escapeHtml(service.image)}"`;
            } else if (col.id === 'log_size') {
                cellClass = 'class="logs-cell"';
            } else if (col.id === 'actions') {
                cellClass = 'class="controls-cell"';
            }
            
            return `<td ${cellClass} ${cellAttrs}>${cellContent[col.id] || ''}</td>`;
        }).join('');

        return `
            <tr class="service-row" data-container="${escapeHtml(service.container_name)}" data-service="${escapeHtml(service.name)}" data-source="${escapeHtml(service.source || 'docker')}" data-host="${escapeHtml(service.host || '')}" data-project="${escapeHtml(service.project || '')}" data-has-traefik="${hasTraefikIntegration}">
                ${cells}
            </tr>
        `;
    }).join('');

    tbody.innerHTML = rows;

    // Add click handlers for row (logs toggle)
    if (callbacks.onRowClick) {
        tbody.querySelectorAll('.service-row').forEach(row => {
            row.addEventListener('click', (e) => {
                if (e.target.closest('.service-controls')) {
                    return;
                }
                callbacks.onRowClick(row);
            });
        });
    }

    // Update stats
    if (updateStats) {
        document.getElementById('totalCount').textContent = services.length;
        document.getElementById('runningCount').textContent = running;
        document.getElementById('stoppedCount').textContent = stopped;
        document.getElementById('dockerCount').innerHTML = '<i class="bi bi-box text-primary"></i> ' + dockerCount;
        document.getElementById('systemdCount').innerHTML = '<i class="bi bi-gear-fill text-info"></i> ' + systemdCount;
        document.getElementById('traefikCount').innerHTML = '<i class="bi bi-signpost-split text-warning"></i> ' + traefikCount;
        document.getElementById('homeassistantCount').innerHTML = '<i class="bi bi-house-heart-fill text-primary"></i> ' + homeassistantCount;
    }
}

/**
 * Update a single service row reactively without re-rendering the entire table.
 * @param {Object} update - Service update payload from WebSocket
 * @param {string} update.host - Host name
 * @param {string} update.service_name - Service name
 * @param {string} update.source - Service source (docker, systemd, etc.)
 * @param {string} update.current_state - New state
 * @param {string} update.status - New status text
 * @param {Object} callbacks - Callback functions for row click handlers
 */
export function updateServiceRow(update, callbacks = {}) {
    if (typeof document === 'undefined') return;
    
    const tbody = document.getElementById('servicesTable');
    if (!tbody) return;
    
    // Find the row for this service
    const rows = tbody.querySelectorAll('.service-row');
    let targetRow = null;
    
    for (const row of rows) {
        const rowService = row.dataset.service;
        const rowHost = row.dataset.host;
        const rowSource = row.dataset.source;
        
        // Match by service name, host, and source
        if (rowService === update.service_name && 
            rowHost === update.host && 
            rowSource === update.source) {
            targetRow = row;
            break;
        }
    }
    
    if (!targetRow) {
        console.log('WebSocket: service row not found for update', update);
        return;
    }
    
    // Find the status cell dynamically based on visible columns
    const visibleColumns = getVisibleColumns();
    const statusColIndex = visibleColumns.findIndex(col => col.id === 'status');
    
    // Update the status badge in the row (if status column is visible)
    if (statusColIndex >= 0) {
        const statusCell = targetRow.querySelector(`td:nth-child(${statusColIndex + 1})`);
        if (statusCell) {
            const statusClass = getStatusClass(update.current_state, update.status);
            statusCell.innerHTML = `<span class="badge badge-${statusClass} status-badge" title="${escapeHtml(update.status)}" onclick="event.stopPropagation(); window.__dashboard.showStatusToast('${escapeHtml(update.status)}', '${statusClass}')"><span class="status-text">${escapeHtml(update.status)}</span></span>`;
        }
    }
    
    // Update the control buttons to reflect new state
    const controlsCell = targetRow.querySelector('.controls-cell');
    if (controlsCell) {
        const isRunning = update.current_state.toLowerCase() === 'running';
        const containerName = escapeHtml(targetRow.dataset.container);
        const serviceName = escapeHtml(targetRow.dataset.service);
        const source = escapeHtml(targetRow.dataset.source);
        const host = escapeHtml(targetRow.dataset.host);
        const project = escapeHtml(targetRow.dataset.project);
        
        let buttons = '<div class="service-controls">';
        
        if (!isRunning) {
            buttons += `<button class="service-control-btn btn-start" onclick="window.__dashboard.confirmServiceAction(event, 'start', '${containerName}', '${serviceName}', '${source}', '${host}', '${project}')" title="Start service"><i class="bi bi-play-fill"></i></button>`;
        }
        
        if (isRunning) {
            buttons += `<button class="service-control-btn btn-stop" onclick="window.__dashboard.confirmServiceAction(event, 'stop', '${containerName}', '${serviceName}', '${source}', '${host}', '${project}')" title="Stop service"><i class="bi bi-stop-fill"></i></button>`;
        }
        
        buttons += `<button class="service-control-btn btn-restart" onclick="window.__dashboard.confirmServiceAction(event, 'restart', '${containerName}', '${serviceName}', '${source}', '${host}', '${project}')" title="Restart service"><i class="bi bi-arrow-clockwise"></i></button>`;
        
        buttons += '</div>';
        controlsCell.innerHTML = buttons;
    }
    
    // Add visual feedback for the update
    targetRow.classList.add('service-updated');
    setTimeout(() => {
        targetRow.classList.remove('service-updated');
    }, 2000);
    
    // Update stats counters
    updateStatsFromDOM();
}

/**
 * Update stats counters by scanning the current table state.
 * This is more accurate than maintaining separate state.
 */
export function updateStatsFromDOM() {
    if (typeof document === 'undefined') return;
    
    const tbody = document.getElementById('servicesTable');
    if (!tbody) return;
    
    const rows = tbody.querySelectorAll('.service-row');
    let running = 0;
    let stopped = 0;
    let dockerCount = 0;
    let systemdCount = 0;
    let traefikCount = 0;
    let homeassistantCount = 0;
    
    rows.forEach(row => {
        // Check status badge
        const statusBadge = row.querySelector('td:nth-child(5) .badge');
        if (statusBadge) {
            if (statusBadge.classList.contains('badge-running')) {
                running++;
            } else {
                stopped++;
            }
        }
        
        // Count by source
        const source = row.dataset.source;
        if (source === 'docker') {
            dockerCount++;
        } else if (source === 'systemd') {
            systemdCount++;
        } else if (source === 'traefik') {
            traefikCount++;
        } else if (source === 'homeassistant' || source === 'homeassistant-addon') {
            homeassistantCount++;
        }
        
        // Count traefik integrations
        if (source !== 'traefik' && row.dataset.hasTraefik === 'true') {
            traefikCount++;
        }
    });
    
    // Update DOM
    const totalEl = document.getElementById('totalCount');
    const runningEl = document.getElementById('runningCount');
    const stoppedEl = document.getElementById('stoppedCount');
    const dockerEl = document.getElementById('dockerCount');
    const systemdEl = document.getElementById('systemdCount');
    const traefikEl = document.getElementById('traefikCount');
    const homeassistantEl = document.getElementById('homeassistantCount');
    
    if (totalEl) totalEl.textContent = rows.length;
    if (runningEl) runningEl.textContent = running;
    if (stoppedEl) stoppedEl.textContent = stopped;
    if (dockerEl) dockerEl.innerHTML = '<i class="bi bi-box text-primary"></i> ' + dockerCount;
    if (systemdEl) systemdEl.innerHTML = '<i class="bi bi-gear-fill text-info"></i> ' + systemdCount;
    if (traefikEl) traefikEl.innerHTML = '<i class="bi bi-signpost-split text-warning"></i> ' + traefikCount;
    if (homeassistantEl) homeassistantEl.innerHTML = '<i class="bi bi-house-heart-fill text-primary"></i> ' + homeassistantCount;
}

/**
 * Extract unique hosts from services.
 * @param {Array} services - Array of service objects
 * @returns {Array} Sorted array of unique host names
 */
export function getUniqueHosts(services) {
    const hosts = new Set();
    services.forEach(service => {
        if (service.host) {
            hosts.add(service.host);
        }
    });
    return Array.from(hosts).sort();
}

/**
 * Render host filter badges.
 * @param {Array} services - Array of service objects to extract hosts from
 */
export function renderHostFilters(services) {
    if (typeof document === 'undefined') return;
    
    const container = document.getElementById('hostFiltersContainer');
    if (!container) return;
    
    const hosts = getUniqueHosts(services);
    
    if (hosts.length === 0) {
        container.innerHTML = '<span class="text-muted small">No hosts</span>';
        return;
    }
    
    // Count services per host
    const hostCounts = {};
    services.forEach(service => {
        const host = service.host || '';
        hostCounts[host] = (hostCounts[host] || 0) + 1;
    });
    
    const badges = hosts.map(host => {
        const count = hostCounts[host] || 0;
        return `<span class="host-filter-badge" data-host="${escapeHtml(host)}" onclick="window.__dashboard.toggleHostFilter('${escapeHtml(host)}')" title="Click to filter by ${escapeHtml(host)}">
            <i class="bi bi-hdd-rack me-1"></i>${escapeHtml(host)} <span class="host-count">${count}</span>
        </span>`;
    }).join('');
    
    container.innerHTML = badges;
}
