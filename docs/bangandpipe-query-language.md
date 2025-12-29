# BangAndPipe Query Language

A simple yet powerful expression language for filtering log lines with boolean logic.

## Overview

BangAndPipe is a query language designed for filtering text (particularly log output) using intuitive operators. It compiles expressions into an Abstract Syntax Tree (AST) that can be efficiently evaluated against each line of text.

## Quick Start

| Expression | Matches lines that... |
|------------|----------------------|
| `error` | contain "error" |
| `error \| warning` | contain "error" OR "warning" |
| `docker & error` | contain BOTH "docker" AND "error" |
| `!debug` | do NOT contain "debug" |
| `(error \| warning) & !health` | contain "error" or "warning", but NOT "health" |

## Operators

### OR Operator (`|`)

Matches if **any** of the terms match.

```
error | warning | critical
```

This matches lines containing "error", "warning", OR "critical".

**Examples:**
| Expression | Line | Matches? |
|------------|------|----------|
| `error \| warning` | `ERROR: disk full` | ✅ Yes |
| `error \| warning` | `WARNING: low memory` | ✅ Yes |
| `error \| warning` | `INFO: started` | ❌ No |

### AND Operator (`&`)

Matches only if **all** terms match.

```
docker & container & error
```

This matches lines containing "docker" AND "container" AND "error".

**Examples:**
| Expression | Line | Matches? |
|------------|------|----------|
| `docker & error` | `docker container error` | ✅ Yes |
| `docker & error` | `docker started` | ❌ No |
| `docker & error` | `systemd error` | ❌ No |

### NOT Operator (`!`)

Inverts the match—matches lines that do **not** contain the term.

```
!debug
```

This matches all lines that do NOT contain "debug".

**Examples:**
| Expression | Line | Matches? |
|------------|------|----------|
| `!debug` | `ERROR: critical failure` | ✅ Yes |
| `!debug` | `DEBUG: trace info` | ❌ No |
| `!debug` | `debug mode enabled` | ❌ No |

### Grouping with Parentheses (`()`)

Use parentheses to control operator precedence.

```
(error | warning) & !health
```

This matches lines containing "error" or "warning", but excludes any line containing "health".

**Examples:**
| Expression | Line | Matches? |
|------------|------|----------|
| `(error \| warning) & !health` | `ERROR: disk full` | ✅ Yes |
| `(error \| warning) & !health` | `WARNING: check disk` | ✅ Yes |
| `(error \| warning) & !health` | `ERROR: healthcheck failed` | ❌ No |
| `(error \| warning) & !health` | `INFO: started` | ❌ No |

## Quoted Strings

Use double quotes to match literal text containing special characters.

```
"error|warning"
```

This matches the literal text `error|warning`, not "error" OR "warning".

### Escape Sequences

Inside quoted strings:
- `\"` — literal double quote
- `\\` — literal backslash

**Examples:**
| Expression | Matches |
|------------|---------|
| `"hello world"` | The literal text `hello world` |
| `"[ERROR]"` | The literal text `[ERROR]` (brackets not treated as regex) |
| `"path\\to\\file"` | The literal text `path\to\file` |
| `"say \"hello\""` | The literal text `say "hello"` |

## Operator Precedence

Operators are evaluated in this order (highest to lowest precedence):

1. **NOT** (`!`) — Highest precedence
2. **AND** (`&`)
3. **OR** (`|`) — Lowest precedence

This means:
- `A | B & C` is parsed as `A | (B & C)`
- `!A & B` is parsed as `(!A) & B`
- `!A | B & C` is parsed as `(!A) | (B & C)`

Use parentheses to override default precedence when needed.

## Whitespace Handling

Whitespace is significant in unquoted terms but ignored around operators:

| Expression | Interpretation |
|------------|----------------|
| `hello world` | Single term: "hello world" |
| `hello \| world` | OR: "hello" or "world" |
| `hello\|world` | OR: "hello" or "world" |
| `  error  \|  warning  ` | OR: "error" or "warning" |

