/**
 * Column configuration and management.
 * Handles column visibility, ordering, and localStorage persistence.
 * Settings are stored per-user when authenticated.
 * Mobile and desktop have different default column visibility.
 */

import { authState } from './state.js';

/**
 * Column definitions with metadata.
 * Order matters - this is the default column order.
 * mobileDefault: whether this column is visible by default on mobile devices
 * defaultWidth: default width in pixels (null means auto)
 * minWidth: minimum width in pixels when resizing
 * @type {Array<{id: string, label: string, sortable: boolean, mobileDefault: boolean, defaultWidth: number|null, minWidth: number}>}
 */
export const COLUMN_DEFINITIONS = [
    { id: 'name', label: 'Service', sortable: true, mobileDefault: true, defaultWidth: null, minWidth: 100 },
    { id: 'project', label: 'Project', sortable: true, mobileDefault: false, defaultWidth: null, minWidth: 80 },
    { id: 'host', label: 'Host', sortable: true, mobileDefault: false, defaultWidth: null, minWidth: 80 },
    { id: 'container', label: 'Container', sortable: true, mobileDefault: false, defaultWidth: null, minWidth: 100 },
    { id: 'status', label: 'Status', sortable: true, mobileDefault: true, defaultWidth: null, minWidth: 80 },
    { id: 'image', label: 'Image', sortable: true, mobileDefault: false, defaultWidth: null, minWidth: 100 },
    { id: 'log_size', label: 'Logs', sortable: true, mobileDefault: false, defaultWidth: null, minWidth: 60 },
    { id: 'actions', label: 'Actions', sortable: false, mobileDefault: true, defaultWidth: null, minWidth: 100 }
];

/**
 * Default column configuration (desktop - all visible).
 * Computed from COLUMN_DEFINITIONS.
 * @type {Array<{id: string, label: string, sortable: boolean, visible: boolean, width: number|null}>}
 */
export const DEFAULT_COLUMNS = COLUMN_DEFINITIONS.map(col => ({
    ...col,
    visible: true,
    width: col.defaultWidth
}));

/**
 * Mobile breakpoint width in pixels.
 * Matches Bootstrap's md breakpoint.
 */
const MOBILE_BREAKPOINT = 768;

/** Base storage key for column settings */
const STORAGE_KEY_BASE = 'dashboard_columns';

/**
 * Current column configuration state.
 * Initialized from localStorage or defaults.
 */
export let columnsState = {
    columns: [],
    dropdownOpen: false,
    resizing: null // { columnId, startX, startWidth }
};

/**
 * Detect if the current device is mobile based on screen width.
 * @returns {boolean} - True if mobile device
 */
export function isMobile() {
    if (typeof window === 'undefined') return false;
    return window.innerWidth < MOBILE_BREAKPOINT;
}

/**
 * Get the default columns configuration based on device type.
 * Desktop: all columns visible
 * Mobile: only Service, Status, and Actions visible
 * @returns {Array} - Array of column objects with appropriate visibility
 */
export function getDefaultColumns() {
    const mobile = isMobile();
    return COLUMN_DEFINITIONS.map(col => ({
        ...col,
        visible: mobile ? col.mobileDefault : true,
        width: col.defaultWidth
    }));
}

/**
 * Get the storage key for the current user.
 * Returns a user-specific key if authenticated, otherwise a generic key.
 * @returns {string} - Storage key
 */
export function getStorageKey() {
    const userId = authState.status?.user?.id;
    if (userId) {
        return `${STORAGE_KEY_BASE}_${userId}`;
    }
    return STORAGE_KEY_BASE;
}

/**
 * Check if localStorage is available and functional.
 * @returns {boolean} - True if localStorage can be used
 */
function isLocalStorageAvailable() {
    try {
        if (typeof localStorage === 'undefined') return false;
        if (typeof localStorage.getItem !== 'function') return false;
        if (typeof localStorage.setItem !== 'function') return false;
        return true;
    } catch (e) {
        return false;
    }
}

