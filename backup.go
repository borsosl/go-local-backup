// Package backup copies files to target directories based on plain text configuration.
package backup

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	destDirPerm fs.FileMode = 0770

	isWin             = runtime.GOOS == "windows"
	commentRex        = regexp.MustCompile(`^#`)
	targetRex         = regexp.MustCompile(`^=>\s*(.*)`)
	maxAgeRex         = regexp.MustCompile(`^!@\s*(.*)\s*$`)
	maxSizeRex        = regexp.MustCompile(`^!>\s*(.*)\s*$`)
	excludeRex        = regexp.MustCompile(`^!\s*(.*)`)
	extendExcludeRex  = regexp.MustCompile(`^!\+\s*(.*)`)
	printDotFileCount = 100
	maxErrors         = 100
)

// Data passed around during backup.
type backupContext struct {
	out        io.Writer
	dryRun     bool
	targetPath string
	startDate  time.Time
	maxSize    int64
	exclude    []*regexp.Regexp
	count      backupCounts
	msgs       []string
}

// Stats for a directory source.
type backupCounts struct {
	dir    int
	files  int
	copied int
}

// Invoke this function to perform a backup.
// config contains lines of configuration, out is a writer to send output
// messages to, and dryRun turns on dry-run mode.
//
// See details at https://github.com/borsosl/go-local-backup/README.md
func Backup(config []string, out io.Writer, dryRun bool) (ev error) {
	ev = nil
	ctx := backupContext{
		out:       out,
		dryRun:    dryRun,
		startDate: time.Time{},
		maxSize:   math.MaxInt64,
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(out, r)
		}
		if len(ctx.msgs) > 0 {
			fmt.Fprintf(out, "\n%d errors:\n", len(ctx.msgs))
			for _, s := range ctx.msgs {
				fmt.Fprintln(out, s)
			}
			ev = fmt.Errorf("%d errors", len(ctx.msgs))
		}
	}()

	for _, line := range config {
		line = strings.TrimSpace(line)

		if processNonPathLine(&ctx, line) {
			continue
		}

		if ctx.targetPath == "" {
			errText := "target path must be specified before any source paths"
			fmt.Fprintln(out, "FATAL:", errText)
			return errors.New(errText)
		}

		path := line
		info, err := stat(path)
		if err != nil {
			msg(&ctx, fmt.Sprint("Cannot stat, skipping: ", path))
			continue
		}
		fmt.Fprintf(out, "\n%s\n", path)

		if info.IsDir() {
			handleDir(&ctx, path)
		} else {
			handleFile(&ctx, path, info)
		}
	}

	return
}

// Processes configuration lines that are not source locations.
func processNonPathLine(ctx *backupContext, line string) bool {
	if line == "" || commentRex.MatchString(line) {
		return true
	}

	res := targetRex.FindSubmatch([]byte(line))
	if res != nil {
		ctx.targetPath = strings.TrimSuffix(string(res[1]), string(filepath.Separator))
		fmt.Fprintln(ctx.out, "target", ctx.targetPath)
		return true
	}

	res = maxAgeRex.FindSubmatch([]byte(line))
	if res != nil {
		max, err := strconv.Atoi(string(res[1]))
		if err != nil {
			fmt.Fprintf(ctx.out, "WARN: Expected only number in: %s\n", line)
		} else {
			sub := time.Duration(max) * 24 * time.Hour
			ctx.startDate = time.Now().Add(-sub)
			y, m, d := ctx.startDate.Date()
			ctx.startDate = time.Date(y, m, d, 0, 0, 0, 0, ctx.startDate.Location())
			fmt.Fprintln(ctx.out, "since", ctx.startDate)
		}
		return true
	}

	res = maxSizeRex.FindSubmatch([]byte(line))
	if res != nil {
		max, err := strconv.ParseInt(string(res[1]), 10, 64)
		if err != nil {
			fmt.Fprintf(ctx.out, "WARN: Expected only number in: %s\n", line)
		} else {
			ctx.maxSize = max
			fmt.Fprintln(ctx.out, "max size", ctx.maxSize)
		}
		return true
	}

	res = extendExcludeRex.FindSubmatch([]byte(line))
	if res != nil {
		parseExclude(ctx, string(res[1]), true)
		fmt.Fprintln(ctx.out, "extend exclude", string(res[1]))
		return true
	}

	res = excludeRex.FindSubmatch([]byte(line))
	if res != nil {
		parseExclude(ctx, string(res[1]), false)
		fmt.Fprintln(ctx.out, "exclude", string(res[1]))
		return true
	}

	return false
}

