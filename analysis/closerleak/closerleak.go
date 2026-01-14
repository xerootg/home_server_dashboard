// Package closerleak provides a static analyzer that detects potential resource leaks
// when types with Close() methods are created but not properly closed.
//
// This is similar to .NET's Roslyn disposable leak detection but for Go's io.Closer pattern.
//
// The analyzer flags cases where:
// 1. A function returns a type with a Close() method
// 2. The return value is assigned to a variable
// 3. Neither defer Close() nor explicit Close() is called on that variable
//
// Usage:
//
//	go run ./analysis/closerleak/cmd/closerleak ./...
package closerleak

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the closerleak analyzer.
var Analyzer = &analysis.Analyzer{
	Name:     "closerleak",
	Doc:      "detects potential resource leaks when Close() is not called on closeable types",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// closerInfo tracks information about a variable that needs Close()
type closerInfo struct {
	pos            token.Pos
	name           string
	typeName       string
	isClosed       bool
	isDeferred     bool
	isReturned     bool // If the variable is returned, caller is responsible for Close()
	isStored       bool // If assigned to a struct field (e.g., m.conn = conn)
	isPassedToFunc bool // If passed to a function that may take ownership
	isWrapped      bool // If wrapped in a struct that's returned
}

// knownCloserCreators maps function names to whether they return closers.
// This supplements type-based detection for interfaces.
var knownCloserCreators = map[string]bool{
	// Standard library
	"os.Open":            true,
	"os.OpenFile":        true,
	"os.Create":          true,
	"net.Dial":           true,
	"net.DialTCP":        true,
	"net.DialUDP":        true,
	"net.Listen":         true,
	"http.Get":           true, // Returns *http.Response with Body io.ReadCloser
	"http.Post":          true,
	"http.DefaultClient": true,
	"sql.Open":           true,
	// Project-specific closers
	"docker.NewProvider":        true,
	"traefik.NewProvider":       true,
	"traefik.NewClient":         true,
	"homeassistant.NewProvider": true,
}

// ignoredPackages contains package paths that should be ignored
var ignoredPackages = map[string]bool{
	"testing": true, // Test files often have intentional patterns
}

func run(pass *analysis.Pass) (interface{}, error) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Track closers within each function
	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.FuncLit)(nil),
	}

	inspect.Preorder(nodeFilter, func(n ast.Node) {
		var body *ast.BlockStmt
		var funcName string

		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body == nil {
				return
			}
			body = fn.Body
			funcName = fn.Name.Name
		case *ast.FuncLit:
			body = fn.Body
			funcName = "<anonymous>"
		}

		// Analyze this function's body
		analyzeFunction(pass, body, funcName)
	})

	return nil, nil
}

func analyzeFunction(pass *analysis.Pass, body *ast.BlockStmt, funcName string) {
	// Track variables that need Close()
	closers := make(map[string]*closerInfo)

	// Single pass: find all closer assignments and check Close() calls
	ast.Inspect(body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			// Check for assignments like: provider, err := NewProvider()
			for i, rhs := range stmt.Rhs {
				if call, ok := rhs.(*ast.CallExpr); ok {
					if isCloserCreator(pass, call) {
						// Find the corresponding LHS variable
						if i < len(stmt.Lhs) {
							if ident, ok := stmt.Lhs[i].(*ast.Ident); ok {
								if ident.Name != "_" { // Ignore blank identifier
									typeName := getCallTypeName(pass, call)
									closers[ident.Name] = &closerInfo{
										pos:      ident.Pos(),
										name:     ident.Name,
										typeName: typeName,
									}
								}
							}
						}
					}
				}
			}

			// Check if assigning to a struct field (e.g., m.conn = conn)
			for i, lhs := range stmt.Lhs {
				if _, ok := lhs.(*ast.SelectorExpr); ok {
					// This is an assignment to a struct field
					if i < len(stmt.Rhs) {
						if ident, ok := stmt.Rhs[i].(*ast.Ident); ok {
							if info, exists := closers[ident.Name]; exists {
								info.isStored = true
							}
						}
					}
				}
			}

		case *ast.DeferStmt:
			// Check for defer x.Close()
			call := stmt.Call
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Close" {
					if ident, ok := sel.X.(*ast.Ident); ok {
						if info, exists := closers[ident.Name]; exists {
							info.isClosed = true
							info.isDeferred = true
						}
					}
				}
			}

		case *ast.ExprStmt:
			// Check for direct x.Close() calls and function calls with closer args
			if call, ok := stmt.X.(*ast.CallExpr); ok {
				checkCallForClose(call, closers)
			}

		case *ast.IfStmt:
			// Check for if err := x.Close(); err != nil { ... }
			if stmt.Init != nil {
				if assign, ok := stmt.Init.(*ast.AssignStmt); ok {
					for _, rhs := range assign.Rhs {
						if call, ok := rhs.(*ast.CallExpr); ok {
							checkCallForClose(call, closers)
						}
					}
				}
			}

		// Check for closers wrapped in composite literals anywhere
		case *ast.CompositeLit:
			for _, elt := range stmt.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if ident, ok := kv.Value.(*ast.Ident); ok {
						if info, exists := closers[ident.Name]; exists {
							info.isWrapped = true
						}
					}
				}
			}

		case *ast.ReturnStmt:
			// If a closer is returned, the caller is responsible for closing it
			for _, result := range stmt.Results {
				if ident, ok := result.(*ast.Ident); ok {
					if info, exists := closers[ident.Name]; exists {
						info.isReturned = true
					}
				}
				// Also check for composite literals that wrap the closer
				// e.g., return &Provider{client: cli}
				if composite, ok := result.(*ast.UnaryExpr); ok {
					if lit, ok := composite.X.(*ast.CompositeLit); ok {
						for _, elt := range lit.Elts {
							if kv, ok := elt.(*ast.KeyValueExpr); ok {
								if ident, ok := kv.Value.(*ast.Ident); ok {
									if info, exists := closers[ident.Name]; exists {
										info.isWrapped = true
									}
								}
							}
						}
					}
				}
				// Handle non-pointer composite literals too
				if lit, ok := result.(*ast.CompositeLit); ok {
					for _, elt := range lit.Elts {
						if kv, ok := elt.(*ast.KeyValueExpr); ok {
							if ident, ok := kv.Value.(*ast.Ident); ok {
								if info, exists := closers[ident.Name]; exists {
									info.isWrapped = true
								}
							}
						}
					}
				}
			}
		}
		return true
	})

	// Report unclosed closers
	for _, info := range closers {
		if !info.isClosed && !info.isReturned && !info.isStored && !info.isPassedToFunc && !info.isWrapped {
			pass.Reportf(info.pos,
				"potential resource leak: %s (type %s) is never closed; add defer %s.Close() after creation",
				info.name, info.typeName, info.name)
		}
	}
}

