/**
 * Tests for filter.js
 */

import { describe, it, assert, assertEqual, assertDeepEqual } from './test-utils.mjs';
import { sortServices, tableTextMatches, serviceMatchesTableSearch, getNextFilterMode } from './filter.js';
import { tableSearchState, resetTableSearchState, servicesState } from './state.js';

describe('getNextFilterMode', () => {
    it('starts with include from null', () => {
        assertEqual(getNextFilterMode(null), 'include');
    });

    it('cycles from include to exclude', () => {
        assertEqual(getNextFilterMode('include'), 'exclude');
    });

    it('cycles from exclude to exclusive', () => {
        assertEqual(getNextFilterMode('exclude'), 'exclusive');
    });

    it('clears filter after exclusive (returns null)', () => {
        assertEqual(getNextFilterMode('exclusive'), null);
    });

    it('starts with include from undefined', () => {
        assertEqual(getNextFilterMode(undefined), 'include');
    });

    it('starts with include from invalid mode', () => {
        assertEqual(getNextFilterMode('invalid'), 'include');
    });
});

describe('sortServices', () => {
    const services = [
        { name: 'zebra', project: 'proj-a', host: 'host1', container_name: 'c1', status: 'running', image: 'img1' },
        { name: 'apple', project: 'proj-c', host: 'host2', container_name: 'c3', status: 'stopped', image: 'img3' },
        { name: 'mango', project: 'proj-b', host: 'host1', container_name: 'c2', status: 'running', image: 'img2' }
    ];

    it('sorts by name ascending', () => {
        const sorted = sortServices([...services], 'name', 'asc');
        assertEqual(sorted[0].name, 'apple');
        assertEqual(sorted[1].name, 'mango');
        assertEqual(sorted[2].name, 'zebra');
    });

    it('sorts by name descending', () => {
        const sorted = sortServices([...services], 'name', 'desc');
        assertEqual(sorted[0].name, 'zebra');
        assertEqual(sorted[1].name, 'mango');
        assertEqual(sorted[2].name, 'apple');
    });

    it('sorts by project', () => {
        const sorted = sortServices([...services], 'project', 'asc');
        assertEqual(sorted[0].project, 'proj-a');
        assertEqual(sorted[1].project, 'proj-b');
        assertEqual(sorted[2].project, 'proj-c');
    });

    it('sorts by host', () => {
        const sorted = sortServices([...services], 'host', 'asc');
        assertEqual(sorted[0].host, 'host1');
        assertEqual(sorted[2].host, 'host2');
    });

    it('sorts by status', () => {
        const sorted = sortServices([...services], 'status', 'asc');
        assertEqual(sorted[0].status, 'running');
        assertEqual(sorted[2].status, 'stopped');
    });

    it('handles empty array', () => {
        const sorted = sortServices([], 'name', 'asc');
        assertEqual(sorted.length, 0);
    });
});

describe('Table Search - Plain Text', () => {
    it('empty search matches everything', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        assertEqual(tableTextMatches('any text', ''), true);
    });

    it('case insensitive by default', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        assertEqual(tableTextMatches('Docker Container', 'docker'), true);
        assertEqual(tableTextMatches('DOCKER', 'docker'), true);
        assertEqual(tableTextMatches('docker', 'DOCKER'), true);
    });

    it('case sensitive when enabled', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.caseSensitive = true;
        assertEqual(tableTextMatches('Docker Container', 'docker'), false);
        assertEqual(tableTextMatches('Docker Container', 'Docker'), true);
    });

    it('partial matches work', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        assertEqual(tableTextMatches('my-container-name', 'container'), true);
        assertEqual(tableTextMatches('nginx:latest', 'nginx'), true);
    });

    it('no match returns false', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        assertEqual(tableTextMatches('docker container', 'systemd'), false);
    });
});

describe('Table Search - Regex Mode', () => {
    it('basic regex pattern', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.regex = true;
        assertEqual(tableTextMatches('container-123', 'container-\\d+'), true);
        assertEqual(tableTextMatches('container-abc', 'container-\\d+'), false);
    });

    it('regex with anchors', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.regex = true;
        assertEqual(tableTextMatches('nginx', '^nginx$'), true);
        assertEqual(tableTextMatches('my-nginx', '^nginx$'), false);
    });

    it('regex alternation', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.regex = true;
        assertEqual(tableTextMatches('docker', 'docker|systemd'), true);
        assertEqual(tableTextMatches('systemd', 'docker|systemd'), true);
        assertEqual(tableTextMatches('podman', 'docker|systemd'), false);
    });

    it('inverse regex with ! prefix', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.regex = true;
        assertEqual(tableTextMatches('systemd', '!docker'), true);
        assertEqual(tableTextMatches('docker', '!docker'), false);
    });

    it('escaped ! at start matches literal !', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.regex = true;
        assertEqual(tableTextMatches('!important', '\\!important'), true);
        assertEqual(tableTextMatches('!other', '\\!important'), false);
    });

    it('invalid regex returns false', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.regex = true;
        assertEqual(tableTextMatches('text', '[invalid'), false);
    });
});

