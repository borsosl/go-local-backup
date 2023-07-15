package backup

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

type fileAttr struct {
	path   string
	age    int
	size   int64
	copied bool
	tcase  string
}

var pathsNoErrors = []*fileAttr{
	{"/d1/d2/f1", 10, 100, true, "included, dest readonly"},
	{"/d1/d2/s1", 10, 0, false, "symlink"},
	{"/d3/d4/f1", 10, 100, false, "not changed"},
	{"/d3/d4/f2", 10, 200, false, "large"},
	{"/d3/d4/f3", 30, 100, false, "old"},
	{"/d3/d4/f4", 30, 200, false, "old and large"},
	{"/d5/f1", 10, 100, true, "included"},
	{"/d5/f5", 10, 100, false, "excluded file"},
	{"/d5/d6/f1", 10, 100, false, "excluded dir"},
	{"/d7/d_has_middle_part/f1", 10, 100, false, "excluded by subpattern"},
	{"/d8/f1", 10, 100, true, "included standalone file"},
	{"/d9/f1", 10, 100, false, "dir not in sources"},
	{"/f1", 10, 100, false, "file not in sources"},
}

var pathsErrors = []*fileAttr{
	{"/nostat", 10, 100, false, "cannot stat source"},
	{"/er1/f1", 10, 100, false, "copy fails"},
	{"/er1/d1/f2", 10, 100, false, "mkdirs fails when target file missing"},
}

var walkedDirs = map[string][]string{
	"/d1":    {"/d1/d2", "/d1/d2/f1", "/d1/d2/s1"},
	"/d3/d4": {"/d3/d4/f1", "/d3/d4/f2", "/d3/d4/f3", "/d3/d4/f4"},
	"/d5":    {"/d5/f1", "/d5/f5", "/d5/d6", "/d5/d6/f1"},
	"/d7":    {"/d7/d_has_middle_part", "/d7/d_has_middle_part/f1"},
	"/er1":   {"/er1/f1", "/er1/d1/f2"},
}

var configNoErrors = `
=> /backup

!@20
!>150
!f5,,/d6/
!+/[^/]*middle[^/]*/

/d1
/d3/d4
/d5
/d7
/d8/f1
`

var configErrors = `
=> /backup
!@100 days
!>10MB
# bad and empty regexp
!(notclosed,,
/nostat
/er1
`

var configNoTarget = `
/any_source
`

var testStart = func() time.Time {
	// override isTest as part of var initialization, before init()
	isTest = true
	return time.Now()
}()

var attrMap map[string]*fileAttr
var copiedFiles map[string]bool

// fileAttr implements fs.FileInfo and fs.DirEntry
func (fa *fileAttr) Name() string { return fa.path }
func (fa *fileAttr) Size() int64  { return fa.size }
func (fa *fileAttr) IsDir() bool  { return fa.size == -1 }
func (fa *fileAttr) Sys() any     { return nil }
func (fa *fileAttr) ModTime() time.Time {
	return testStart.Add(-time.Duration(fa.age*24) * time.Hour)
}
func (fa *fileAttr) Mode() fs.FileMode {
	if fa.path == "/backup/d1/d2/f1" {
		return 0200
	}
	if strings.HasSuffix(fa.path, "/s1") {
		return fs.ModeSymlink
	}
	return 0
}
func (fa *fileAttr) Type() fs.FileMode          { return fa.Mode() }
func (fa *fileAttr) Info() (fs.FileInfo, error) { return fa, nil }

func TestBackup_NoErrors(t *testing.T) {
	printDotFileCount = 2
	_, err := testBackup(t, pathsNoErrors, configNoErrors, false)
	if err != nil {
		t.Errorf("Expected no errors")
	}
	printDotFileCount = 100
}

func TestBackup_Errors(t *testing.T) {
	_, err := testBackup(t, pathsErrors, configErrors, false)
	if err == nil {
		t.Errorf("Expected errors")
	}
	if err.Error() != "4 errors" {
		t.Errorf("Expected 4 errors, but got %s", err.Error())
	}
}

func TestBackup_TooManyErrors(t *testing.T) {
	maxErrors = 2
	_, err := testBackup(t, pathsErrors, configErrors, false)
	if err == nil {
		t.Errorf("Expected errors")
	}
	if err.Error() != "2 errors" {
		t.Errorf("Expected 2 errors, but got %s", err.Error())
	}
	maxErrors = 100
}