// checkCallForClose checks if a call expression is a Close() call or passes a closer as argument.
func checkCallForClose(call *ast.CallExpr, closers map[string]*closerInfo) {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if sel.Sel.Name == "Close" {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if info, exists := closers[ident.Name]; exists {
					info.isClosed = true
				}
			}
		}
	}
	// Check if passed to a function (may transfer ownership)
	for _, arg := range call.Args {
		if ident, ok := arg.(*ast.Ident); ok {
			if info, exists := closers[ident.Name]; exists {
				info.isPassedToFunc = true
			}
		}
	}
}

// isCloserCreator checks if a call expression returns a type that has a Close() method.
func isCloserCreator(pass *analysis.Pass, call *ast.CallExpr) bool {
	// Check against known closer creators
	callName := getCallName(call)
	if knownCloserCreators[callName] {
		return true
	}

	// Check if the return type has a Close() method
	callType := pass.TypesInfo.TypeOf(call)
	if callType == nil {
		return false
	}

	return hasCloseMethod(callType)
}

// hasCloseMethod checks if a type has a Close() method (directly or via underlying type).
func hasCloseMethod(t types.Type) bool {
	// Handle tuple types (multiple return values) - check first element
	if tuple, ok := t.(*types.Tuple); ok {
		if tuple.Len() > 0 {
			return hasCloseMethod(tuple.At(0).Type())
		}
		return false
	}

	// Get the method set for this type
	mset := types.NewMethodSet(t)
	for i := 0; i < mset.Len(); i++ {
		if mset.At(i).Obj().Name() == "Close" {
			return true
		}
	}

	// Also check pointer type if this is a named type
	if named, ok := t.(*types.Named); ok {
		ptrType := types.NewPointer(named)
		mset := types.NewMethodSet(ptrType)
		for i := 0; i < mset.Len(); i++ {
			if mset.At(i).Obj().Name() == "Close" {
				return true
			}
		}
	}

	// Check underlying type for interfaces
	if iface, ok := t.Underlying().(*types.Interface); ok {
		for i := 0; i < iface.NumMethods(); i++ {
			if iface.Method(i).Name() == "Close" {
				return true
			}
		}
	}

	return false
}

// getCallName returns the qualified name of a function call (e.g., "docker.NewProvider").
func getCallName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		// package.Function
		if x, ok := fn.X.(*ast.Ident); ok {
			return x.Name + "." + fn.Sel.Name
		}
	case *ast.Ident:
		// Local function
		return fn.Name
	}
	return ""
}

// getCallTypeName returns a human-readable type name for a call expression.
func getCallTypeName(pass *analysis.Pass, call *ast.CallExpr) string {
	callType := pass.TypesInfo.TypeOf(call)
	if callType == nil {
		return getCallName(call)
	}

	// Handle tuple types
	if tuple, ok := callType.(*types.Tuple); ok {
		if tuple.Len() > 0 {
			return formatTypeName(tuple.At(0).Type())
		}
	}

	return formatTypeName(callType)
}

