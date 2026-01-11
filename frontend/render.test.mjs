/**
 * Tests for services.js and render.js
 */

import { describe, it, assert, assertEqual, assertDeepEqual } from './test-utils.mjs';
import { servicesState, authState } from './state.js';
import { getServiceHostIP } from './services.js';
import { renderPorts, renderTraefikURLs, getSourceIcons, renderControlButtons, renderLogSize, getUniqueHosts } from './render.js';

describe('getServiceHostIP', () => {
    it('returns host_ip for matching service', () => {
        servicesState.all = [
            { name: 'test-service', host: 'host1', host_ip: '192.168.1.10' },
            { name: 'other-service', host: 'host1', host_ip: '192.168.1.20' }
        ];
        
        assertEqual(getServiceHostIP('test-service', 'host1'), '192.168.1.10');
        assertEqual(getServiceHostIP('other-service', 'host1'), '192.168.1.20');
    });

    it('returns null for non-existent service', () => {
        servicesState.all = [
            { name: 'test-service', host: 'host1', host_ip: '192.168.1.10' }
        ];
        
        assertEqual(getServiceHostIP('nonexistent', 'host1'), null);
    });

    it('matches both name and host', () => {
        servicesState.all = [
            { name: 'test-service', host: 'host1', host_ip: '192.168.1.10' },
            { name: 'test-service', host: 'host2', host_ip: '192.168.1.20' }
        ];
        
        assertEqual(getServiceHostIP('test-service', 'host1'), '192.168.1.10');
        assertEqual(getServiceHostIP('test-service', 'host2'), '192.168.1.20');
    });

    it('returns null for wrong host', () => {
        servicesState.all = [
            { name: 'gluetun', host: 'nas', host_ip: '192.168.1.10' }
        ];
        assertEqual(getServiceHostIP('gluetun', 'other-host'), null);
    });

    it('handles empty allServices array', () => {
        servicesState.all = [];
        assertEqual(getServiceHostIP('gluetun', 'nas'), null);
    });
});

describe('renderPorts', () => {
    it('returns empty string for null ports', () => {
        assertEqual(renderPorts(null, '192.168.1.1', {}), '');
    });

    it('returns empty string for empty ports array', () => {
        assertEqual(renderPorts([], '192.168.1.1', {}), '');
    });

    it('filters out hidden ports', () => {
        const ports = [
            { host_port: 80, protocol: 'tcp', hidden: false },
            { host_port: 443, protocol: 'tcp', hidden: true }
        ];
        const result = renderPorts(ports, '192.168.1.1', {});
        assert(result.includes(':80'), 'Should include port 80');
        assert(!result.includes(':443'), 'Should not include port 443');
    });

    it('renders port with custom label', () => {
        const ports = [
            { host_port: 8080, protocol: 'tcp', label: 'Admin' }
        ];
        const result = renderPorts(ports, '192.168.1.1', {});
        assert(result.includes('Admin'), 'Should include label');
        assert(result.includes('http://192.168.1.1:8080'), 'Should include URL');
    });

    it('uses localhost when hostIP is null', () => {
        const ports = [{ host_port: 3000, protocol: 'tcp' }];
        const result = renderPorts(ports, null, { host: 'nas' });
        assert(result.includes('http://localhost:3000'), 'Should use localhost');
    });

    it('renders target_service port as scroll action', () => {
        const ports = [{ host_port: 8193, protocol: 'tcp', target_service: 'qbittorrent' }];
        const service = { host: 'nas' };
        const result = renderPorts(ports, '192.168.1.10', service);
        assert(result.includes('qbittorrent'), 'Should include target service name');
        assert(result.includes('8193'), 'Should include port number');
        assert(result.includes('scrollToService'), 'Should include scroll function');
        assert(result.includes('bg-secondary'), 'Should have secondary badge class');
    });

    it('renders source_service port with source IP', () => {
        servicesState.all = [
            { name: 'gluetun', host: 'nas', host_ip: '192.168.1.10' },
            { name: 'qbittorrent', host: 'nas', host_ip: '192.168.1.10' }
        ];
        const ports = [{ host_port: 8193, protocol: 'tcp', source_service: 'gluetun' }];
        const service = { name: 'qbittorrent', host: 'nas' };
        const result = renderPorts(ports, '192.168.1.10', service);
        assert(result.includes('gluetun:8193'), 'Should include source service and port');
        assert(result.includes('http://192.168.1.10:8193'), 'Should include URL with source IP');
    });

    it('falls back to target host when source service not found', () => {
        servicesState.all = [
            { name: 'firefox', host: 'nas', host_ip: '192.168.1.10' }
        ];
        const ports = [{ host_port: 8193, protocol: 'tcp', source_service: 'missing-vpn' }];
        const service = { name: 'firefox', host: 'nas' };
        const result = renderPorts(ports, '10.0.0.1', service);
        assert(result.includes('http://10.0.0.1:8193'), 'Should fall back to passed hostIP');
    });

    it('handles mixed port types', () => {
        servicesState.all = [
            { name: 'gluetun', host: 'nas', host_ip: '192.168.1.10' },
            { name: 'qbittorrent', host: 'nas', host_ip: '192.168.1.10' }
        ];
        const ports = [
            { host_port: 8080, protocol: 'tcp' },
            { host_port: 8193, protocol: 'tcp', target_service: 'qbittorrent' },
            { host_port: 9000, protocol: 'tcp', label: 'Admin' }
        ];
        const service = { name: 'gluetun', host: 'nas' };
        const result = renderPorts(ports, '192.168.1.10', service);
        
        assert(result.includes(':8080'), 'Should include regular port');
        assert(result.includes('qbittorrent'), 'Should include target service name');
        assert(result.includes('Admin'), 'Should include labeled port');
    });
});