/**
 * Get a value from localStorage.
 * @param {string} key - Storage key
 * @returns {string|null} - Stored value or null if not found
 */
export function getStorageValue(key) {
    if (!isLocalStorageAvailable()) return null;
    try {
        return localStorage.getItem(key);
    } catch (e) {
        console.warn('Failed to read from localStorage:', e);
        return null;
    }
}

/**
 * Set a value in localStorage.
 * @param {string} key - Storage key
 * @param {string} value - Value to store
 */
export function setStorageValue(key, value) {
    if (!isLocalStorageAvailable()) return;
    try {
        localStorage.setItem(key, value);
    } catch (e) {
        console.warn('Failed to write to localStorage:', e);
    }
}

/**
 * Load column configuration from localStorage or use defaults.
 * @returns {Array} - Array of column objects
 */
export function loadColumnsConfig() {
    const storageKey = getStorageKey();
    const storedValue = getStorageValue(storageKey);
    
    if (storedValue) {
        try {
            const savedConfig = JSON.parse(storedValue);
            // Validate and merge with defaults (in case new columns were added)
            return mergeWithDefaults(savedConfig);
        } catch (e) {
            console.warn('Failed to parse columns config, using defaults:', e);
        }
    }
    
    return getDefaultColumns();
}

/**
 * Merge saved configuration with defaults.
 * Preserves order, visibility, and width from saved config, adds any new columns from defaults.
 * New columns are added with visibility based on device type:
 * - Desktop: visible by default
 * - Mobile: hidden by default (unless marked as mobileDefault)
 * @param {Array} savedConfig - Saved column configuration
 * @returns {Array} - Merged column configuration
 */
export function mergeWithDefaults(savedConfig) {
    const result = [];
    const definitionsMap = new Map(COLUMN_DEFINITIONS.map(col => [col.id, col]));
    const savedIds = new Set(savedConfig.map(col => col.id));
    const mobile = isMobile();
    
    // First, add columns from saved config (preserving order, visibility, and width)
    for (const saved of savedConfig) {
        const colDef = definitionsMap.get(saved.id);
        if (colDef) {
            result.push({
                ...colDef,
                visible: saved.visible !== undefined ? saved.visible : (mobile ? colDef.mobileDefault : true),
                width: saved.width !== undefined ? saved.width : colDef.defaultWidth
            });
        }
    }
    
    // Then, add any new columns from definitions that weren't in saved config
    // New columns: visible on desktop, use mobileDefault on mobile
    for (const colDef of COLUMN_DEFINITIONS) {
        if (!savedIds.has(colDef.id)) {
            result.push({
                ...colDef,
                visible: mobile ? colDef.mobileDefault : true,
                width: colDef.defaultWidth
            });
        }
    }
    
    return result;
}

/**
 * Save column configuration to localStorage.
 * @param {Array} columns - Column configuration to save
 */
export function saveColumnsConfig(columns) {
    // Save id, visible state, and width to localStorage
    const minimalConfig = columns.map(col => ({
        id: col.id,
        visible: col.visible,
        width: col.width
    }));
    const storageKey = getStorageKey();
    setStorageValue(storageKey, JSON.stringify(minimalConfig));
}

/**
 * Initialize columns state from cookie or defaults.
 */
export function initColumnsState() {
    columnsState.columns = loadColumnsConfig();
}

/**
 * Toggle column visibility.
 * @param {string} columnId - Column ID to toggle
 */
export function toggleColumnVisibility(columnId) {
    const column = columnsState.columns.find(col => col.id === columnId);
    if (column) {
        // Don't allow hiding all columns - at least one must remain visible
        const visibleCount = columnsState.columns.filter(col => col.visible).length;
        if (column.visible && visibleCount <= 1) {
            return; // Can't hide the last visible column
        }
        column.visible = !column.visible;
        saveColumnsConfig(columnsState.columns);
    }
}

