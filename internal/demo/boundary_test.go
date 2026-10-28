package demo

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Demo mode must never reach the user's real data or the network.
var demoBannedImports = []string{
	"net", "net/http", "os", "os/exec", "database/sql",
	"github.com/gorilla/websocket",
	"modernc.org/sqlite",
	"github.com/gammons/slk/internal/avatar",
	"github.com/gammons/slk/internal/bootstrap",
	"github.com/gammons/slk/internal/cache",
	"github.com/gammons/slk/internal/config",
	"github.com/gammons/slk/internal/editor",
	"github.com/gammons/slk/internal/export",
	"github.com/gammons/slk/internal/filedl",
	"github.com/gammons/slk/internal/notify",
	"github.com/gammons/slk/internal/service",
	"github.com/gammons/slk/internal/wake",
}

func demoImportBanned(path string) bool {
	// mrkdwn is pure text conversion, the same one internal/ui uses to
	// render its optimistic send.
	if path == "github.com/gammons/slk/internal/slack/mrkdwn" {
		return false
	}
	if strings.HasPrefix(path, "github.com/gammons/slk/internal/slack") {
		return true
	}
	return slices.Contains(demoBannedImports, path)
}

func TestDemoStaysOffTheNetworkAndDisk(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if demoImportBanned(path) {
				t.Errorf("%s imports %s; demo mode must not touch real data or the network", name, path)
			}
		}
	}
}

func TestDemoImportBanCoversSlackSubpackages(t *testing.T) {
	for path, want := range map[string]bool{
		"github.com/gammons/slk/internal/slack":        true,
		"github.com/gammons/slk/internal/slackhttp":    true,
		"github.com/gammons/slk/internal/slackdesktop": true,
		"github.com/gammons/slk/internal/slack/mrkdwn": false,
		"github.com/gammons/slk/internal/core":         false,
	} {
		if got := demoImportBanned(path); got != want {
			t.Errorf("demoImportBanned(%s) = %v, want %v", path, got, want)
		}
	}
}
