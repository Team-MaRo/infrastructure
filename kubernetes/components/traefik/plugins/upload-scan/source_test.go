package uploadscan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// sourceFiles are what Traefik loads: go.mod, the manifest and every non-test .go file.
func sourceFiles(t *testing.T) []string {
	t.Helper()
	files := []string{"go.mod", ".traefik.yml"}
	gos, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(gos)
	for _, f := range gos {
		if !strings.HasSuffix(f, "_test.go") {
			files = append(files, f)
		}
	}
	return files
}

// Traefik reads the plugin only at start; the hash in helmchartconfig.yaml changes with the source
// and so rolls Traefik.
func TestHelmChartConfigNamesThisSource(t *testing.T) {
	h := sha256.New()
	for _, name := range sourceFiles(t) {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(name + "\x00"))
		h.Write(b)
	}
	want := hex.EncodeToString(h.Sum(nil))[:16]

	config, err := os.ReadFile("../../helmchartconfig.yaml")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`upload-scan/source: "([0-9a-f]*)"`).FindSubmatch(config)
	if m == nil || string(m[1]) != want {
		t.Fatalf(`helmchartconfig.yaml must carry upload-scan/source: "%s"`, want)
	}
}

// The ConfigMap Traefik loads the plugin from must carry exactly these files.
func TestKustomizationListsEverySourceFile(t *testing.T) {
	k, err := os.ReadFile("../../kustomization.yaml")
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, m := range regexp.MustCompile(`plugins/upload-scan/([^\s]+)`).FindAllSubmatch(k, -1) {
		listed[string(m[1])] = true
	}
	for _, f := range sourceFiles(t) {
		if !listed[f] {
			t.Errorf("kustomization.yaml does not list plugins/upload-scan/%s", f)
		}
		delete(listed, f)
	}
	for f := range listed {
		t.Errorf("kustomization.yaml lists plugins/upload-scan/%s, which is not a source file", f)
	}
}

// The upload scan's Middleware, one copy per app (kubernetes/charts/cloudflare-only).
const middlewareFile = "../../../../charts/cloudflare-only/files/middleware.yaml"

// The Middleware's settings must be ones New accepts: Traefik decodes them by field name, in any
// case, and a value New refuses breaks every route that uses the Middleware.
func TestMiddlewareSettingsAreAccepted(t *testing.T) {
	b, err := os.ReadFile(middlewareFile)
	if err != nil {
		t.Fatal(err)
	}
	config := CreateConfig()
	fields := reflect.ValueOf(config).Elem()
	var list reflect.Value
	indent := -1
	for _, line := range strings.Split(string(b), "\n") {
		setting := strings.TrimSpace(line)
		depth := len(line) - len(strings.TrimLeft(line, " "))
		if setting == "upload-scan:" {
			indent = depth
			continue
		}
		if indent < 0 || setting == "" || strings.HasPrefix(setting, "#") {
			continue
		}
		if depth <= indent {
			break
		}
		if item, isItem := strings.CutPrefix(setting, "- "); isItem {
			list.Set(reflect.Append(list, reflect.ValueOf(strings.Trim(item, `"`))))
			continue
		}
		key, value, _ := strings.Cut(setting, ":")
		value = strings.Trim(strings.TrimSpace(value), `"`)
		field := fields.FieldByNameFunc(func(name string) bool { return strings.EqualFold(name, key) })
		switch {
		case !field.IsValid():
			t.Fatalf("%s: %s is no setting of Config", middlewareFile, key)
		case field.Kind() == reflect.Slice:
			field.Set(reflect.MakeSlice(field.Type(), 0, 0))
			list = field
		case field.Kind() == reflect.Int64:
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				t.Fatalf("%s: %s: %v", middlewareFile, key, err)
			}
			field.SetInt(n)
		default:
			field.SetString(value)
		}
	}
	if indent < 0 {
		t.Fatalf("%s has no upload-scan settings", middlewareFile)
	}
	if _, err := New(context.Background(), http.NotFoundHandler(), config, "upload-scan"); err != nil {
		t.Fatalf("%s: %v", middlewareFile, err)
	}
}