## Case Sensitivity

By default, matching is **case-insensitive**. The case sensitivity toggle in the UI controls this behavior.

| Expression | Line | Case-Insensitive | Case-Sensitive |
|------------|------|------------------|----------------|
| `error` | `ERROR: fail` | ✅ Yes | ❌ No |
| `error` | `Error: fail` | ✅ Yes | ❌ No |
| `error` | `error: fail` | ✅ Yes | ✅ Yes |

## Real-World Examples

### Filter Docker Errors (excluding healthchecks)

```
docker & error & !health
```

Finds Docker-related errors while filtering out healthcheck noise.

### Monitor Multiple Log Levels

```
error | warning | critical | fatal
```

Shows all lines with concerning log levels.

### Debug a Specific Service

```
nginx & (error | warn) & !404
```

Shows nginx errors and warnings, but hides 404 not-found messages.

### Exclude Verbose Logging

```
!(debug | trace | verbose)
```

Shows everything except debug, trace, and verbose log lines.

### Complex Container Filtering

```
(docker | container) & (error | failed | crash) & !health & !"expected"
```

Finds container errors/failures while excluding healthchecks and expected failures.

### Find Startup or Shutdown Issues

```
(start | stop | restart) & (error | failed | timeout)
```

Finds problems during service lifecycle events.

## Grammar Specification

For those implementing parsers or wanting to understand the formal structure:

```ebnf
expression  = or_expr ;
or_expr     = and_expr { "|" and_expr } ;
and_expr    = unary { "&" unary } ;
unary       = "!" unary | primary ;
primary     = "(" expression ")" | quoted | term ;
quoted      = '"' { char | escape } '"' ;
term        = { char } ;  (* until operator or end *)
escape      = "\\" | '\"' ;
```

### Token Types

| Token | Description |
|-------|-------------|
| `(` | Left parenthesis |
| `)` | Right parenthesis |
| `\|` | OR operator |
| `&` | AND operator |
| `!` | NOT operator |
| `"..."` | Quoted string literal |
| `term` | Unquoted text (ends at operator or EOF) |

## AST Structure

The compiler produces a JSON AST with these node types:

### Pattern Node
```json
{
  "type": "pattern",
  "pattern": "error",
  "regex": "error"
}
```

### OR Node
```json
{
  "type": "or",
  "children": [
    { "type": "pattern", "pattern": "error", "regex": "error" },
    { "type": "pattern", "pattern": "warning", "regex": "warning" }
  ]
}
```

### AND Node
```json
{
  "type": "and",
  "children": [
    { "type": "pattern", "pattern": "docker", "regex": "docker" },
    { "type": "pattern", "pattern": "error", "regex": "error" }
  ]
}
```

### NOT Node
```json
{
  "type": "not",
  "child": { "type": "pattern", "pattern": "debug", "regex": "debug" }
}
```

## Error Handling

The compiler provides detailed error messages with position information:

```json
{
  "valid": false,
  "error": {
    "message": "Unexpected end of expression",
    "position": 6,
    "length": 1
  }
}
```

### Common Errors

| Expression | Error |
|------------|-------|
| `(error` | Unclosed parenthesis |
| `error \|` | Unexpected end of expression |
| `\| error` | Unexpected operator |
| `()` | Empty expression in parentheses |
| `"unterminated` | Unterminated string |

## API Endpoint

The dashboard exposes a compilation endpoint:

```
GET /api/bangAndPipeToRegex?expr=<expression>
```

**Success Response:**
```json
{
  "valid": true,
  "ast": { ... }
}
```

**Error Response:**
```json
{
  "valid": false,
  "error": {
    "message": "Error description",
    "position": 5,
    "length": 1
  }
}
```

## Implementation Notes

- The `regex` field in pattern nodes contains the pattern with regex special characters escaped
- Evaluation is performed client-side using the AST for performance
- Empty expressions are valid and produce a `null` AST (matches nothing)
- OR with no matching children returns `false`
- AND with no children returns `true` (vacuous truth)
