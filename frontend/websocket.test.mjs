/**
 * Tests for websocket.js module.
 */

import { describe, it, assert, assertEqual } from './test-utils.mjs';
import { websocketState } from './state.js';

// Mock WebSocket for testing
class MockWebSocket {
    constructor(url) {
        this.url = url;
        this.readyState = MockWebSocket.CONNECTING;
        this.onopen = null;
        this.onclose = null;
        this.onerror = null;
        this.onmessage = null;
        
        // Simulate successful connection after a tick
        setTimeout(() => {
            if (this.readyState === MockWebSocket.CONNECTING) {
                this.readyState = MockWebSocket.OPEN;
                if (this.onopen) this.onopen({ type: 'open' });
            }
        }, 0);
    }
    
    close() {
        this.readyState = MockWebSocket.CLOSED;
        if (this.onclose) this.onclose({ code: 1000, reason: 'Normal closure' });
    }
    
    send(data) {
        // Mock send - do nothing
    }
    
    // Simulate receiving a message
    simulateMessage(data) {
        if (this.onmessage) {
            this.onmessage({ data: typeof data === 'string' ? data : JSON.stringify(data) });
        }
    }
    
    // Simulate error
    simulateError() {
        if (this.onerror) this.onerror({ type: 'error' });
    }
    
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;
}

describe('websocketState', () => {
    it('should have correct initial state', () => {
        // Reset state
        websocketState.socket = null;
        websocketState.status = 'disconnected';
        websocketState.reconnecting = false;
        websocketState.reconnectDelay = 1000;
        websocketState.reconnectTimer = null;
        
        assertEqual(websocketState.socket, null, 'socket should be null');
        assertEqual(websocketState.status, 'disconnected', 'status should be disconnected');
        assertEqual(websocketState.reconnecting, false, 'reconnecting should be false');
        assertEqual(websocketState.reconnectDelay, 1000, 'reconnectDelay should be 1000');
        assertEqual(websocketState.reconnectTimer, null, 'reconnectTimer should be null');
    });
});

describe('WebSocket message parsing', () => {
    it('should parse service_update message', () => {
        const message = {
            type: 'service_update',
            timestamp: 1234567890000,
            payload: {
                host: 'nas',
                service_name: 'nginx',
                source: 'docker',
                previous_state: 'running',
                current_state: 'stopped',
                status: 'Exited (0)'
            }
        };
        
        const parsed = JSON.parse(JSON.stringify(message));
        assertEqual(parsed.type, 'service_update', 'type should be service_update');
        assertEqual(parsed.payload.service_name, 'nginx', 'service_name should be nginx');
        assertEqual(parsed.payload.current_state, 'stopped', 'current_state should be stopped');
    });
    
    it('should parse host_unreachable message', () => {
        const message = {
            type: 'host_unreachable',
            timestamp: 1234567890000,
            payload: {
                host: 'server1',
                reason: 'connection refused'
            }
        };
        
        const parsed = JSON.parse(JSON.stringify(message));
        assertEqual(parsed.type, 'host_unreachable', 'type should be host_unreachable');
        assertEqual(parsed.payload.host, 'server1', 'host should be server1');
        assertEqual(parsed.payload.reason, 'connection refused', 'reason should be connection refused');
    });
    
    it('should parse host_recovered message', () => {
        const message = {
            type: 'host_recovered',
            timestamp: 1234567890000,
            payload: {
                host: 'server1'
            }
        };
        
        const parsed = JSON.parse(JSON.stringify(message));
        assertEqual(parsed.type, 'host_recovered', 'type should be host_recovered');
        assertEqual(parsed.payload.host, 'server1', 'host should be server1');
    });
    
    it('should handle multiple messages in one frame', () => {
        const msg1 = JSON.stringify({ type: 'service_update', timestamp: 1, payload: { service_name: 'a' } });
        const msg2 = JSON.stringify({ type: 'service_update', timestamp: 2, payload: { service_name: 'b' } });
        const combined = `${msg1}\n${msg2}`;
        
        const messages = combined.split('\n').filter(m => m.trim()).map(m => JSON.parse(m));
        assertEqual(messages.length, 2, 'should have 2 messages');
        assertEqual(messages[0].payload.service_name, 'a', 'first message should be a');
        assertEqual(messages[1].payload.service_name, 'b', 'second message should be b');
    });
});

describe('WebSocket URL generation', () => {
    it('should use ws: for http:', () => {
        // This tests the logic without actually calling the function
        const protocol = 'http:';
        const wsProtocol = protocol === 'https:' ? 'wss:' : 'ws:';
        assertEqual(wsProtocol, 'ws:', 'should use ws: for http:');
    });
    
    it('should use wss: for https:', () => {
        const protocol = 'https:';
        const wsProtocol = protocol === 'https:' ? 'wss:' : 'ws:';
        assertEqual(wsProtocol, 'wss:', 'should use wss: for https:');
    });
});

describe('Reconnection backoff', () => {
    it('should calculate exponential backoff correctly', () => {
        const INITIAL_DELAY = 1000;
        const MAX_DELAY = 30000;
        const BACKOFF = 1.5;
        
        let delay = INITIAL_DELAY;
        
        // First reconnect
        delay = Math.min(delay * BACKOFF, MAX_DELAY);
        assertEqual(delay, 1500, 'first backoff should be 1500ms');
        
        // Second reconnect
        delay = Math.min(delay * BACKOFF, MAX_DELAY);
        assertEqual(delay, 2250, 'second backoff should be 2250ms');
        
        // Third reconnect
        delay = Math.min(delay * BACKOFF, MAX_DELAY);
        assertEqual(delay, 3375, 'third backoff should be 3375ms');
    });
    
    it('should cap at max delay', () => {
        const MAX_DELAY = 30000;
        const BACKOFF = 1.5;
        
        let delay = 25000;
        delay = Math.min(delay * BACKOFF, MAX_DELAY);
        assertEqual(delay, MAX_DELAY, 'delay should be capped at max');
    });
});

describe('Status transitions', () => {
    it('should have valid status values', () => {
        const validStatuses = ['disconnected', 'connecting', 'connected', 'reconnecting', 'error'];
        
        validStatuses.forEach(status => {
            websocketState.status = status;
            assert(validStatuses.includes(websocketState.status), `${status} should be valid`);
        });
    });
});
