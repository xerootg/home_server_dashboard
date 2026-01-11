/**
 * Service action functions (start/stop/restart).
 */

import { escapeHtml } from './utils.js';
import { actionState } from './state.js';

// Bootstrap Modal reference
let actionModal = null;

/**
 * Get or create Bootstrap Modal instance.
 * @returns {Object} Bootstrap Modal instance
 */
function getActionModal() {
    if (!actionModal && typeof bootstrap !== 'undefined') {
        const modalEl = document.getElementById('actionModal');
        if (modalEl) {
            actionModal = new bootstrap.Modal(modalEl);
        }
    }
    return actionModal;
}

/**
 * Show confirmation modal for a service action.
 * @param {Event} event - The click event
 * @param {string} action - The action (start, stop, restart)
 * @param {string} containerName - The container/unit name
 * @param {string} serviceName - The display service name
 * @param {string} source - The service source (docker, systemd)
 * @param {string} host - The host name
 * @param {string} project - The project name (for docker-compose)
 */
export function confirmServiceAction(event, action, containerName, serviceName, source, host, project) {
    event.stopPropagation();
    
    // Store pending action
    actionState.pending = {
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
    const modal = getActionModal();
    if (modal) {
        modal.show();
    }
}

/**
 * Execute the pending service action with SSE status updates.
 */
export function executeServiceAction() {
    if (!actionState.pending) return;
    
    const { action, containerName, serviceName, source, host, project } = actionState.pending;
    
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
            buffer = lines.pop() || '';
            
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
 * Handle SSE events from service action.
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
 * Add a line to the action status log.
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
 * Show retry/close buttons after action failure.
 */
function showActionRetry() {
    const footer = document.getElementById('actionModalFooter');
    footer.style.display = 'flex';
    footer.innerHTML = `
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Close</button>
        <button type="button" class="btn btn-primary" onclick="window.__dashboard.executeServiceAction()">Retry</button>
    `;
}

/**
 * Start countdown and then close modal.
 * WebSocket handles real-time updates, no refresh needed.
 */
function startCountdown() {
    const countdownEl = document.getElementById('actionCountdown');
    const valueEl = document.getElementById('countdownValue');
    countdownEl.style.display = 'block';
    
    let count = 3;
    valueEl.textContent = count;
    
    const interval = setInterval(() => {
        count--;
        valueEl.textContent = count;
        
        if (count <= 0) {
            clearInterval(interval);
            closeActionModal();
        }
    }, 1000);
}

/**
 * Close the action modal.
 * WebSocket handles real-time updates, no refresh needed.
 */
export function closeActionModal() {
    // Close modal
    const modal = getActionModal();
    if (modal) {
        modal.hide();
    }
    
    // Clear pending action
    actionState.pending = null;
}
