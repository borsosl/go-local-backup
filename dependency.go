package backup

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

var (
	stat     = statImpl
	walkDir  = walkDirImpl
	chmod    = chmodImpl
	mkdirAll = mkdirAllImpl
	copy     = copyImpl

	isTest = false
)

func statImpl(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

func walkDirImpl(root string, callback fs.WalkDirFunc) {
	filepath.WalkDir(root, callback)
}

func chmodImpl(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func mkdirAllImpl(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func copyImpl(src, dest string, srcInfo fs.FileInfo) error {
	srcReader, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("cannot open %q for reading: %w", src, err)
	}
	defer srcReader.Close()

	destWriter, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("cannot open %q for writing: %w", dest, err)
	}
	defer destWriter.Close()

	if _, err := io.Copy(destWriter, srcReader); err != nil {
		return fmt.Errorf("error copying content of %q: %w", src, err)
	}

	os.Chmod(dest, 0660)
	os.Chtimes(dest, time.Now(), srcInfo.ModTime())
	return nil
}
