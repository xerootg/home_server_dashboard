/**
 * Tests for columns.js module.
 */

import { describe, it, assert, assertEqual, assertDeepEqual } from './test-utils.mjs';
import {
    COLUMN_DEFINITIONS,
    DEFAULT_COLUMNS,
    mergeWithDefaults,
    loadColumnsConfig,
    getStorageKey,
    getStorageValue,
    setStorageValue,
    getDefaultColumns,
    isMobile,
    columnsState,
    initColumnsState,
    toggleColumnVisibility,
    moveColumn,
    resetColumnsToDefault,
    getVisibleColumns,
    getAllColumns,
    startColumnResize,
    resetColumnWidth
} from './columns.js';

describe('COLUMN_DEFINITIONS', () => {
    it('should have all expected columns', () => {
        const columnIds = COLUMN_DEFINITIONS.map(c => c.id);
        assert(columnIds.includes('name'), 'Should have name column');
        assert(columnIds.includes('project'), 'Should have project column');
        assert(columnIds.includes('host'), 'Should have host column');
        assert(columnIds.includes('container'), 'Should have container column');
        assert(columnIds.includes('status'), 'Should have status column');
        assert(columnIds.includes('image'), 'Should have image column');
        assert(columnIds.includes('log_size'), 'Should have log_size column');
        assert(columnIds.includes('actions'), 'Should have actions column');
    });
    
    it('should have correct mobileDefault flags', () => {
        const mobileDefaults = COLUMN_DEFINITIONS.filter(c => c.mobileDefault).map(c => c.id);
        assertDeepEqual(mobileDefaults, ['name', 'status', 'actions'], 'Mobile defaults should be name, status, actions');
    });
    
    it('should have correct sortable flags', () => {
        const sortableIds = COLUMN_DEFINITIONS.filter(c => c.sortable).map(c => c.id);
        assert(sortableIds.includes('name'), 'name should be sortable');
        assert(sortableIds.includes('status'), 'status should be sortable');
        assert(!COLUMN_DEFINITIONS.find(c => c.id === 'actions').sortable, 'actions should not be sortable');
    });
    
    it('should have minWidth for all columns', () => {
        COLUMN_DEFINITIONS.forEach(col => {
            assert(typeof col.minWidth === 'number', `Column ${col.id} should have minWidth`);
            assert(col.minWidth > 0, `Column ${col.id} minWidth should be positive`);
        });
    });
    
    it('should have defaultWidth defined (null for auto)', () => {
        COLUMN_DEFINITIONS.forEach(col => {
            assert(col.defaultWidth === null || typeof col.defaultWidth === 'number', 
                `Column ${col.id} defaultWidth should be null or number`);
        });
    });
});

describe('DEFAULT_COLUMNS', () => {
    it('should have all columns visible (desktop default)', () => {
        DEFAULT_COLUMNS.forEach(col => {
            assert(col.visible === true, `Column ${col.id} should be visible by default`);
        });
    });
    
    it('should be derived from COLUMN_DEFINITIONS', () => {
        assertEqual(DEFAULT_COLUMNS.length, COLUMN_DEFINITIONS.length, 'Should have same number of columns');
    });
    
    it('should have width property from defaultWidth', () => {
        DEFAULT_COLUMNS.forEach((col, i) => {
            assertEqual(col.width, COLUMN_DEFINITIONS[i].defaultWidth, 
                `Column ${col.id} width should match defaultWidth`);
        });
    });
});

describe('getDefaultColumns', () => {
    it('should return columns with visibility (desktop mode in Node.js)', () => {
        // In Node.js, isMobile() returns false, so we get desktop defaults
        const defaults = getDefaultColumns();
        assertEqual(defaults.length, COLUMN_DEFINITIONS.length, 'Should have all columns');
        defaults.forEach(col => {
            assert(col.visible === true, `Column ${col.id} should be visible on desktop`);
        });
    });
    
    it('should include width property', () => {
        const defaults = getDefaultColumns();
        defaults.forEach((col, i) => {
            assertEqual(col.width, COLUMN_DEFINITIONS[i].defaultWidth,
                `Column ${col.id} should have width from defaultWidth`);
        });
    });
});

describe('isMobile', () => {
    it('should return false in Node.js (no window)', () => {
        assertEqual(isMobile(), false, 'Should be false in Node.js');
    });
});