describe('renderTraefikURLs', () => {
    it('returns empty string for null URLs', () => {
        assertEqual(renderTraefikURLs(null), '');
    });

    it('returns empty string for empty URLs array', () => {
        assertEqual(renderTraefikURLs([]), '');
    });

    it('renders URL badges', () => {
        const urls = ['https://app.example.com', 'https://api.example.com'];
        const result = renderTraefikURLs(urls);
        assert(result.includes('app.example.com'), 'Should include first hostname');
        assert(result.includes('api.example.com'), 'Should include second hostname');
        assert(result.includes('bg-success'), 'Should have success badge class');
    });
});

describe('getSourceIcons', () => {
    it('returns Docker icon for docker source', () => {
        const result = getSourceIcons({ source: 'docker' });
        assert(result.includes('bi-box'), 'Should include box icon');
        assert(result.includes('text-primary'), 'Should be primary color');
    });

    it('returns systemd icon for systemd source', () => {
        const result = getSourceIcons({ source: 'systemd' });
        assert(result.includes('bi-gear-fill'), 'Should include gear icon');
        assert(result.includes('text-info'), 'Should be info color');
    });

    it('returns traefik icon for traefik source', () => {
        const result = getSourceIcons({ source: 'traefik' });
        assert(result.includes('bi-signpost-split'), 'Should include signpost icon');
        assert(result.includes('text-warning'), 'Should be warning color');
    });

    it('returns homeassistant icon for homeassistant source', () => {
        const result = getSourceIcons({ source: 'homeassistant' });
        assert(result.includes('bi-house-heart-fill'), 'Should include house-heart icon');
        assert(result.includes('text-primary'), 'Should be primary color');
    });

    it('returns homeassistant-addon icon for homeassistant-addon source', () => {
        const result = getSourceIcons({ source: 'homeassistant-addon' });
        assert(result.includes('bi-puzzle-fill'), 'Should include puzzle icon');
        assert(result.includes('text-info'), 'Should be info color');
    });

    it('adds traefik icon when service has traefik_urls', () => {
        const result = getSourceIcons({ source: 'docker', traefik_urls: ['https://app.example.com'] });
        assert(result.includes('bi-box'), 'Should include docker icon');
        assert(result.includes('bi-signpost-split'), 'Should include traefik icon');
    });
});