// Collects active regular expressions for exclusion.
func parseExclude(ctx *backupContext, arg string, extend bool) {
	if !extend {
		ctx.exclude = nil
	}
	arg = strings.TrimSpace(arg)
	parts := strings.Split(arg, ",,")
	for _, part := range parts {
		if part == "" {
			continue
		}
		rex, err := regexp.Compile(part)
		if err != nil {
			msg(ctx, fmt.Sprintf("Error in exlude regexp: %s", part))
		} else {
			ctx.exclude = append(ctx.exclude, rex)
		}
	}
}

// Recurses a source directory, filters excluded subdirectories.
func handleDir(ctx *backupContext, path string) {
	ctx.count = backupCounts{}
	walkCallback := func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			path += string(filepath.Separator)
			for _, rex := range ctx.exclude {
				if rex.MatchString(path) {
					return fs.SkipDir
				}
			}
			ctx.count.dir++
		} else {
			info, err := d.Info()
			if err == nil {
				handleFile(ctx, path, info)
			}
		}
		return nil
	}
	walkDir(path, walkCallback)
	if ctx.count.files >= printDotFileCount {
		fmt.Fprintln(ctx.out)
	}
	fmt.Fprintf(ctx.out, "Dirs: %d, Files: %d, Copied: %d\n",
		ctx.count.dir, ctx.count.files, ctx.count.copied)
}

// Copies a file if not filtered.
func handleFile(ctx *backupContext, srcPath string, srcInfo fs.FileInfo) {
	ctx.count.files++
	if !ctx.dryRun && ctx.count.files%printDotFileCount == 0 {
		fmt.Fprint(ctx.out, ".")
	}

	if srcInfo.Mode()&fs.ModeSymlink != 0 {
		return
	}

	if srcInfo.Size() > ctx.maxSize {
		return
	}

	if srcInfo.ModTime().Before(ctx.startDate) {
		return
	}

	for _, rex := range ctx.exclude {
		if rex.MatchString(srcPath) {
			return
		}
	}

	vol := filepath.VolumeName(srcPath)
	destPath := ctx.targetPath + srcPath[len(vol):]

	destInfo, err := stat(destPath)
	if err == nil {
		if !destInfo.ModTime().Before(srcInfo.ModTime()) {
			return
		}
		if isWin && destInfo.Mode()&0200 != 0 {
			chmod(destPath, destDirPerm)
		}
	} else if !ctx.dryRun {
		err := mkdirAll(filepath.Dir(destPath), destDirPerm)
		if err != nil {
			msg(ctx, fmt.Sprint("Cannot create dirs for: ", destPath))
			return
		}
	}

	if ctx.dryRun {
		fmt.Fprintln(ctx.out, srcPath)
		ctx.count.copied++
		return
	}

	err = copy(srcPath, destPath, srcInfo)
	if err != nil {
		msg(ctx, err.Error())
		return
	}

	ctx.count.copied++
}

// Adds to collected messages that are printed after sources are processed.
func msg(ctx *backupContext, msg string) {
	ctx.msgs = append(ctx.msgs, msg)
	if len(ctx.msgs) >= maxErrors {
		panic("Quitting due to too many errors!")
	}
}
