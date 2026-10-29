package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mayoi/locales"
)

// settleNested waits for the window's work trees to be found and read: those
// in the folders under its own when it is none, those inside it otherwise.
func settleNested(t *testing.T, hs *harness, count int) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		hs.settle()
		ready := !hs.a.repositories.scanning
		found := 0
		for _, ctx := range hs.a.repositories.contexts {
			g := ctx.git
			if g.repo == nil {
				continue // the folder itself, when it is not a work tree
			}
			found++
			ready = ready && !g.running && !g.stale && g.pending == 0 && g.busy == "" && g.status != nil
		}
		if ready && found == count {
			return
		}
	}
	t.Fatalf("work trees not found: want %d, got %+v", count, hs.a.repositories.contexts)
}

// A folder of projects, each its own work tree, lists them all in Source
// Control and commits in each (#17).
func TestNestedRepositories(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	hs := newHarness(t, true)
	a := hs.a
	var roots []string
	for _, name := range []string{"project-a", "project-b"} {
		root := hs.path(name)
		os.MkdirAll(root, 0o755)
		gitIn(t, root, "init", "-q", "-b", "main")
		gitIn(t, root, "config", "user.name", "t")
		gitIn(t, root, "config", "user.email", "t@t")
		os.WriteFile(filepath.Join(root, "f.txt"), []byte("one\n"), 0o644)
		gitIn(t, root, "add", ".")
		gitIn(t, root, "commit", "-q", "-m", "first")
		roots = append(roots, root)
	}
	a.repositories.dirty = true
	a.showSourceControl()
	settleNested(t, hs, 2)
	if a.git.repo != nil {
		t.Fatalf("the folder is a work tree: %v", a.git.repo.Root)
	}
	file := filepath.Join(roots[1], "f.txt")
	os.WriteFile(file, []byte("two\n"), 0o644)
	a.gitChanged()
	settleNested(t, hs, 2)
	b := a.repositoryForPath(file)
	if b.git.repo == nil || b.git.repo.Root != roots[1] || b.changeCount() != 1 {
		t.Fatalf("project-b: %+v", b.git.status)
	}
	if a.repository().git.repo == nil {
		t.Fatal("no repository for window commands")
	}
	if hs.tt.HasText(locales.ScmNoRepo()) || !hs.tt.HasText("project-a") || !hs.tt.HasText("project-b") {
		t.Fatalf("source control: %q", hs.tt.Texts())
	}
	hs.shot("nested-repositories")
	b.git.message = "in b"
	b.stage(file)
	settleNested(t, hs, 2)
	b.commit(false)
	settleNested(t, hs, 2)
	if got := strings.TrimSpace(gitIn(t, roots[1], "log", "-1", "--format=%s")); got != "in b" {
		t.Fatalf("project-b log: %q", got)
	}
	if got := strings.TrimSpace(gitIn(t, roots[0], "log", "-1", "--format=%s")); got != "first" {
		t.Fatalf("project-a log: %q", got)
	}
	// Saving a file in a project does not look for work trees again.
	a.diskChanged([]string{file}, nil)
	if a.repositories.dirty {
		t.Fatal("a file saved in a project rescans")
	}
	// A project cloned later is found: the watcher reports its folder,
	// whose .git it does not watch yet.
	c := hs.path("project-c")
	os.MkdirAll(c, 0o755)
	gitIn(t, c, "init", "-q")
	a.diskChanged([]string{c}, nil)
	if !a.repositories.dirty {
		t.Fatal("a project cloned does not rescan")
	}
	settleNested(t, hs, 3)
	// One removed is dropped.
	os.RemoveAll(c)
	a.diskChanged(nil, []string{c})
	if !a.repositories.dirty {
		t.Fatal("a project removed does not rescan")
	}
	settleNested(t, hs, 2)
}