/**
 * Move a column to a new position.
 * @param {number} fromIndex - Current index
 * @param {number} toIndex - Target index
 */
export function moveColumn(fromIndex, toIndex) {
    if (fromIndex === toIndex) return;
    if (fromIndex < 0 || fromIndex >= columnsState.columns.length) return;
    if (toIndex < 0 || toIndex >= columnsState.columns.length) return;
    
    const [removed] = columnsState.columns.splice(fromIndex, 1);
    columnsState.columns.splice(toIndex, 0, removed);
    saveColumnsConfig(columnsState.columns);
}

/**
 * Reset columns to default configuration.
 * Uses device-appropriate defaults (mobile vs desktop).
 */
export function resetColumnsToDefault() {
    columnsState.columns = getDefaultColumns();
    saveColumnsConfig(columnsState.columns);
}

/**
 * Get visible columns in current order.
 * @returns {Array} - Array of visible column objects
 */
export function getVisibleColumns() {
    return columnsState.columns.filter(col => col.visible);
}

/**
 * Get all columns in current order.
 * @returns {Array} - Array of all column objects
 */
export function getAllColumns() {
    return columnsState.columns;
}

/**
 * Toggle the column settings dropdown.
 */
export function toggleColumnDropdown() {
    columnsState.dropdownOpen = !columnsState.dropdownOpen;
    renderColumnDropdown();
}

/**
 * Close the column settings dropdown.
 */
export function closeColumnDropdown() {
    columnsState.dropdownOpen = false;
    renderColumnDropdown();
}

/**
 * Render the column settings dropdown.
 */
export function renderColumnDropdown() {
    if (typeof document === 'undefined') return;
    
    const container = document.getElementById('columnSettingsDropdown');
    if (!container) return;
    
    if (!columnsState.dropdownOpen) {
        container.style.display = 'none';
        return;
    }
    
    container.style.display = 'block';
    
    const items = columnsState.columns.map((col, index) => {
        const isChecked = col.visible ? 'checked' : '';
        const visibleCount = columnsState.columns.filter(c => c.visible).length;
        const isLastVisible = col.visible && visibleCount === 1;
        const checkboxDisabled = isLastVisible ? 'disabled' : '';
        
        return `
            <li class="column-settings-item" draggable="true" data-index="${index}" data-column-id="${col.id}">
                <span class="column-drag-handle" title="Drag to reorder"><i class="bi bi-grip-vertical"></i></span>
                <label class="column-checkbox-label">
                    <input type="checkbox" ${isChecked} ${checkboxDisabled} onchange="window.__dashboard.toggleColumnVisibility('${col.id}')">
                    <span>${col.label}</span>
                </label>
            </li>
        `;
    }).join('');
    
    container.innerHTML = `
        <div class="column-settings-header">
            <span>Columns</span>
            <button class="column-settings-reset" onclick="window.__dashboard.resetColumns()" title="Reset to defaults">
                <i class="bi bi-arrow-counterclockwise"></i>
            </button>
        </div>
        <ul class="column-settings-list">
            ${items}
        </ul>
    `;
    
    // Add drag and drop handlers
    setupColumnDragDrop();
}

/**
 * Set up drag and drop for column reordering.
 */