describe('renderControlButtons', () => {
    it('renders start button when service is stopped', () => {
        const service = {
            state: 'stopped',
            container_name: 'test-container',
            name: 'test-service',
            source: 'docker',
            host: 'host1',
            project: 'test-project'
        };
        const result = renderControlButtons(service);
        assert(result.includes('btn-start'), 'Should include start button');
        assert(result.includes('bi-play-fill'), 'Should include play icon');
    });

    it('renders stop button when service is running', () => {
        const service = {
            state: 'running',
            container_name: 'test-container',
            name: 'test-service',
            source: 'docker',
            host: 'host1',
            project: 'test-project'
        };
        const result = renderControlButtons(service);
        assert(result.includes('btn-stop'), 'Should include stop button');
        assert(result.includes('bi-stop-fill'), 'Should include stop icon');
    });

    it('always renders restart button', () => {
        const service = {
            state: 'running',
            container_name: 'test-container',
            name: 'test-service',
            source: 'docker',
            host: 'host1',
            project: 'test-project'
        };
        const result = renderControlButtons(service);
        assert(result.includes('btn-restart'), 'Should include restart button');
        assert(result.includes('bi-arrow-clockwise'), 'Should include restart icon');
    });
});

describe('renderLogSize', () => {
    it('returns dash for non-docker services', () => {
        const service = { source: 'systemd', log_size: 1000 };
        const result = renderLogSize(service);
        assert(result.includes('-'), 'Should show dash');
        assert(result.includes('text-muted'), 'Should be muted');
    });

    it('returns dash for docker services without log_size', () => {
        const service = { source: 'docker', container_name: 'test', name: 'test', host: 'host1' };
        const result = renderLogSize(service);
        assert(result.includes('-'), 'Should show dash');
    });

    it('returns dash for docker services with zero log_size', () => {
        const service = { source: 'docker', log_size: 0, container_name: 'test', name: 'test', host: 'host1' };
        const result = renderLogSize(service);
        assert(result.includes('-'), 'Should show dash');
    });

    it('renders readonly button for non-admin users', () => {
        authState.status = { user: { is_admin: false } };
        const service = { source: 'docker', log_size: 1024, container_name: 'test', name: 'test', host: 'host1' };
        const result = renderLogSize(service);
        assert(result.includes('btn-logs-readonly'), 'Should have readonly class');
        assert(result.includes('1.00K'), 'Should show formatted size');
        assert(!result.includes('confirmLogFlush'), 'Should not have onclick handler');
    });

    it('renders clickable button for admin users', () => {
        authState.status = { user: { is_admin: true } };
        const service = { source: 'docker', log_size: 1048576, container_name: 'test-container', name: 'test-service', host: 'host1' };
        const result = renderLogSize(service);
        assert(!result.includes('btn-logs-readonly'), 'Should not have readonly class');
        assert(result.includes('btn-logs'), 'Should have logs button class');
        assert(result.includes('1.00M'), 'Should show formatted size');
        assert(result.includes('confirmLogFlush'), 'Should have onclick handler');
        assert(result.includes('test-container'), 'Should include container name');
    });
});

describe('getUniqueHosts', () => {
    it('returns empty array for empty services', () => {
        const result = getUniqueHosts([]);
        assertDeepEqual(result, []);
    });

    it('extracts unique hosts from services', () => {
        const services = [
            { name: 'svc1', host: 'host1' },
            { name: 'svc2', host: 'host2' },
            { name: 'svc3', host: 'host1' }
        ];
        const result = getUniqueHosts(services);
        assertDeepEqual(result, ['host1', 'host2']);
    });

    it('sorts hosts alphabetically', () => {
        const services = [
            { name: 'svc1', host: 'zeta' },
            { name: 'svc2', host: 'alpha' },
            { name: 'svc3', host: 'mango' }
        ];
        const result = getUniqueHosts(services);
        assertDeepEqual(result, ['alpha', 'mango', 'zeta']);
    });

    it('ignores services without host', () => {
        const services = [
            { name: 'svc1', host: 'host1' },
            { name: 'svc2' },
            { name: 'svc3', host: '' }
        ];
        const result = getUniqueHosts(services);
        assertDeepEqual(result, ['host1']);
    });
});