// A work tree that holds work trees of their own lists them as it does its
// own changes, though Git counts each of them as one untracked folder.
func TestNestedRepositoriesInWorkTree(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	hs := newHarness(t, true)
	a := hs.a
	// The window's folder is a work tree, with a change of its own.
	gitIn(t, hs.root, "init", "-q", "-b", "main")
	gitIn(t, hs.root, "add", ".")
	gitIn(t, hs.root, "commit", "-q", "-m", "first")
	os.WriteFile(hs.path("README.md"), []byte("# Mayoi\n\nA small editor, changed.\n"), 0o644)
	var roots []string
	for _, name := range []string{"project-a", "project-b"} {
		root := hs.path(name)
		os.MkdirAll(root, 0o755)
		gitIn(t, root, "init", "-q", "-b", "main")
		gitIn(t, root, "config", "user.name", "t")
		gitIn(t, root, "config", "user.email", "t@t")
		os.WriteFile(filepath.Join(root, "f.txt"), []byte("one\n"), 0o644)
		gitIn(t, root, "add", ".")
		gitIn(t, root, "commit", "-q", "-m", "first")
		roots = append(roots, root)
	}
	a.openFolder(hs.root)
	a.showSourceControl()
	settleNested(t, hs, 3)
	if a.git.repo == nil || a.git.repo.Root != hs.root {
		t.Fatalf("the folder's own work tree: %+v", a.git.repo)
	}
	// The changes of the work tree above count each one under it as one
	// untracked folder, as Git reports them.
	above := a.rootRepository()
	if above.changeCount() != 3 {
		t.Fatalf("the work tree above: %+v", above.git.status.Entries)
	}
	file := filepath.Join(roots[1], "f.txt")
	os.WriteFile(file, []byte("two\n"), 0o644)
	a.gitChanged()
	settleNested(t, hs, 3)
	b := a.repositoryForPath(file)
	if b.git.repo == nil || b.git.repo.Root != roots[1] || b.changeCount() != 1 {
		t.Fatalf("project-b: %+v", b.git.status)
	}
	if !hs.tt.HasText("project-a") || !hs.tt.HasText("project-b") {
		t.Fatalf("source control: %q", hs.tt.Texts())
	}
	hs.shot("nested-repositories-in-work-tree")
	// Each work tree takes its own commit, the one above none.
	b.git.message = "in b"
	b.stage(file)
	settleNested(t, hs, 3)
	b.commit(false)
	settleNested(t, hs, 3)
	if got := strings.TrimSpace(gitIn(t, roots[1], "log", "-1", "--format=%s")); got != "in b" {
		t.Fatalf("project-b log: %q", got)
	}
	if got := strings.TrimSpace(gitIn(t, hs.root, "log", "-1", "--format=%s")); got != "first" {
		t.Fatalf("the work tree above committed: %q", got)
	}
	// Saving a file looks for work trees only in a folder, never for a file.
	a.repositories.dirty = false
	a.diskChanged([]string{file}, nil)
	a.diskChanged([]string{hs.path("README.md")}, nil)
	if a.repositories.dirty {
		t.Fatal("a file saved rescans")
	}
	// One cloned inside it later is found, and one removed dropped.
	c := hs.path("project-c")
	os.MkdirAll(c, 0o755)
	gitIn(t, c, "init", "-q")
	a.diskChanged([]string{c}, nil)
	if !a.repositories.dirty {
		t.Fatal("a work tree cloned inside does not rescan")
	}
	settleNested(t, hs, 4)
	os.RemoveAll(c)
	a.diskChanged(nil, []string{c})
	settleNested(t, hs, 3)
}

// A folder with no work tree in or under it still offers to make one.
func TestNoNestedRepositories(t *testing.T) {
	hs := newHarness(t, true)
	hs.a.showSourceControl()
	hs.settle()
	if !hs.tt.HasText(locales.ScmNoRepo()) {
		t.Fatalf("source control: %q", hs.tt.Texts())
	}
}
