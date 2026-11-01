package main

import (
	"fmt"
	"html"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Go vanity imports. The module path of the repository is zeroparse.dev/zp, and zeroparse.dev/X
// resolves to https://github.com/zeroparse/X for the repositories of the organization. The go
// command (and the module proxy) discover this by requesting https://zeroparse.dev/X/...?go-get=1
// and reading go-import meta tags, so the site publishes one small HTML page per importable
// package directory instead of running a script. See docs/site.md.
const (
	vanityHost   = "zeroparse.dev"
	vanityOrgURL = "https://github.com/zeroparse"
	vanityBranch = "main"
)

// vanityRepo is a repository of the organization served as zeroparse.dev/<Name>.
type vanityRepo struct {
	Name string
	// Local reports that the repository is the one being built, so a page is published for every
	// importable package directory of it. The go command asks for the full import path first, and
	// a path without a page would answer 404 instead of the module's metadata.
	Local bool
}

// vanityRepos lists the repositories of the organization that Go code imports. Adding a repository
// means adding it here and deploying the site.
var vanityRepos = []vanityRepo{
	{Name: "zp", Local: true},
	{Name: "alpha-parsers"},
}

// vanityPages returns the files of the vanity import pages, keyed by output path. repo is the
// root of the repository being built.
func vanityPages(repo string) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, r := range vanityRepos {
		dirs := []string{""}
		if r.Local {
			var err error
			if dirs, err = packageDirs(repo); err != nil {
				return nil, err
			}
		}
		for _, d := range dirs {
			files[path.Join(r.Name, d, "index.html")] = vanityPage(r.Name, d)
		}
	}
	return files, nil
}

// vanityPage renders the page for the import path zeroparse.dev/<repo>/<dir>. Every page names the
// repository root as the go-import prefix, so that the go command resolves a package to its module
// from any of them.
func vanityPage(repo, dir string) []byte {
	prefix := vanityHost + "/" + repo
	gh := vanityOrgURL + "/" + repo
	importPath := prefix
	if dir != "" {
		importPath += "/" + dir
	}
	doc := "https://pkg.go.dev/" + importPath
	return []byte(fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>%[1]s</title>
<meta name="go-import" content="%[2]s git %[3]s">
<meta name="go-source" content="%[2]s %[3]s %[3]s/tree/%[4]s{/dir} %[3]s/blob/%[4]s{/dir}/{file}#L{line}">
<meta http-equiv="refresh" content="0; url=%[5]s">
</head>
<body>
<p>Go package <code>%[1]s</code>. See the <a href="%[5]s">documentation</a> or the <a href="%[3]s">source repository</a>.</p>
</body>
</html>
`, html.EscapeString(importPath), prefix, gh, vanityBranch, doc))
}

// packageDirs returns the directories of the repository, relative to its root with forward
// slashes ("" is the root), that an outside module can import: directories with a non-test Go
// file, outside internal and testdata directories, hidden directories and nested modules.
func packageDirs(repo string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(repo, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(repo, p)
		if err != nil {
			return err
		}
		if rel != "." {
			switch name := d.Name(); {
			case name == "internal", name == "testdata", name == "vendor", strings.HasPrefix(name, "."), strings.HasPrefix(name, "_"):
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
				return filepath.SkipDir
			}
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if n := e.Name(); !e.IsDir() && strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
				if rel == "." {
					rel = ""
				}
				dirs = append(dirs, filepath.ToSlash(rel))
				break
			}
		}
		return nil
	})
	sort.Strings(dirs)
	return dirs, err
}
