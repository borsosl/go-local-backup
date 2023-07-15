package backup

import (
	"io/fs"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel        *windows.LazyDLL
	procCopyFileW *windows.LazyProc
)

func init() {
	if !isTest {
		kernel = windows.NewLazySystemDLL("kernel32")
		procCopyFileW = kernel.NewProc("CopyFileW")
		copy = copyWinImpl
		destDirPerm = 0777
	}
}

func copyWinImpl(src, dest string, srcInfo fs.FileInfo) error {
	lpcwstrSrc, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return err
	}

	lpcwstrDest, err := windows.UTF16PtrFromString(dest)
	if err != nil {
		return err
	}

	r1, _, err := procCopyFileW.Call(
		uintptr(unsafe.Pointer(lpcwstrSrc)),
		uintptr(unsafe.Pointer(lpcwstrDest)),
		uintptr(uint32(0)),
	)

	if r1 == 0 {
		return err
	}
	return nil
}
