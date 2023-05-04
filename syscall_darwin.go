package pathfs

import (
	"bytes"
	"errors"
	"github.com/hanwen/go-fuse/v2/fuse"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func utimes(path string, atime *time.Time, mtime *time.Time) error {
	timevals := []syscall.Timeval{
		{Sec: atime.Unix(), Usec: int32(atime.Nanosecond())},
		{Sec: mtime.Unix(), Usec: int32(mtime.Nanosecond())},
	}
	return syscall.Utimes(path, timevals)
}

func fUtimes(fd int, atime *time.Time, mtime *time.Time) error {
	timevals := []syscall.Timeval{
		{Sec: atime.Unix(), Usec: int32(atime.Nanosecond())},
		{Sec: mtime.Unix(), Usec: int32(mtime.Nanosecond())},
	}
	return syscall.Futimes(fd, timevals)
}

func getXAttrSyscall(path string, attr string, dest []byte) (sz int, err error) {
	pathBs, err := syscall.BytePtrFromString(path)
	if err != nil {
		return 0, err
	}
	attrBs, err := syscall.BytePtrFromString(attr)
	if err != nil {
		return 0, err
	}
	size, _, errNo := syscall.Syscall6(
		syscall.SYS_GETXATTR,
		uintptr(unsafe.Pointer(pathBs)),
		uintptr(unsafe.Pointer(attrBs)),
		uintptr(unsafe.Pointer(&dest[0])),
		uintptr(len(dest)),
		0, 0)
	return int(size), errors.New(errNo.Error())
}


func listXAttrSyscall(path string, dest []byte) (sz int, err error) {
	pathbs, err := syscall.BytePtrFromString(path)
	if err != nil {
		return 0, err
	}
	var destPointer unsafe.Pointer
	if len(dest) > 0 {
		destPointer = unsafe.Pointer(&dest[0])
	}
	size, _, errNo := syscall.Syscall(
		syscall.SYS_LISTXATTR,
		uintptr(unsafe.Pointer(pathbs)),
		uintptr(destPointer),
		uintptr(len(dest)))

	return int(size), errors.New(errNo.Error())
}

func setXAttr(path string, attr string, data []byte, flags int) error {
	pathbs, err := syscall.BytePtrFromString(path)
	if err != nil {
		return  err
	}
	attrbs, err := syscall.BytePtrFromString(attr)
	if err != nil {
		return  err
	}
	_, _, errNo := syscall.Syscall6(
		syscall.SYS_SETXATTR,
		uintptr(unsafe.Pointer(pathbs)),
		uintptr(unsafe.Pointer(attrbs)),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(flags), 0)

	return errors.New(errNo.Error())
}

func removeXAttr(path string, attr string) error {
	pathbs, err := syscall.BytePtrFromString(path)
	if err != nil {
		return  err
	}
	attrbs, err := syscall.BytePtrFromString(attr)
	if err != nil {
		return  err
	}
	_, _, errNo := syscall.Syscall(
		syscall.SYS_REMOVEXATTR,
		uintptr(unsafe.Pointer(pathbs)),
		uintptr(unsafe.Pointer(attrbs)), 0)
	return errors.New(errNo.Error())
}

func verifyAttrTesting(t *testing.T, st *syscall.Stat_t, mode uint32, timeVal []syscall.Timeval, fileSize int64) {
	if st.Mode != uint16(mode) {
		t.Errorf("want mode %o, have %o", mode, st.Mode)
	}
	if st.Atimespec.Sec != timeVal[0].Sec {
		t.Errorf("want atime %d, have %d", timeVal[0].Sec, st.Atimespec.Sec)
	}
	if st.Mtimespec.Sec != timeVal[1].Sec {
		t.Errorf("want mtime %d, have %d", timeVal[1].Sec, st.Mtimespec.Sec)
	}
	if st.Size != fileSize {
		t.Errorf(6"want size %d, have %d", fileSize, st.Size)
	}
}
