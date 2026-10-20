package pathfs

import (
	"errors"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestXAttrSyscallEmptyAndFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := ioutil.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	create, replace := 1, 2 // Linux XATTR_CREATE / XATTR_REPLACE.
	if runtime.GOOS == "darwin" {
		create, replace = 2, 4
	}
	const attr = "user.pathfs_test"
	if err := setXAttr(path, attr, nil, create); err != nil {
		if errors.Is(err, syscall.ENOTSUP) {
			t.Skipf("temporary filesystem lacks xattrs: %v", err)
		}
		t.Fatal(err)
	}
	if err := setXAttr(path, attr, []byte("duplicate"), create); !errors.Is(err, syscall.EEXIST) {
		t.Fatalf("create existing xattr: %v", err)
	}
	if got, err := getXAttr(path, attr); err != nil || len(got) != 0 {
		t.Fatalf("empty xattr: %q %v", got, err)
	}
	if err := setXAttr(path, attr, []byte("replacement"), replace); err != nil {
		t.Fatal(err)
	}
	if got, err := getXAttr(path, attr); err != nil || string(got) != "replacement" {
		t.Fatalf("replaced xattr: %q %v", got, err)
	}
	attrs, err := listXAttr(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range attrs {
		if name == attr {
			found = true
		}
	}
	if !found {
		t.Fatalf("attribute missing from list: %v", attrs)
	}
	if err := removeXAttr(path, attr); err != nil {
		t.Fatal(err)
	}
	if err := setXAttr(path, attr, nil, replace); err == nil {
		t.Fatal("replace missing xattr succeeded")
	}
}
