package pathfs

import (
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func utimes(path string, atime *time.Time, mtime *time.Time) error {
	var st syscall.Stat_t
	if atime == nil || mtime == nil {
		if err := syscall.Stat(path, &st); err != nil {
			return err
		}
	}
	return syscall.Utimes(path, timestampValues(atime, mtime, time.Unix(st.Atimespec.Sec, st.Atimespec.Nsec), time.Unix(st.Mtimespec.Sec, st.Mtimespec.Nsec)))
}

func fUtimes(fd int, atime *time.Time, mtime *time.Time) error {
	var st syscall.Stat_t
	if atime == nil || mtime == nil {
		if err := syscall.Fstat(fd, &st); err != nil {
			return err
		}
	}
	return syscall.Futimes(fd, timestampValues(atime, mtime, time.Unix(st.Atimespec.Sec, st.Atimespec.Nsec), time.Unix(st.Mtimespec.Sec, st.Mtimespec.Nsec)))
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

	var destPointer unsafe.Pointer
	if len(dest) > 0 {
		destPointer = unsafe.Pointer(&dest[0])
	}

	size, _, errNo := syscall.Syscall6(
		syscall.SYS_GETXATTR,
		uintptr(unsafe.Pointer(pathBs)),
		uintptr(unsafe.Pointer(attrBs)),
		uintptr(destPointer),
		uintptr(len(dest)),
		0, 0)

	if errNo != 0 {
		err = error(errNo)
	}
	return int(size), err
}

func listXAttrSyscall(path string, dest []byte) (sz int, err error) {
	pathbs, err := syscall.BytePtrFromString(path)
	if err != nil {
		return 0, err
	}

	var destPointer unsafe.Pointer
	var zero uintptr
	if len(dest) == 0 {
		destPointer = unsafe.Pointer(&zero)
	} else {
		destPointer = unsafe.Pointer(&dest[0])
	}

	size, _, errNo := syscall.Syscall6(
		syscall.SYS_LISTXATTR,
		uintptr(unsafe.Pointer(pathbs)),
		uintptr(destPointer),
		uintptr(len(dest)),
		0, 0, 0)

	if errNo != 0 {
		err = error(errNo)
	}
	return int(size), err
}

func setXAttr(path string, attr string, data []byte, flags int) error {
	pathbs, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	attrbs, err := syscall.BytePtrFromString(attr)
	if err != nil {
		return err
	}
	var dataPointer unsafe.Pointer
	if len(data) > 0 {
		dataPointer = unsafe.Pointer(&data[0])
	}
	_, _, errNo := syscall.Syscall6(
		syscall.SYS_SETXATTR,
		uintptr(unsafe.Pointer(pathbs)),
		uintptr(unsafe.Pointer(attrbs)),
		uintptr(dataPointer),
		uintptr(len(data)),
		0, uintptr(flags))

	if errNo != 0 {
		err = error(errNo)
	}
	return err
}

func removeXAttr(path string, attr string) error {
	pathbs, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	attrbs, err := syscall.BytePtrFromString(attr)
	if err != nil {
		return err
	}
	_, _, errNo := syscall.Syscall(
		syscall.SYS_REMOVEXATTR,
		uintptr(unsafe.Pointer(pathbs)),
		uintptr(unsafe.Pointer(attrbs)), 0)

	if errNo != 0 {
		err = error(errNo)
	}
	return err
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
		t.Errorf("want size %d, have %d", fileSize, st.Size)
	}
}