describe('Table Search - Bang & Pipe Mode', () => {
    it('simple pattern with AST', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = true;
        tableSearchState.ast = { type: 'pattern', pattern: 'docker', regex: 'docker' };
        assertEqual(tableTextMatches('docker container', 'docker'), true);
        assertEqual(tableTextMatches('systemd unit', 'docker'), false);
    });

    it('OR expression', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = true;
        tableSearchState.ast = {
            type: 'or',
            children: [
                { type: 'pattern', pattern: 'docker', regex: 'docker' },
                { type: 'pattern', pattern: 'systemd', regex: 'systemd' }
            ]
        };
        assertEqual(tableTextMatches('docker', 'ignored'), true);
        assertEqual(tableTextMatches('systemd', 'ignored'), true);
        assertEqual(tableTextMatches('podman', 'ignored'), false);
    });

    it('AND expression', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = true;
        tableSearchState.ast = {
            type: 'and',
            children: [
                { type: 'pattern', pattern: 'docker', regex: 'docker' },
                { type: 'pattern', pattern: 'running', regex: 'running' }
            ]
        };
        assertEqual(tableTextMatches('docker running', 'ignored'), true);
        assertEqual(tableTextMatches('docker stopped', 'ignored'), false);
        assertEqual(tableTextMatches('systemd running', 'ignored'), false);
    });

    it('NOT expression', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = true;
        tableSearchState.ast = {
            type: 'not',
            child: { type: 'pattern', pattern: 'stopped', regex: 'stopped' }
        };
        assertEqual(tableTextMatches('running', 'ignored'), true);
        assertEqual(tableTextMatches('stopped', 'ignored'), false);
    });

    it('returns false without valid AST', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = true;
        tableSearchState.ast = null;
        assertEqual(tableTextMatches('docker', 'docker'), false);
    });
});

describe('Table Search - Service Matching', () => {
    const sampleService = {
        name: 'nginx',
        project: 'web-stack',
        host: 'nas',
        container_name: 'web-nginx-1',
        status: 'running (healthy)',
        state: 'running',
        image: 'nginx:1.25-alpine',
        source: 'docker',
        ports: [
            { host_port: 8080, container_port: 80, protocol: 'tcp' },
            { host_port: 8443, container_port: 443, protocol: 'tcp' }
        ],
        traefik_urls: [
            'https://nginx.example.com',
            'https://web.mysite.org'
        ]
    };

    it('matches service name', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = 'nginx';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('matches project', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = 'web-stack';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('matches host', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = 'nas';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('matches container name', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = 'web-nginx-1';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('matches status', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = 'healthy';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('matches state', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = 'running';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('matches image', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = 'alpine';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('matches source', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = 'docker';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('matches port number', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = '8080';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
        tableSearchState.term = '8443';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('matches traefik hostname', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = 'nginx.example.com';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
        tableSearchState.term = 'web.mysite.org';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('matches partial traefik hostname', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = 'example.com';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
        tableSearchState.term = 'mysite';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('no match returns false', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.term = 'postgresql';
        assertEqual(serviceMatchesTableSearch(sampleService), false);
    });

    it('handles missing fields gracefully', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        const minimalService = { name: 'test' };
        tableSearchState.term = 'test';
        assertEqual(serviceMatchesTableSearch(minimalService), true);
        tableSearchState.term = 'missing';
        assertEqual(serviceMatchesTableSearch(minimalService), false);
    });

    it('regex matching on services', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = false;
        tableSearchState.regex = true;
        tableSearchState.term = 'nginx.*alpine';
        assertEqual(serviceMatchesTableSearch(sampleService), true);
    });

    it('bang and pipe on services', () => {
        resetTableSearchState();
        tableSearchState.bangAndPipe = true;
        tableSearchState.term = 'docker & running';
        tableSearchState.ast = {
            type: 'and',
            children: [
                { type: 'pattern', pattern: 'docker', regex: 'docker' },
                { type: 'pattern', pattern: 'running', regex: 'running' }
            ]
        };
        assertEqual(serviceMatchesTableSearch(sampleService), true);
        
        tableSearchState.ast = {
            type: 'and',
            children: [
                { type: 'pattern', pattern: 'systemd', regex: 'systemd' },
                { type: 'pattern', pattern: 'running', regex: 'running' }
            ]
        };
        assertEqual(serviceMatchesTableSearch(sampleService), false);
    });
});