function setupColumnDragDrop() {
    if (typeof document === 'undefined') return;
    
    const list = document.querySelector('.column-settings-list');
    if (!list) return;
    
    let draggedItem = null;
    let draggedIndex = -1;
    
    list.querySelectorAll('.column-settings-item').forEach((item) => {
        item.addEventListener('dragstart', (e) => {
            draggedItem = item;
            draggedIndex = parseInt(item.dataset.index);
            item.classList.add('dragging');
            e.dataTransfer.effectAllowed = 'move';
            e.dataTransfer.setData('text/plain', item.dataset.index);
        });
        
        item.addEventListener('dragend', () => {
            item.classList.remove('dragging');
            draggedItem = null;
            draggedIndex = -1;
            // Remove all drag-over classes
            list.querySelectorAll('.column-settings-item').forEach(i => {
                i.classList.remove('drag-over', 'drag-over-top', 'drag-over-bottom');
            });
        });
        
        item.addEventListener('dragover', (e) => {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'move';
            
            if (!draggedItem || item === draggedItem) return;
            
            const rect = item.getBoundingClientRect();
            const midY = rect.top + rect.height / 2;
            
            // Remove existing classes
            item.classList.remove('drag-over-top', 'drag-over-bottom');
            
            if (e.clientY < midY) {
                item.classList.add('drag-over-top');
            } else {
                item.classList.add('drag-over-bottom');
            }
        });
        
        item.addEventListener('dragleave', () => {
            item.classList.remove('drag-over', 'drag-over-top', 'drag-over-bottom');
        });
        
        item.addEventListener('drop', (e) => {
            e.preventDefault();
            
            if (!draggedItem || item === draggedItem) return;
            
            const targetIndex = parseInt(item.dataset.index);
            const rect = item.getBoundingClientRect();
            const midY = rect.top + rect.height / 2;
            
            let newIndex = targetIndex;
            if (e.clientY > midY && targetIndex > draggedIndex) {
                // Dropping below the target item
                newIndex = targetIndex;
            } else if (e.clientY <= midY && targetIndex < draggedIndex) {
                // Dropping above the target item
                newIndex = targetIndex;
            } else if (e.clientY > midY) {
                newIndex = targetIndex + 1;
            }
            
            // Adjust for the removal of the dragged item
            if (newIndex > draggedIndex) {
                newIndex--;
            }
            
            moveColumn(draggedIndex, newIndex);
            
            // Re-render dropdown and table
            renderColumnDropdown();
            
            // Dispatch custom event to trigger table re-render
            if (typeof window !== 'undefined') {
                window.dispatchEvent(new CustomEvent('columnsChanged'));
            }
            
            // Remove all drag-over classes
            list.querySelectorAll('.column-settings-item').forEach(i => {
                i.classList.remove('drag-over', 'drag-over-top', 'drag-over-bottom');
            });
        });
    });
}

/**
 * Apply column visibility changes to the table.
 * @param {Function} rerenderCallback - Callback to re-render the table
 */
export function applyColumnVisibility(rerenderCallback) {
    if (typeof document === 'undefined') return;
    
    // Re-render the table header
    renderTableHeader();
    
    // Dispatch custom event to trigger table re-render
    if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('columnsChanged'));
    }
    
    // Call rerender callback if provided
    if (rerenderCallback) {
        rerenderCallback();
    }
}

/**
 * Render the table header based on current column configuration.
 * Includes resize handles and applies column widths.
 */
export function renderTableHeader() {
    if (typeof document === 'undefined') return;
    
    const thead = document.querySelector('#servicesTable')?.closest('table')?.querySelector('thead tr');
    if (!thead) return;
    
    const visibleColumns = getVisibleColumns();
    
    thead.innerHTML = visibleColumns.map((col, index) => {
        const sortableClass = col.sortable ? 'sortable' : '';
        const sortableAttr = col.sortable ? `data-sort="${col.id}" onclick="window.__dashboard.toggleSort('${col.id}')"` : '';
        const sortIndicator = col.sortable ? '<span class="sort-indicator"></span>' : '';
        const widthStyle = col.width ? `style="width: ${col.width}px; min-width: ${col.minWidth}px;"` : `style="min-width: ${col.minWidth}px;"`;
        
        // Add resize handle to all columns except the last one
        const isLast = index === visibleColumns.length - 1;
        const resizeHandle = !isLast 
            ? `<span class="resize-handle" data-column="${col.id}" onmousedown="window.__dashboard.startColumnResize(event, '${col.id}')" ondblclick="window.__dashboard.resetColumnWidth(event, '${col.id}')"></span>` 
            : '';
        
        return `<th ${sortableAttr} class="${sortableClass} resizable-col" data-column-id="${col.id}" ${widthStyle}><span class="th-content">${col.label}${sortIndicator}</span>${resizeHandle}</th>`;
    }).join('');
    
    // Apply widths to table cells
    applyColumnWidths();
}

