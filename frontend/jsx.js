/**
 * Minimal JSX runtime for the dashboard.
 * Provides createElement (h) and Fragment support for JSX transpilation.
 * Returns HTML strings, not DOM elements, for server-compatible rendering.
 */

// Self-closing tags that don't need closing
const VOID_ELEMENTS = new Set([
    'area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input',
    'link', 'meta', 'param', 'source', 'track', 'wbr'
]);

// Attributes that need special handling
const ATTR_MAP = {
    className: 'class',
    htmlFor: 'for',
    tabIndex: 'tabindex',
    colSpan: 'colspan',
    rowSpan: 'rowspan'
};

// Boolean attributes
const BOOLEAN_ATTRS = new Set([
    'checked', 'disabled', 'hidden', 'readonly', 'required', 'selected',
    'autofocus', 'autoplay', 'controls', 'defer', 'loop', 'multiple',
    'muted', 'novalidate', 'open', 'reversed', 'scoped'
]);

/**
 * Escape HTML special characters.
 * @param {string} str - String to escape
 * @returns {string} Escaped string
 */
function escapeHtml(str) {
    if (str == null) return '';
    return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

/**
 * Create an HTML string from JSX-like syntax.
 * @param {string|Function} tag - HTML tag name or component function
 * @param {Object} props - Element properties/attributes
 * @param {...any} children - Child elements
 * @returns {string} HTML string
 */
export function h(tag, props, ...children) {
    // Handle fragments
    if (tag === Fragment) {
        return children.flat(Infinity).join('');
    }

    // Handle component functions
    if (typeof tag === 'function') {
        return tag({ ...props, children: children.flat(Infinity) });
    }

    // Build attribute string
    let attrs = '';
    if (props) {
        for (const [key, value] of Object.entries(props)) {
            if (key === 'children' || key === 'key' || key === 'ref') continue;
            if (value == null || value === false) continue;

            // Handle event handlers - convert to inline onclick etc.
            if (key.startsWith('on') && typeof value === 'string') {
                attrs += ` ${key.toLowerCase()}="${escapeHtml(value)}"`;
                continue;
            }

            // Skip function event handlers (they can't be serialized to HTML)
            if (typeof value === 'function') continue;

            // Handle style objects
            if (key === 'style' && typeof value === 'object') {
                const styleStr = Object.entries(value)
                    .map(([k, v]) => `${k.replace(/[A-Z]/g, m => '-' + m.toLowerCase())}:${v}`)
                    .join(';');
                attrs += ` style="${escapeHtml(styleStr)}"`;
                continue;
            }

            // Handle dangerouslySetInnerHTML
            if (key === 'dangerouslySetInnerHTML') continue; // Handled in children

            // Map attribute names
            const attrName = ATTR_MAP[key] || key;

            // Boolean attributes
            if (BOOLEAN_ATTRS.has(attrName)) {
                if (value === true) {
                    attrs += ` ${attrName}`;
                }
                continue;
            }

            attrs += ` ${attrName}="${escapeHtml(value)}"`;
        }
    }

    // Handle void elements
    if (VOID_ELEMENTS.has(tag)) {
        return `<${tag}${attrs}>`;
    }

    // Handle dangerouslySetInnerHTML
    if (props && props.dangerouslySetInnerHTML) {
        return `<${tag}${attrs}>${props.dangerouslySetInnerHTML.__html}</${tag}>`;
    }

    // Render children
    const childHtml = children
        .flat(Infinity)
        .map(child => {
            if (child == null || child === false || child === true) return '';
            if (typeof child === 'string' || typeof child === 'number') return escapeHtml(String(child));
            return child; // Already rendered HTML string
        })
        .join('');

    return `<${tag}${attrs}>${childHtml}</${tag}>`;
}

/**
 * Fragment component - renders children without a wrapper element.
 */
export function Fragment({ children }) {
    if (!children) return '';
    return (Array.isArray(children) ? children : [children]).flat(Infinity).join('');
}

/**
 * Render raw HTML without escaping.
 * Use sparingly and only with trusted content.
 * @param {string} html - Raw HTML string
 * @returns {string} The HTML string unchanged
 */
export function raw(html) {
    return html;
}

// Export for JSX automatic runtime
export { h as jsx, h as jsxs, h as jsxDEV };
