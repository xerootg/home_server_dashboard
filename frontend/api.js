/**
 * API and authentication functions.
 */

import { servicesState, authState } from './state.js';
import { escapeHtml } from './utils.js';

/**
 * Load services from API.
 * @param {Object} callbacks - Callback functions
 * @param {Function} callbacks.onSuccess - Called with services array on success
 * @param {Function} callbacks.onError - Called on error
 * @returns {Promise<Array>} Array of services
 */
export async function loadServices(callbacks = {}) {
    try {
        const response = await fetch('/api/services');
        if (response.status === 401) {
            handleUnauthorized();
            return [];
        }
        if (!response.ok) {
            throw new Error('Failed to fetch services');
        }
        const rawServices = await response.json();
        // Filter out hidden services
        servicesState.all = rawServices.filter(service => !service.hidden);
        
        if (callbacks.onSuccess) {
            callbacks.onSuccess(servicesState.all);
        }
        
        return servicesState.all;
    } catch (error) {
        console.error('Error loading services:', error);
        if (callbacks.onError) {
            callbacks.onError(error);
        }
        return [];
    }
}

/**
 * Check authentication status.
 * @returns {Promise<Object>} Auth status object
 */
export async function checkAuthStatus() {
    try {
        const response = await fetch('/auth/status');
        if (!response.ok) {
            throw new Error('Failed to check auth status');
        }
        authState.status = await response.json();
        updateAuthUI();
        return authState.status;
    } catch (error) {
        console.error('Auth status check failed:', error);
        authState.status = { authenticated: false, oidc_enabled: false };
        updateAuthUI();
        return authState.status;
    }
}

/**
 * Update the UI based on authentication status.
 */
export function updateAuthUI() {
    if (typeof document === 'undefined') return;
    
    const authControls = document.getElementById('authControls');
    const userInfo = document.getElementById('userInfo');
    
    if (!authState.status) {
        if (authControls) authControls.style.display = 'none';
        return;
    }
    
    if (authState.status.oidc_enabled && authState.status.authenticated && authState.status.user) {
        if (authControls) authControls.style.display = 'flex';
        const displayName = authState.status.user.name || authState.status.user.email || 'User';
        if (userInfo) userInfo.innerHTML = `<i class="bi bi-person-circle"></i> ${escapeHtml(displayName)}`;
    } else if (authState.status.oidc_enabled && !authState.status.authenticated) {
        if (authControls) authControls.style.display = 'none';
    } else {
        if (authControls) authControls.style.display = 'none';
    }
}

/**
 * Handle logout.
 */
export function logout() {
    if (typeof window !== 'undefined') {
        window.location.href = '/logout';
    }
}

/**
 * Handle 401 Unauthorized responses.
 */
export function handleUnauthorized() {
    if (authState.status && authState.status.oidc_enabled) {
        if (typeof window !== 'undefined') {
            window.location.href = '/login?redirect=' + encodeURIComponent(window.location.pathname);
        }
    }
}

/**
 * Wrap fetch to handle auth errors.
 * @param {string} url - The URL to fetch
 * @param {Object} options - Fetch options
 * @returns {Promise<Response>} Fetch response
 */
export async function authFetch(url, options = {}) {
    const response = await fetch(url, options);
    if (response.status === 401) {
        handleUnauthorized();
        throw new Error('Authentication required');
    }
    return response;
}
