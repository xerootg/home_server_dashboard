/**
 * Core search functionality shared between logs and table search.
 * Pure functions with no DOM dependencies - testable in Node.js.
 */

/**
 * Parse a search term to extract pattern and inverse flag.
 * @param {string} searchTerm - The search term
 * @param {boolean} isRegexMode - Whether regex mode is enabled
 * @returns {{pattern: string, isInverse: boolean}} Parsed search term
 */
export function parseSearchTerm(searchTerm, isRegexMode) {
    if (!searchTerm) return { pattern: '', isInverse: false };
    
    // Only support ! prefix in regex mode
    if (!isRegexMode) {
        return { pattern: searchTerm, isInverse: false };
    }
    
    // Check for escaped \! at start
    if (searchTerm.startsWith('\\!')) {
        return { pattern: searchTerm.slice(2), isInverse: false };
    }
    
    // Check for ! prefix (inverse match)
    if (searchTerm.startsWith('!')) {
        return { pattern: searchTerm.slice(1), isInverse: true };
    }
    
    return { pattern: searchTerm, isInverse: false };
}

/**
 * Check if text matches a search term.
 * @param {string} text - The text to search in
 * @param {string} searchTerm - The search term
 * @param {Object} options - Search options
 * @param {boolean} options.caseSensitive - Whether to match case-sensitively
 * @param {boolean} options.regex - Whether to use regex matching
 * @param {boolean} options.bangAndPipe - Whether bang-and-pipe mode is enabled
 * @param {Object|null} options.ast - The compiled AST for bang-and-pipe mode
 * @returns {boolean} Whether the text matches
 */
export function textMatches(text, searchTerm, options = {}) {
    const { caseSensitive = false, regex = false, bangAndPipe = false, ast = null } = options;
    
    if (!searchTerm) return false;
    
    // Bang-and-pipe mode: use AST evaluation
    if (bangAndPipe) {
        if (!ast) return false;
        return evaluateAST(ast, text, caseSensitive);
    }
    
    const { pattern, isInverse } = parseSearchTerm(searchTerm, regex);
    if (!pattern) return isInverse; // Empty pattern after ! means match all (inverse of nothing)
    
    let matches;
    if (regex) {
        try {
            const flags = caseSensitive ? '' : 'i';
            const regexObj = new RegExp(pattern, flags);
            matches = regexObj.test(text);
        } catch (e) {
            // Invalid regex - no match
            return false;
        }
    } else {
        if (caseSensitive) {
            matches = text.includes(pattern);
        } else {
            matches = text.toLowerCase().includes(pattern.toLowerCase());
        }
    }
    
    return isInverse ? !matches : matches;
}

/**
 * Evaluate a Bang & Pipe AST against text.
 * @param {Object} ast - The AST node
 * @param {string} text - The text to evaluate against
 * @param {boolean} caseSensitive - Whether to match case-sensitively
 * @returns {boolean} Whether the AST matches the text
 */
export function evaluateAST(ast, text, caseSensitive = false) {
    if (!ast) return false;
    
    switch (ast.type) {
        case 'pattern':
            // Use the regex field from the AST
            try {
                const flags = caseSensitive ? '' : 'i';
                const regex = new RegExp(ast.regex, flags);
                return regex.test(text);
            } catch (e) {
                return false;
            }
        case 'or':
            return ast.children.some(child => evaluateAST(child, text, caseSensitive));
        case 'and':
            return ast.children.every(child => evaluateAST(child, text, caseSensitive));
        case 'not':
            return !evaluateAST(ast.child, text, caseSensitive);
        default:
            return false;
    }
}

/**
 * Create a regex for highlighting matches in text.
 * @param {string} searchTerm - The search term
 * @param {Object} options - Search options
 * @param {boolean} options.caseSensitive - Whether to match case-sensitively
 * @param {boolean} options.regex - Whether to use regex matching
 * @param {boolean} options.bangAndPipe - Whether bang-and-pipe mode is enabled
 * @returns {RegExp|null} Regex for highlighting, or null if not applicable
 */
export function getSearchRegex(searchTerm, options = {}) {
    const { caseSensitive = false, regex = false, bangAndPipe = false } = options;
    
    if (!searchTerm) return null;
    
    // For bang-and-pipe mode, we don't use regex highlighting
    if (bangAndPipe) return null;
    
    const { pattern } = parseSearchTerm(searchTerm, regex);
    if (!pattern) return null;
    
    try {
        const flags = caseSensitive ? 'g' : 'gi';
        if (regex) {
            return new RegExp(`(${pattern})`, flags);
        } else {
            const escapedTerm = pattern.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
            return new RegExp(`(${escapedTerm})`, flags);
        }
    } catch (e) {
        return null;
    }
}

/**
 * Check if search term has inverse prefix in regex mode.
 * @param {string} searchTerm - The search term
 * @param {boolean} isRegexMode - Whether regex mode is enabled
 * @returns {boolean} Whether the search is inverse
 */
export function hasInversePrefix(searchTerm, isRegexMode) {
    if (!isRegexMode) return false;
    if (!searchTerm) return false;
    if (searchTerm.startsWith('\\!')) return false;
    return searchTerm.startsWith('!');
}

/**
 * Collect all matches in text for navigation.
 * @param {string} text - The text to search in
 * @param {string} searchTerm - The search term
 * @param {Object} options - Search options
 * @returns {Array<{position: number, length: number}>} Array of match positions
 */
export function findAllMatches(text, searchTerm, options = {}) {
    const { caseSensitive = false, regex = false, bangAndPipe = false, ast = null } = options;
    const matches = [];
    
    if (!searchTerm) return matches;
    
    if (bangAndPipe) {
        // Bang-and-pipe mode: each matching line is one "match" (whole line)
        if (ast && evaluateAST(ast, text, caseSensitive)) {
            matches.push({ position: 0, length: text.length, isLineMatch: true });
        }
        return matches;
    }
    
    const { pattern, isInverse } = parseSearchTerm(searchTerm, regex);
    
    if (isInverse) {
        // Inverse mode: if line doesn't match, it's a "match" (whole line)
        let lineMatches = false;
        if (pattern) {
            try {
                const flags = caseSensitive ? '' : 'i';
                const regexObj = new RegExp(pattern, flags);
                lineMatches = regexObj.test(text);
            } catch (e) {
                // Invalid regex - consider it not matching
            }
        }
        if (!lineMatches) {
            matches.push({ position: 0, length: text.length, isLineMatch: true });
        }
        return matches;
    }
    
    if (!pattern) return matches;
    
    if (regex) {
        try {
            const flags = caseSensitive ? 'g' : 'gi';
            const regexObj = new RegExp(pattern, flags);
            let match;
            while ((match = regexObj.exec(text)) !== null) {
                matches.push({ position: match.index, length: match[0].length });
                if (match[0].length === 0) regexObj.lastIndex++;
            }
        } catch (e) {
            // Invalid regex - no matches
        }
    } else {
        const searchStr = caseSensitive ? searchTerm : searchTerm.toLowerCase();
        const textStr = caseSensitive ? text : text.toLowerCase();
        
        let pos = 0;
        while ((pos = textStr.indexOf(searchStr, pos)) !== -1) {
            matches.push({ position: pos, length: searchStr.length });
            pos += searchStr.length;
        }
    }
    
    return matches;
}
