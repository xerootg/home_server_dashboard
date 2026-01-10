/**
 * Service rendering functions.
 */

import { escapeHtml, getStatusClass } from './utils.js';
import { getServiceHostIP, scrollToService } from './services.js';

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
            
            if (port.label) {
                const url = `http://${targetHost}:${port.host_port}`;
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
    } else {
        icons += '<i class="bi bi-box text-primary" title="Docker"></i>';
    }
    
    if (service.source !== 'traefik' && service.traefik_urls && service.traefik_urls.length > 0) {
        icons += '<i class="bi bi-signpost-split text-warning ms-1" title="Exposed via Traefik"></i>';
    }
    
    return icons;
}

/**
 * Render control buttons for a service.
 * @param {Object} service - The service object
 * @returns {string} HTML string of control buttons
 */
export function renderControlButtons(service) {
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
    
    if (!services || services.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" class="text-center">No services found</td></tr>';
        return;
    }

    let running = 0;
    let stopped = 0;
    let dockerCount = 0;
    let systemdCount = 0;
    let traefikCount = 0;

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
    }
}
