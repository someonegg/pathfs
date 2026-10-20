package pathfs

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func TestNativeBackendIOErrors(t *testing.T) {
	root := t.TempDir()
	fs := NewTestFileSystem(root)
	if err := ioutil.WriteFile(filepath.Join(root, "file"), []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	if n, code := fs.Write(nil, "file", 0, []byte("after!"), 0); code != fuse.OK || n != 6 {
		t.Errorf("write without handle: %d %v", n, code)
	}
	content, err := ioutil.ReadFile(filepath.Join(root, "file"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "after!" {
		t.Errorf("content = %q", content)
	}
	if _, code := fs.Lsdir(nil, "file"); code == fuse.OK {
		t.Error("listing a regular file succeeded")
	}
	// Pread on a directory fails and must return a status, not panic slicing dest[:-1].
	fd, err := syscall.Open(root, syscall.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	if _, code := fs.Read(nil, "", uint32(fd), make([]byte, 8), 0); code == fuse.OK {
		t.Error("read directory succeeded")
	}
	if n, code := fs.Write(nil, "", uint32(fd), []byte("x"), 0); code == fuse.OK || n != 0 {
		t.Errorf("failed write: %d %v", n, code)
	}
}

func TestNativeBackendAttrAfterUnlink(t *testing.T) {
	root := t.TempDir()
	fs := NewTestFileSystem(root)
	fh, _, code := fs.Create(nil, "file", syscall.O_RDWR, 0600)
	if code != fuse.OK {
		t.Fatal(code)
	}
	defer fs.Release(nil, "file", fh)
	if code := fs.Unlink(nil, "file"); code != fuse.OK {
		t.Fatal(code)
	}
	var attr fuse.Attr
	if code := fs.GetAttr(nil, "file", fh, &attr); code != fuse.OK {
		t.Fatalf("fstat unlinked handle: %v", code)
	}
}

func TestNativeBackendTimes(t *testing.T) {
	for _, handle := range []bool{false, true} {
		t.Run(map[bool]string{false: "path", true: "handle"}[handle], func(t *testing.T) {
			root := t.TempDir()
			fs := NewTestFileSystem(root)
			fh, _, code := fs.Create(nil, "file", syscall.O_RDWR, 0600)
			if code != fuse.OK {
				t.Fatal(code)
			}
			defer fs.Release(nil, "file", fh)
			useFH := uint32(0)
			if handle {
				useFH = fh
			}
			atime := time.Unix(1700000000, 123456000)
			mtime := time.Unix(1700000100, 654321000)
			if code := fs.Utimens(nil, "file", useFH, &atime, &mtime); code != fuse.OK {
				t.Fatal(code)
			}
			var before, after fuse.Attr
			if code := fs.GetAttr(nil, "file", fh, &before); code != fuse.OK {
				t.Fatal(code)
			}
			if before.Atime != uint64(atime.Unix()) || before.Mtime != uint64(mtime.Unix()) || before.Atimensec != 123456000 || before.Mtimensec != 654321000 {
				t.Fatalf("incorrect subsecond timestamps: %+v", before)
			}
			next := mtime.Add(time.Hour)
			if code := fs.Utimens(nil, "file", useFH, nil, &next); code != fuse.OK {
				t.Fatal(code)
			}
			if code := fs.GetAttr(nil, "file", fh, &after); code != fuse.OK {
				t.Fatal(code)
			}
			if after.Atime != before.Atime || after.Atimensec != before.Atimensec || after.Mtime != uint64(next.Unix()) {
				t.Errorf("partial timestamp update: %+v", after)
			}
			if code := fs.Utimens(nil, "file", useFH, &atime, nil); code != fuse.OK {
				t.Fatal(code)
			}
			if code := fs.GetAttr(nil, "file", fh, &after); code != fuse.OK {
				t.Fatal(code)
			}
			if after.Mtime != uint64(next.Unix()) || after.Mtimensec != uint32(next.Nanosecond()) {
				t.Errorf("omitted mtime changed: %+v", after)
			}
		})
	}
}

func TestNativeBackendXAttrIsolation(t *testing.T) {
	fs := NewTestFileSystem(t.TempDir())
	input := []byte("value")
	fs.SetXAttr(nil, "file", "attr", input, 0)
	input[0] = 'X'
	got, code := fs.GetXAttr(nil, "file", "attr")
	if code != fuse.OK || !bytes.Equal(got, []byte("value")) {
		t.Fatalf("input aliases stored value: %q %v", got, code)
	}
	got[0] = 'Y'
	got, _ = fs.GetXAttr(nil, "file", "attr")
	if string(got) != "value" {
		t.Fatal("output aliases stored value")
	}
	if code := fs.RemoveXAttr(nil, "file", "missing"); code != fuse.ENODATA {
		t.Errorf("missing attribute: %v", code)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				fs.SetXAttr(nil, "file", "attr", []byte("value"), 0)
				fs.GetXAttr(nil, "file", "attr")
				fs.ListXAttr(nil, "file")
				fs.RemoveXAttr(nil, "file", "attr")
			}
		}()
	}
	wg.Wait()
}
