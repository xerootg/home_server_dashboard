/**
 * WebSocket module for real-time updates.
 * Provides connection management with auto-reconnect and message handling.
 */

import { websocketState } from './state.js';

// Message types (must match backend)
const MessageType = {
    SERVICE_UPDATE: 'service_update',
    HOST_UNREACHABLE: 'host_unreachable',
    HOST_RECOVERED: 'host_recovered',
    PING: 'ping'
};

// Reconnection settings
const INITIAL_RECONNECT_DELAY = 1000; // 1 second
const MAX_RECONNECT_DELAY = 30000;    // 30 seconds
const RECONNECT_BACKOFF = 1.5;        // Exponential backoff multiplier

// Callbacks registered for different event types
let eventCallbacks = {
    service_update: [],
    host_unreachable: [],
    host_recovered: [],
    connect: [],
    disconnect: [],
    error: []
};

/**
 * Get the WebSocket URL based on current page location.
 * @returns {string} WebSocket URL
 */
function getWebSocketURL() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${window.location.host}/ws`;
}

/**
 * Connect to the WebSocket server.
 */
export function connect() {
    if (websocketState.socket && 
        (websocketState.socket.readyState === WebSocket.OPEN || 
         websocketState.socket.readyState === WebSocket.CONNECTING)) {
        return; // Already connected or connecting
    }

    const url = getWebSocketURL();
    console.log('WebSocket: connecting to', url);
    
    try {
        websocketState.socket = new WebSocket(url);
        websocketState.status = 'connecting';
        updateConnectionIndicator();
        
        websocketState.socket.onopen = handleOpen;
        websocketState.socket.onclose = handleClose;
        websocketState.socket.onerror = handleError;
        websocketState.socket.onmessage = handleMessage;
    } catch (error) {
        console.error('WebSocket: connection error', error);
        scheduleReconnect();
    }
}

/**
 * Disconnect from the WebSocket server.
 */
export function disconnect() {
    websocketState.reconnecting = false;
    if (websocketState.reconnectTimer) {
        clearTimeout(websocketState.reconnectTimer);
        websocketState.reconnectTimer = null;
    }
    
    if (websocketState.socket) {
        websocketState.socket.close();
        websocketState.socket = null;
    }
    
    websocketState.status = 'disconnected';
    updateConnectionIndicator();
}

/**
 * Handle WebSocket open event.
 */
function handleOpen() {
    console.log('WebSocket: connected');
    websocketState.status = 'connected';
    websocketState.reconnectDelay = INITIAL_RECONNECT_DELAY;
    websocketState.reconnecting = false;
    updateConnectionIndicator();
    
    // Notify connect listeners
    eventCallbacks.connect.forEach(cb => {
        try {
            cb();
        } catch (e) {
            console.error('WebSocket: connect callback error', e);
        }
    });
}

/**
 * Handle WebSocket close event.
 * @param {CloseEvent} event - The close event
 */
function handleClose(event) {
    console.log('WebSocket: disconnected', event.code, event.reason);
    websocketState.status = 'disconnected';
    websocketState.socket = null;
    updateConnectionIndicator();
    
    // Notify disconnect listeners
    eventCallbacks.disconnect.forEach(cb => {
        try {
            cb(event);
        } catch (e) {
            console.error('WebSocket: disconnect callback error', e);
        }
    });
    
    // Auto-reconnect unless intentionally closed
    if (event.code !== 1000) {
        scheduleReconnect();
    }
}

/**
 * Handle WebSocket error event.
 * @param {Event} event - The error event
 */
function handleError(event) {
    console.error('WebSocket: error', event);
    websocketState.status = 'error';
    updateConnectionIndicator();
    
    // Notify error listeners
    eventCallbacks.error.forEach(cb => {
        try {
            cb(event);
        } catch (e) {
            console.error('WebSocket: error callback error', e);
        }
    });
}

/**
 * Handle incoming WebSocket message.
 * @param {MessageEvent} event - The message event
 */
function handleMessage(event) {
    try {
        // Handle multiple messages in one frame (newline separated)
        const messages = event.data.split('\n').filter(m => m.trim());
        
        for (const data of messages) {
            const message = JSON.parse(data);
            
            switch (message.type) {
                case MessageType.SERVICE_UPDATE:
                    handleServiceUpdate(message.payload, message.timestamp);
                    break;
                case MessageType.HOST_UNREACHABLE:
                    handleHostUnreachable(message.payload, message.timestamp);
                    break;
                case MessageType.HOST_RECOVERED:
                    handleHostRecovered(message.payload, message.timestamp);
                    break;
                case MessageType.PING:
                    // Ping messages are handled by the WebSocket protocol
                    break;
                default:
                    console.log('WebSocket: unknown message type', message.type);
            }
        }
    } catch (error) {
        console.error('WebSocket: failed to parse message', error, event.data);
    }
}

/**
 * Handle service update message.
 * @param {Object} payload - The service update payload
 * @param {number} timestamp - The event timestamp
 */
function handleServiceUpdate(payload, timestamp) {
    console.log('WebSocket: service update', payload);
    
    eventCallbacks.service_update.forEach(cb => {
        try {
            cb(payload, timestamp);
        } catch (e) {
            console.error('WebSocket: service_update callback error', e);
        }
    });
}

/**
 * Handle host unreachable message.
 * @param {Object} payload - The host event payload
 * @param {number} timestamp - The event timestamp
 */
function handleHostUnreachable(payload, timestamp) {
    console.log('WebSocket: host unreachable', payload);
    
    eventCallbacks.host_unreachable.forEach(cb => {
        try {
            cb(payload, timestamp);
        } catch (e) {
            console.error('WebSocket: host_unreachable callback error', e);
        }
    });
}

/**
 * Handle host recovered message.
 * @param {Object} payload - The host event payload
 * @param {number} timestamp - The event timestamp
 */
function handleHostRecovered(payload, timestamp) {
    console.log('WebSocket: host recovered', payload);
    
    eventCallbacks.host_recovered.forEach(cb => {
        try {
            cb(payload, timestamp);
        } catch (e) {
            console.error('WebSocket: host_recovered callback error', e);
        }
    });
}

/**
 * Schedule a reconnection attempt with exponential backoff.
 */
function scheduleReconnect() {
    if (websocketState.reconnecting) {
        return; // Already scheduled
    }
    
    websocketState.reconnecting = true;
    websocketState.status = 'reconnecting';
    updateConnectionIndicator();
    
    console.log(`WebSocket: reconnecting in ${websocketState.reconnectDelay}ms`);
    
    websocketState.reconnectTimer = setTimeout(() => {
        websocketState.reconnectTimer = null;
        connect();
        
        // Increase delay for next attempt (with max cap)
        websocketState.reconnectDelay = Math.min(
            websocketState.reconnectDelay * RECONNECT_BACKOFF,
            MAX_RECONNECT_DELAY
        );
    }, websocketState.reconnectDelay);
}

/**
 * Update the connection status indicator in the UI.
 */
function updateConnectionIndicator() {
    if (typeof document === 'undefined') return;
    
    const indicator = document.getElementById('wsStatus');
    if (!indicator) return;
    
    // Remove all status classes
    indicator.classList.remove('ws-connected', 'ws-disconnected', 'ws-connecting', 'ws-error');
    
    switch (websocketState.status) {
        case 'connected':
            indicator.classList.add('ws-connected');
            indicator.title = 'Real-time updates connected';
            indicator.innerHTML = '<i class="bi bi-broadcast"></i>';
            break;
        case 'connecting':
        case 'reconnecting':
            indicator.classList.add('ws-connecting');
            indicator.title = 'Connecting...';
            indicator.innerHTML = '<i class="bi bi-broadcast"></i>';
            break;
        case 'error':
            indicator.classList.add('ws-error');
            indicator.title = 'Connection error';
            indicator.innerHTML = '<i class="bi bi-broadcast"></i>';
            break;
        default:
            indicator.classList.add('ws-disconnected');
            indicator.title = 'Real-time updates disconnected';
            indicator.innerHTML = '<i class="bi bi-broadcast"></i>';
    }
}

/**
 * Register a callback for a specific event type.
 * @param {string} eventType - Event type: 'service_update', 'host_unreachable', 'host_recovered', 'connect', 'disconnect', 'error'
 * @param {Function} callback - Callback function
 * @returns {Function} Unsubscribe function
 */
export function on(eventType, callback) {
    if (!eventCallbacks[eventType]) {
        console.warn('WebSocket: unknown event type', eventType);
        return () => {};
    }
    
    eventCallbacks[eventType].push(callback);
    
    // Return unsubscribe function
    return () => {
        const index = eventCallbacks[eventType].indexOf(callback);
        if (index !== -1) {
            eventCallbacks[eventType].splice(index, 1);
        }
    };
}

/**
 * Get the current connection status.
 * @returns {string} Connection status
 */
export function getStatus() {
    return websocketState.status;
}

/**
 * Check if connected.
 * @returns {boolean} True if connected
 */
export function isConnected() {
    return websocketState.status === 'connected';
}
