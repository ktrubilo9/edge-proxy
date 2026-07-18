package docs_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var routePattern = regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) ([^"]+)"`)

func TestOpenAPICoversRegisteredAdminRoutes(t *testing.T) {
	t.Parallel()

	mainSource, err := os.ReadFile(filepath.Join("..", "cmd", "admin-api", "main.go"))
	if err != nil {
		t.Fatalf("read route registrations: %v", err)
	}

	registered := make(map[string]struct{})
	for _, match := range routePattern.FindAllStringSubmatch(string(mainSource), -1) {
		registered[match[1]+" "+match[2]] = struct{}{}
	}
	if got, want := len(registered), 18; got != want {
		t.Fatalf("registered operation count = %d, want %d", got, want)
	}

	specFile, err := os.Open("openapi.yaml")
	if err != nil {
		t.Fatalf("open OpenAPI document: %v", err)
	}
	defer specFile.Close()

	documented := make(map[string]struct{})
	operationIDs := make(map[string]int)
	var currentPath string
	scanner := bufio.NewScanner(specFile)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "  /") && strings.HasSuffix(line, ":") {
			currentPath = strings.TrimSuffix(strings.TrimSpace(line), ":")
			continue
		}
		if currentPath != "" && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") {
			method := strings.TrimSuffix(strings.TrimSpace(line), ":")
			switch method {
			case "get", "post", "put", "patch", "delete":
				documented[strings.ToUpper(method)+" "+currentPath] = struct{}{}
			}
		}
		if strings.HasPrefix(line, "      operationId: ") {
			id := strings.TrimSpace(strings.TrimPrefix(line, "      operationId: "))
			operationIDs[id]++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan OpenAPI document: %v", err)
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