/**
 * Apply column widths to table cells.
 */
export function applyColumnWidths() {
    if (typeof document === 'undefined') return;
    
    const table = document.querySelector('#servicesTable')?.closest('table');
    if (!table) return;
    
    const visibleColumns = getVisibleColumns();
    
    // Apply to all rows
    const rows = table.querySelectorAll('tbody tr:not(.logs-row)');
    rows.forEach(row => {
        const cells = row.querySelectorAll('td');
        visibleColumns.forEach((col, index) => {
            if (cells[index]) {
                if (col.width) {
                    cells[index].style.width = `${col.width}px`;
                    cells[index].style.minWidth = `${col.minWidth}px`;
                } else {
                    cells[index].style.width = '';
                    cells[index].style.minWidth = `${col.minWidth}px`;
                }
            }
        });
    });
}

/**
 * Start resizing a column.
 * @param {MouseEvent} e - Mouse event
 * @param {string} columnId - Column ID to resize
 */
export function startColumnResize(e, columnId) {
    e.preventDefault();
    e.stopPropagation();
    
    const th = e.target.closest('th');
    if (!th) return;
    
    const startWidth = th.offsetWidth;
    
    columnsState.resizing = {
        columnId,
        startX: e.clientX,
        startWidth
    };
    
    // Add document-level listeners
    document.addEventListener('mousemove', handleColumnResize);
    document.addEventListener('mouseup', stopColumnResize);
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
}

/**
 * Handle column resize drag.
 * @param {MouseEvent} e - Mouse event
 */
function handleColumnResize(e) {
    if (!columnsState.resizing) return;
    
    const { columnId, startX, startWidth } = columnsState.resizing;
    const diff = e.clientX - startX;
    const col = columnsState.columns.find(c => c.id === columnId);
    if (!col) return;
    
    const newWidth = Math.max(col.minWidth, startWidth + diff);
    
    // Update the column width in state
    col.width = newWidth;
    
    // Apply to header
    const th = document.querySelector(`th[data-column-id="${columnId}"]`);
    if (th) {
        th.style.width = `${newWidth}px`;
    }
    
    // Apply to all cells in that column
    applyColumnWidths();
}

/**
 * Stop resizing a column and persist the new width.
 */
function stopColumnResize() {
    if (!columnsState.resizing) return;
    
    // Save the new configuration
    saveColumnsConfig(columnsState.columns);
    
    columnsState.resizing = null;
    
    // Remove document-level listeners
    document.removeEventListener('mousemove', handleColumnResize);
    document.removeEventListener('mouseup', stopColumnResize);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
}

/**
 * Reset a column width to default (auto).
 * @param {Event} e - Event
 * @param {string} columnId - Column ID to reset
 */
export function resetColumnWidth(e, columnId) {
    e.preventDefault();
    e.stopPropagation();
    
    const col = columnsState.columns.find(c => c.id === columnId);
    if (!col) return;
    
    const colDef = COLUMN_DEFINITIONS.find(c => c.id === columnId);
    col.width = colDef ? colDef.defaultWidth : null;
    
    // Save and re-render
    saveColumnsConfig(columnsState.columns);
    renderTableHeader();
}

/**
 * Handle click outside dropdown to close it.
 * @param {Event} e - Click event
 */
export function handleClickOutside(e) {
    if (!columnsState.dropdownOpen) return;
    
    const dropdown = document.getElementById('columnSettingsDropdown');
    const button = document.getElementById('columnSettingsBtn');
    
    if (dropdown && button && !dropdown.contains(e.target) && !button.contains(e.target)) {
        closeColumnDropdown();
    }
}

/**
 * Initialize click-outside handler.
 */
export function initClickOutsideHandler() {
    if (typeof document === 'undefined') return;
    document.addEventListener('click', handleClickOutside);
}
