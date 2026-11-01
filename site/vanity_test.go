package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	goImportRe = regexp.MustCompile(`<meta name="go-import" content="([^"]*)">`)
	goSourceRe = regexp.MustCompile(`<meta name="go-source" content="([^"]*)">`)
)

// TestVanityPages checks the pages of the local repository against its real package directories.
func TestVanityPages(t *testing.T) {
	pages, err := vanityPages("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zp/index.html", "zp/cmd/zp/index.html", "zp/grammar/index.html", "zp/runtime/index.html", "alpha-parsers/index.html"} {
		if _, ok := pages[name]; !ok {
			t.Errorf("missing vanity page %s", name)
		}
	}
	for name := range pages {
		for _, bad := range []string{"internal", "testdata", "site/"} {
			if strings.Contains("/"+name, "/"+bad) {
				t.Errorf("page %s publishes a package that cannot be imported", name)
			}
		}
	}
	page := string(pages["zp/cmd/zp/index.html"])
	if m := goImportRe.FindStringSubmatch(page); m == nil || m[1] != "zeroparse.dev/zp git https://github.com/zeroparse/zp" {
		t.Errorf("go-import = %v", m)
	}
	want := "zeroparse.dev/zp https://github.com/zeroparse/zp https://github.com/zeroparse/zp/tree/main{/dir} https://github.com/zeroparse/zp/blob/main{/dir}/{file}#L{line}"
	if m := goSourceRe.FindStringSubmatch(page); m == nil || m[1] != want {
		t.Errorf("go-source = %v", m)
	}
}

// TestVanityCoversPackages fails when an importable package directory of the module has no page,
// because the go command requests the full import path first.
func TestVanityCoversPackages(t *testing.T) {
	dirs, err := packageDirs("..")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := vanityPages("..")
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, d := range dirs {
		if _, ok := pages[path.Join("zp", d, "index.html")]; !ok {
			missing = append(missing, d)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("package directories without a page: %v", missing)
	}
	if len(dirs) == 0 || dirs[0] != "" {
		t.Errorf("the module root is not a package directory: %v", dirs)
	}
}

// TestPackageDirsSkips checks which directories are not importable.
func TestPackageDirsSkips(t *testing.T) {
	root := t.TempDir()
	write := func(name string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a/a.go")
	write("a/b/b.go")
	write("onlytests/x_test.go")
	write("internal/i/i.go")
	write("a/testdata/t.go")
	write(".hidden/h.go")
	write("nested/n.go")
	write("nested/go.mod")
	write("root.go")
	got, err := packageDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != ",a,a/b" {
		t.Errorf("packageDirs = %q", got)
	}
}

// TestVanityDiscovery follows the go command's discovery: it requests the import path with
// ?go-get=1, follows redirects, and reads the go-import tag whose prefix is a prefix of the path.
// The test server redirects to the trailing-slash URL as Cloudflare does for
// html_handling = auto-trailing-slash, so this checks that the redirect keeps the query and the
// metadata is found after it.
func TestVanityDiscovery(t *testing.T) {
	pages, err := vanityPages("..")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if !strings.HasSuffix(name, "/") {
			if _, ok := pages[name+"/index.html"]; ok {
				http.Redirect(w, r, r.URL.Path+"/?"+r.URL.RawQuery, http.StatusTemporaryRedirect)
				return
			}
		}
		page, ok := pages[name+"index.html"]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(page)
	}))
	defer srv.Close()
	for _, c := range []struct{ importPath, prefix, repo string }{
		{"zeroparse.dev/zp", "zeroparse.dev/zp", "https://github.com/zeroparse/zp"},
		{"zeroparse.dev/zp/cmd/zp", "zeroparse.dev/zp", "https://github.com/zeroparse/zp"},
		{"zeroparse.dev/zp/grammar", "zeroparse.dev/zp", "https://github.com/zeroparse/zp"},
		{"zeroparse.dev/alpha-parsers", "zeroparse.dev/alpha-parsers", "https://github.com/zeroparse/alpha-parsers"},
	} {
		resp, err := http.Get(srv.URL + strings.TrimPrefix(c.importPath, vanityHost) + "?go-get=1")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("%s: status %d", c.importPath, resp.StatusCode)
			continue
		}
		m := goImportRe.FindStringSubmatch(string(body))
		if m == nil {
			t.Errorf("%s: no go-import tag", c.importPath)
			continue
		}
		f := strings.Fields(m[1])
		if len(f) != 3 || f[0] != c.prefix || f[1] != "git" || f[2] != c.repo {
			t.Errorf("%s: go-import = %q", c.importPath, m[1])
		}
		if c.importPath != f[0] && !strings.HasPrefix(c.importPath, f[0]+"/") {
			t.Errorf("%s: prefix %s is not a path prefix", c.importPath, f[0])
		}
	}
}