describe('mergeWithDefaults', () => {
    it('should preserve order from saved config', () => {
        const saved = [
            { id: 'status', visible: true },
            { id: 'name', visible: true },
            { id: 'host', visible: false }
        ];
        const result = mergeWithDefaults(saved);
        
        assertEqual(result[0].id, 'status', 'First column should be status');
        assertEqual(result[1].id, 'name', 'Second column should be name');
        assertEqual(result[2].id, 'host', 'Third column should be host');
    });
    
    it('should preserve visibility from saved config', () => {
        const saved = [
            { id: 'name', visible: false },
            { id: 'project', visible: true }
        ];
        const result = mergeWithDefaults(saved);
        
        const nameCol = result.find(c => c.id === 'name');
        const projectCol = result.find(c => c.id === 'project');
        
        assertEqual(nameCol.visible, false, 'name should be hidden');
        assertEqual(projectCol.visible, true, 'project should be visible');
    });
    
    it('should preserve width from saved config', () => {
        const saved = [
            { id: 'name', visible: true, width: 200 },
            { id: 'project', visible: true, width: 150 }
        ];
        const result = mergeWithDefaults(saved);
        
        const nameCol = result.find(c => c.id === 'name');
        const projectCol = result.find(c => c.id === 'project');
        
        assertEqual(nameCol.width, 200, 'name should have saved width');
        assertEqual(projectCol.width, 150, 'project should have saved width');
    });
    
    it('should use default width for columns without saved width', () => {
        const saved = [
            { id: 'name', visible: true }  // No width specified
        ];
        const result = mergeWithDefaults(saved);
        
        const nameCol = result.find(c => c.id === 'name');
        const nameDef = COLUMN_DEFINITIONS.find(c => c.id === 'name');
        assertEqual(nameCol.width, nameDef.defaultWidth, 'name should use default width');
    });
    
    it('should add missing columns from defaults at the end', () => {
        const saved = [
            { id: 'name', visible: true }
        ];
        const result = mergeWithDefaults(saved);
        
        // Should have all columns
        assertEqual(result.length, DEFAULT_COLUMNS.length, 'Should have all columns');
        
        // First should be name
        assertEqual(result[0].id, 'name', 'First should be name');
        
        // Rest should be from defaults (minus name)
        const expectedRest = DEFAULT_COLUMNS.filter(c => c.id !== 'name').map(c => c.id);
        const actualRest = result.slice(1).map(c => c.id);
        assertDeepEqual(actualRest, expectedRest, 'Rest should be from defaults');
    });
    
    it('should ignore unknown columns from saved config', () => {
        const saved = [
            { id: 'unknown_column', visible: true },
            { id: 'name', visible: true }
        ];
        const result = mergeWithDefaults(saved);
        
        assert(!result.find(c => c.id === 'unknown_column'), 'Should not include unknown column');
        assertEqual(result.length, DEFAULT_COLUMNS.length, 'Should only have default columns');
    });
    
    it('should preserve sortable and label from defaults', () => {
        const saved = [
            { id: 'actions', visible: true }
        ];
        const result = mergeWithDefaults(saved);
        
        const actionsCol = result.find(c => c.id === 'actions');
        assertEqual(actionsCol.sortable, false, 'actions should not be sortable');
        assertEqual(actionsCol.label, 'Actions', 'actions should have correct label');
    });
});

describe('columnsState management', () => {
    // Reset state before each test by manually setting
    const resetState = () => {
        columnsState.columns = JSON.parse(JSON.stringify(DEFAULT_COLUMNS));
        columnsState.dropdownOpen = false;
    };
    
    it('initColumnsState should set columns from defaults when no cookie', () => {
        resetState();
        initColumnsState();
        assertEqual(columnsState.columns.length, DEFAULT_COLUMNS.length, 'Should have all columns');
    });
    
    it('toggleColumnVisibility should toggle visible state', () => {
        resetState();
        const nameCol = columnsState.columns.find(c => c.id === 'name');
        const originalVisible = nameCol.visible;
        
        toggleColumnVisibility('name');
        
        assertEqual(nameCol.visible, !originalVisible, 'Should toggle visibility');
    });
    
    it('toggleColumnVisibility should not hide last visible column', () => {
        resetState();
        // Hide all but one column
        columnsState.columns.forEach(col => {
            col.visible = col.id === 'name';
        });
        
        toggleColumnVisibility('name');
        
        const nameCol = columnsState.columns.find(c => c.id === 'name');
        assertEqual(nameCol.visible, true, 'Should not hide last visible column');
    });
    
    it('moveColumn should reorder columns', () => {
        resetState();
        const originalFirst = columnsState.columns[0].id;
        const originalSecond = columnsState.columns[1].id;
        
        moveColumn(0, 1);
        
        assertEqual(columnsState.columns[0].id, originalSecond, 'Second should now be first');
        assertEqual(columnsState.columns[1].id, originalFirst, 'First should now be second');
    });
    
    it('moveColumn should handle invalid indices gracefully', () => {
        resetState();
        const originalOrder = columnsState.columns.map(c => c.id);
        
        moveColumn(-1, 0);
        moveColumn(0, 100);
        moveColumn(100, 0);
        
        assertDeepEqual(columnsState.columns.map(c => c.id), originalOrder, 'Order should be unchanged');
    });
    
    it('resetColumnsToDefault should restore default config', () => {
        resetState();
        // Make some changes
        columnsState.columns[0].visible = false;
        moveColumn(0, 3);
        
        resetColumnsToDefault();
        
        // In Node.js, isMobile() returns false, so we get desktop defaults (all visible)
        const expected = getDefaultColumns();
        assertDeepEqual(
            columnsState.columns.map(c => ({ id: c.id, visible: c.visible })),
            expected.map(c => ({ id: c.id, visible: c.visible })),
            'Should match device-appropriate defaults'
        );
    });
    
    it('getVisibleColumns should return only visible columns', () => {
        resetState();
        columnsState.columns[1].visible = false;
        columnsState.columns[3].visible = false;
        
        const visible = getVisibleColumns();
        
        assertEqual(visible.length, DEFAULT_COLUMNS.length - 2, 'Should have 2 less columns');
        assert(!visible.find(c => c.id === columnsState.columns[1].id), 'Should not include hidden column');
    });
    
    it('getAllColumns should return all columns', () => {
        resetState();
        columnsState.columns[1].visible = false;
        
        const all = getAllColumns();
        
        assertEqual(all.length, DEFAULT_COLUMNS.length, 'Should have all columns');
    });
});

describe('localStorage functions', () => {
    it('getStorageKey should return base key when no user is logged in', () => {
        const key = getStorageKey();
        assertEqual(key, 'dashboard_columns', 'Should return base key');
    });
    
    it('getStorageValue should return null when localStorage is undefined', () => {
        const result = getStorageValue('test');
        assertEqual(result, null, 'Should return null in Node.js');
    });
    
    it('setStorageValue should not throw when localStorage is undefined', () => {
        // Just ensure it doesn't throw
        setStorageValue('test', 'value');
        assert(true, 'Should not throw');
    });
});
