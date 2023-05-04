package pathfs

import (
	"syscall"
	"testing"
	"time"
)

func utimes(path string, atime *time.Time, mtime *time.Time) error {
	timevals := []syscall.Timeval{
		{Sec: atime.Unix(), Usec: int64(atime.Nanosecond())},
		{Sec: mtime.Unix(), Usec: int64(mtime.Nanosecond())},
	}
	return syscall.Utimes(path, timevals)
}

func fUtimes(fd int, atime *time.Time, mtime *time.Time) error {
	timevals := []syscall.Timeval{
		{Sec: atime.Unix(), Usec: int64(atime.Nanosecond())},
		{Sec: mtime.Unix(), Usec: int64(mtime.Nanosecond())},
	}
	return syscall.Futimes(fd, timevals)
}

func setXAttr(path string, attr string, data []byte, flags int) error {
	return syscall.Setxattr(path, attr, data, flags)
}

func removeXAttr(path string, attr string) error {
	return syscall.Removexattr(path, attr)
}

func getXAttrSyscall(path string, attr string, dest []byte) (int, error) {
	return syscall.Getxattr(path, attr, dest)
}

func listXAttrSyscall(path string, dest []byte) (int, error) {
	return syscall.Listxattr(path, dest)
}

func verifyAttrTesting(t *testing.T, st *syscall.Stat_t, mode uint32, timeVal []syscall.Timeval, fileSize int64) {
	if st.Mode != mode {
		t.Errorf("want mode %o, have %o", mode, st.Mode)
	}
	if st.Atim.Sec != timeVal[0].Sec {
		t.Errorf("want atime %d, have %d", timeVal[0].Sec, st.Atim.Sec)
	}
	if st.Mtim.Sec != timeVal[1].Sec {
		t.Errorf("want mtime %d, have %d", timeVal[1].Sec, st.Mtim.Sec)
	}
	if st.Size != fileSize {
		t.Errorf("want size %d, have %d", fileSize, st.Size)
	}
}