func TestBackup_NoTarget(t *testing.T) {
	_, err := testBackup(t, nil, configNoTarget, false)
	if err == nil {
		t.Errorf("Expected error")
	}
	if !strings.HasPrefix(err.Error(), "target path must be specified") {
		t.Errorf("expected target not specified error")
	}
}

func TestBackup_DryRun(t *testing.T) {
	buf, err := testBackup(t, pathsNoErrors, configNoErrors, true)
	if err != nil {
		t.Errorf("Expected no errors")
	}
	if !regexp.MustCompile("Copied: 1").Match(buf.Bytes()) {
		t.Errorf("Expected dry-run to show number of would-be copied files")
	}
}

func testBackup(t *testing.T, paths []*fileAttr, config string, dryRun bool) (*bytes.Buffer, error) {
	if isWin {
		config = rewritePaths(config)
	}
	mapAttributes(paths)
	mockDependencies()
	copiedFiles = map[string]bool{}

	var out bytes.Buffer
	conf := strings.Split(config, "\n")
	err := Backup(conf, &out, dryRun)

	if dryRun {
		if len(copiedFiles) != 0 {
			t.Errorf("Expected no copies on dry run, but %d was copied", len(copiedFiles))
		}
	} else {
		for _, fa := range paths {
			if fa.copied && !copiedFiles[fa.path] {
				t.Errorf("%s: Expected %s to be copied, but it wasn't", fa.tcase, fa.path)
			} else if !fa.copied && copiedFiles[fa.path] {
				t.Errorf("%s: Expected %s not to be copied, but it was", fa.tcase, fa.path)
			}
		}
	}
	return &out, err
}

func rewritePaths(config string) string {
	configLines := strings.Split(config, "\n")
	rex := regexp.MustCompile(`( |^)/`)
	for i, cl := range configLines {
		if len(cl) == 0 {
			continue
		}
		configLines[i] = rex.ReplaceAllString(cl, "${1}c:\\")
		if configLines[i][0] == '!' {
			configLines[i] = strings.ReplaceAll(configLines[i], "/", "\\\\")
		}
	}
	return filepath.FromSlash(strings.Join(configLines, "\n"))
}

func mapAttributes(paths []*fileAttr) {
	attrMap = make(map[string]*fileAttr, len(paths))
	for _, e := range paths {
		attrMap[e.path] = e
	}
}

func mockDependencies() {
	stat = statTestImpl
	walkDir = walkDirTestImpl
	chmod = func(name string, mode fs.FileMode) error { return nil }
	mkdirAll = mkdirAllTestImpl
	copy = copyTestImpl
}

func statTestImpl(name string) (fs.FileInfo, error) {
	name = slashify(name)
	if walkedDirs[name] != nil {
		return &fileAttr{size: -1}, nil
	}
	if strings.HasPrefix(name, "/backup") {
		srcAttr := attrMap[name[7:]]
		fa := *srcAttr
		fa.path = name
		if strings.HasSuffix(name, "/er1/d1/f2") {
			return nil, errors.New("target does not exist")
		} else if !strings.HasSuffix(name, "/d3/d4/f1") {
			fa.age++
		}
		return &fa, nil
	}
	if name == "/nostat" {
		return nil, errors.New("cannot stat source file")
	}
	return attrMap[name], nil
}

func walkDirTestImpl(root string, callback fs.WalkDirFunc) {
	path := slashify(root)
	callback(root, &fileAttr{path, 0, -1, false, ""}, nil)
	for _, path := range walkedDirs[path] {
		if attrMap[path] != nil {
			callback(windowsify(path), attrMap[path], nil)
		} else {
			callback(windowsify(path), &fileAttr{path, 0, -1, false, ""}, nil)
		}
	}
}

func mkdirAllTestImpl(path string, perm fs.FileMode) error {
	path = slashify(path)
	if path == "/backup/er1/d1" {
		return errors.New("mkdir failed")
	}
	return nil
}

func copyTestImpl(src, dest string, srcInfo fs.FileInfo) error {
	src = slashify(src)
	if src == "/er1/f1" {
		return errors.New("cannot copy")
	}
	copiedFiles[src] = true
	return nil
}

func slashify(path string) string {
	if !isWin {
		return path
	}
	vol := filepath.VolumeName(path)
	return strings.ReplaceAll(path[len(vol):], "\\", "/")
}

func windowsify(path string) string {
	if !isWin {
		return path
	}
	return "c:" + filepath.FromSlash(path)
}
