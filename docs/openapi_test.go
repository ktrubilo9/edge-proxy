package docs_test

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v2"
)

const expectedAdminRouteCount = 18

type openAPIDocument struct {
	Paths map[string]openAPIPathItem `yaml:"paths"`
}

type openAPIPathItem struct {
	Get     *openAPIOperation `yaml:"get"`
	Put     *openAPIOperation `yaml:"put"`
	Post    *openAPIOperation `yaml:"post"`
	Delete  *openAPIOperation `yaml:"delete"`
	Options *openAPIOperation `yaml:"options"`
	Head    *openAPIOperation `yaml:"head"`
	Patch   *openAPIOperation `yaml:"patch"`
	Trace   *openAPIOperation `yaml:"trace"`
}

type openAPIOperation struct {
	OperationID string `yaml:"operationId"`
}

func TestOpenAPICoversRegisteredAdminRoutes(t *testing.T) {
	t.Parallel()

	registered := registeredAdminRoutes(t)
	if got := len(registered); got != expectedAdminRouteCount {
		t.Fatalf("registered operation count = %d, want %d", got, expectedAdminRouteCount)
	}

	specSource, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI document: %v", err)
	}

	var document openAPIDocument
	if err := yaml.Unmarshal(specSource, &document); err != nil {
		t.Fatalf("parse OpenAPI document: %v", err)
	}
	documented := make(map[string]struct{})
	operationIDs := make(map[string]int)
	for path, item := range document.Paths {
		for method, operation := range item.operations() {
			if operation == nil {
				continue
			}
			documented[method+" "+path] = struct{}{}
			operationIDs[operation.OperationID]++
		}
	}

	assertSameOperations(t, registered, documented)
	if got, want := len(operationIDs), len(documented); got != want {
		t.Fatalf("unique operationId count = %d, want %d", got, want)
	}
	for id, count := range operationIDs {
		if id == "" || count != 1 {
			t.Errorf("operationId %q occurs %d times", id, count)
		}
	}
}

func registeredAdminRoutes(t *testing.T) map[string]struct{} {
	t.Helper()

	directory := filepath.Join("..", "cmd", "admin-api")
	fileSet := token.NewFileSet()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read admin API directory: %v", err)
	}

	packageFiles := make([]*ast.File, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		matches, matchErr := build.Default.MatchFile(directory, entry.Name())
		if matchErr != nil {
			t.Fatalf("match admin API file %s: %v", entry.Name(), matchErr)
		}
		if !matches || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		path := filepath.Join(directory, entry.Name())
		file, parseErr := parser.ParseFile(fileSet, path, nil, parser.AllErrors)
		if parseErr != nil {
			t.Fatalf("parse admin API file %s: %v", path, parseErr)
		}
		packageFiles = append(packageFiles, file)
	}
	if len(packageFiles) == 0 {
		t.Fatalf("no Go files found in admin API directory %s", directory)
	}
	constants := stringConstants(packageFiles)

	routes := make(map[string]struct{})
	var extractionErrors []string
	for _, file := range packageFiles {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "HandleFunc" {
				return true
			}
			position := fileSet.Position(call.Pos())
			if len(call.Args) < 2 {
				extractionErrors = append(extractionErrors, fmt.Sprintf("%s: HandleFunc has fewer than two arguments", position))
				return true
			}
			pattern, ok := stringExpression(call.Args[0], constants)
			if !ok {
				extractionErrors = append(extractionErrors, fmt.Sprintf("%s: HandleFunc pattern is not a resolvable string constant", position))
				return true
			}
			method, path, ok := strings.Cut(pattern, " ")
			if !ok || method == "" || !strings.HasPrefix(path, "/") {
				extractionErrors = append(extractionErrors, fmt.Sprintf("%s: unsupported HandleFunc pattern %q", position, pattern))
				return true
			}
			routes[strings.ToUpper(method)+" "+path] = struct{}{}
			return true
		})
	}
	if len(extractionErrors) > 0 {
		t.Fatalf("extract admin API routes:\n%s", strings.Join(extractionErrors, "\n"))
	}
	return routes
}

func stringConstants(files []*ast.File) map[string]string {
	constants := make(map[string]string)
	for changed := true; changed; {
		changed = false
		for _, file := range files {
			for _, declaration := range file.Decls {
				generic, ok := declaration.(*ast.GenDecl)
				if !ok || generic.Tok != token.CONST {
					continue
				}
				for _, specification := range generic.Specs {
					values, ok := specification.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for index, name := range values.Names {
						if index >= len(values.Values) {
							continue
						}
						value, ok := stringExpression(values.Values[index], constants)
						if !ok || constants[name.Name] == value {
							continue
						}
						constants[name.Name] = value
						changed = true
					}
				}
			}
		}
	}
	return constants
}

func stringExpression(expression ast.Expr, constants map[string]string) (string, bool) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return "", false
		}
		unquoted, err := strconv.Unquote(value.Value)
		return unquoted, err == nil
	case *ast.Ident:
		resolved, ok := constants[value.Name]
		return resolved, ok
	case *ast.ParenExpr:
		return stringExpression(value.X, constants)
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return "", false
		}
		left, leftOK := stringExpression(value.X, constants)
		right, rightOK := stringExpression(value.Y, constants)
		return left + right, leftOK && rightOK
	default:
		return "", false
	}
}

func (item openAPIPathItem) operations() map[string]*openAPIOperation {
	return map[string]*openAPIOperation{
		"GET":     item.Get,
		"PUT":     item.Put,
		"POST":    item.Post,
		"DELETE":  item.Delete,
		"OPTIONS": item.Options,
		"HEAD":    item.Head,
		"PATCH":   item.Patch,
		"TRACE":   item.Trace,
	}
}

func assertSameOperations(t *testing.T, registered, documented map[string]struct{}) {
	t.Helper()

	var missing, extra []string
	for operation := range registered {
		if _, ok := documented[operation]; !ok {
			missing = append(missing, operation)
		}
	}
	for operation := range documented {
		if _, ok := registered[operation]; !ok {
			extra = append(extra, operation)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("OpenAPI route mismatch\nmissing: %v\nextra: %v", missing, extra)
	}
}