// formatTypeName returns a clean type name string.
func formatTypeName(t types.Type) string {
	s := t.String()
	// Remove package path for readability
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		// Find the next . after the last /
		rest := s[idx+1:]
		if dotIdx := strings.Index(rest, "."); dotIdx >= 0 {
			return rest
		}
	}
	return s
}

// AdditionalChecks contains supplementary validation functions.
type AdditionalChecks struct{}

// CheckForDeferInLoop reports defer statements inside loops, which can cause resource exhaustion.
func CheckForDeferInLoop(pass *analysis.Pass) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.ForStmt)(nil),
		(*ast.RangeStmt)(nil),
	}

	inspect.Preorder(nodeFilter, func(n ast.Node) {
		var body *ast.BlockStmt

		switch loop := n.(type) {
		case *ast.ForStmt:
			body = loop.Body
		case *ast.RangeStmt:
			body = loop.Body
		}

		if body == nil {
			return
		}

		// Find defer statements in loop body
		for _, stmt := range body.List {
			if deferStmt, ok := stmt.(*ast.DeferStmt); ok {
				pass.Reportf(deferStmt.Pos(),
					"defer inside loop; deferred calls will accumulate until function returns, potentially causing resource exhaustion")
			}
		}
	})
}

// AnalyzeProviderLifecycle is a more targeted check for our specific provider pattern.
// It looks for NewProvider calls that aren't followed by defer Close().
func AnalyzeProviderLifecycle(pass *analysis.Pass) {
	inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.BlockStmt)(nil),
	}

	inspect.Preorder(nodeFilter, func(n ast.Node) {
		block := n.(*ast.BlockStmt)

		for i, stmt := range block.List {
			assignStmt, ok := stmt.(*ast.AssignStmt)
			if !ok {
				continue
			}

			// Look for provider := ...NewProvider()
			for j, rhs := range assignStmt.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}

				callName := getCallName(call)
				if !strings.HasSuffix(callName, "NewProvider") && !strings.HasSuffix(callName, "NewClient") {
					continue
				}

				// Get the variable name
				if j >= len(assignStmt.Lhs) {
					continue
				}
				ident, ok := assignStmt.Lhs[j].(*ast.Ident)
				if !ok || ident.Name == "_" {
					continue
				}

				varName := ident.Name

				// Check if the next statements include defer varName.Close()
				found := false
				for k := i + 1; k < len(block.List) && k <= i+5; k++ {
					if isDeferClose(block.List[k], varName) {
						found = true
						break
					}
				}

				if !found {
					// Check if variable is returned (caller responsible)
					if isVariableReturned(block, varName) {
						continue
					}

					pass.Reportf(ident.Pos(),
						"provider %s created with %s but defer %s.Close() not found; this may leak connections",
						varName, callName, varName)
				}
			}
		}
	})
}

func isDeferClose(stmt ast.Stmt, varName string) bool {
	deferStmt, ok := stmt.(*ast.DeferStmt)
	if !ok {
		return false
	}

	call := deferStmt.Call

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Close" {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	return ident.Name == varName
}

func isVariableReturned(block *ast.BlockStmt, varName string) bool {
	var found bool
	ast.Inspect(block, func(n ast.Node) bool {
		if found {
			return false
		}
		retStmt, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range retStmt.Results {
			if ident, ok := result.(*ast.Ident); ok && ident.Name == varName {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// Summary prints a summary of common resource leak patterns to watch for.
func Summary() string {
	return fmt.Sprintf(`
Resource Leak Detection Summary
===============================

Common patterns that cause leaks in this codebase:

1. Missing defer Close() after NewProvider():
   ❌ provider, err := homeassistant.NewProvider(&host)
   ✅ provider, err := homeassistant.NewProvider(&host)
      defer provider.Close()

2. defer inside loops (accumulates until function returns):
   ❌ for _, host := range hosts {
          provider, _ := NewProvider(&host)
          defer provider.Close()  // BAD: all defers run at function end!
      }
   ✅ for _, host := range hosts {
          provider, _ := NewProvider(&host)
          doWork(provider)
          provider.Close()  // Close explicitly in each iteration
      }

3. Error path without cleanup:
   ❌ provider, err := NewProvider()
      if err != nil { return err }  // provider might be non-nil!
      defer provider.Close()
   ✅ provider, err := NewProvider()
      if provider != nil {
          defer provider.Close()
      }
      if err != nil { return err }

Types in this project that require Close():
- *docker.Provider     (holds Docker client connection)
- *traefik.Provider    (holds HTTP client, SSH tunnel)
- *traefik.Client      (holds SSH connection)
- *homeassistant.Provider (holds SSH client for HAOS tunnel)
- *ssh.Client          (SSH connection)
- io.ReadCloser        (log streams, file handles)
`)
}
