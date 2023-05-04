package pathfs

import (
	"fmt"
	"github.com/hanwen/go-fuse/v2/fuse"
	"io"
	"os"
	fp "path/filepath"
	"syscall"
	"testing"
	"time"
)

func setupTest() (mountPoint string, svr *fuse.Server) {
	mountPoint = "/tmp/test_mount"
	nativeRoot := "/tmp/test_native"
	// clear old file
	err := os.RemoveAll(mountPoint)
	if err != nil {
		panic(err)
	}
	err = os.RemoveAll(nativeRoot)
	if err != nil {
		panic(err)
	}
	// create test directory
	err = os.Mkdir(mountPoint, 0700)
	if err != nil {
		panic(err)
	}
	err = os.Mkdir(nativeRoot, 0700)
	if err != nil {
		panic(err)
	}

	server, err := Mount(mountPoint, NewTestFileSystem(nativeRoot), nil, nil)
	if err != nil {
		server.Unmount()
		panic(err)
	}
	return mountPoint, server
}

func assertError(err error, expectedErr error) {
	if err != expectedErr {
		msg := "nil"
		if expectedErr != nil {
			msg = expectedErr.Error()
		}
		panic(fmt.Sprintf("error should be %s, not %s", msg, err))
	}
}

func printDir(dir string) {
	entries, err := os.ReadDir(dir)
	for _, e := range entries {
		fmt.Println(e.Name())
	}
	assertError(err, nil)
	fmt.Println()
}

func umount(server *fuse.Server) {
	err := server.Unmount()
	if err != nil {
		fmt.Printf("unable to umount fs, err:%s\n", err)
	}
}

func Example_dir() {
	mountPoint, server := setupTest()
	defer umount(server)

	dirPath := fp.Join(mountPoint, "test_dir")

	dirPerm := uint32(0700)
	regPerm := uint32(0600)
	st := syscall.Stat_t{}

	err := syscall.Mkdir(dirPath, dirPerm)
	assertError(err, nil)
	err = syscall.Stat(dirPath, &st)
	assertError(err, nil)

	// create a file in an existing directory
	filePath := fp.Join(dirPath, "test_file")
	fd, err := syscall.Open(filePath, syscall.O_CREAT, regPerm)
	defer syscall.Close(fd)
	assertError(err, nil)
	err = syscall.Stat(filePath, &st)
	assertError(err, nil)

	// create a file in a non-existent directory
	fd, err = syscall.Open(fp.Join(mountPoint, "test_dir_1/test_file"), syscall.O_CREAT, regPerm)
	assertError(err, syscall.ENOENT)

	// create a sub directory
	subDirPath := fp.Join(dirPath, "sub_dir")
	err = syscall.Mkdir(subDirPath, dirPerm)
	assertError(err, nil)
	err = syscall.Stat(subDirPath, &st)
	assertError(err, nil)

	// move test_file to sub_dir
	subFilePath := fp.Join(subDirPath, "test_file")
	err = syscall.Rename(filePath, subFilePath)
	assertError(err, nil)
	err = syscall.Stat(filePath, &st)
	assertError(err, syscall.ENOENT)
	err = syscall.Stat(subFilePath, &st)
	assertError(err, nil)

	// link temp_file to root
	rootFilePath := fp.Join(mountPoint, "test_file")
	err = syscall.Link(subFilePath, rootFilePath)
	assertError(err, nil)
	printDir(mountPoint)

	// unlink temp_file from sub_dir
	err = syscall.Unlink(subFilePath)
	assertError(err, nil)
	err = syscall.Stat(subFilePath, &st)
	assertError(err, syscall.ENOENT)
	printDir(mountPoint)

	// unlink temp_file from root
	err = syscall.Unlink(rootFilePath)
	assertError(err, nil)
	err = syscall.Stat(rootFilePath, &st)
	assertError(err, syscall.ENOENT)
	printDir(mountPoint)

	// clear test files
	err = syscall.Rmdir(subDirPath)
	assertError(err, nil)
	err = syscall.Rmdir(dirPath)
	assertError(err, nil)

	// Output:
	// test_dir
	// test_file
	//
	// test_dir
	// test_file
	//
	// test_dir

}

func Example_io() {
	mountPoint, server := setupTest()
	defer umount(server)
	testFilePath := fp.Join(mountPoint, "test_file")

	testContent := []byte("test_content")

	// create a file and write some content
	file, err := os.Create(testFilePath)
	defer file.Close()
	assertError(err, nil)
	_, err = file.Write(testContent)
	assertError(err, nil)

	// reopen the file and verify the content
	file, err = os.Open(testFilePath)
	defer file.Close()
	assertError(err, nil)
	content, err := io.ReadAll(file)
	assertError(err, nil)
	fmt.Println(string(content))

	// Output:
	// test_content

}

func TestAttr(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	st := syscall.Stat_t{}
	path := fp.Join(mountPoint, "test_file")

	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_TRUNC, 0700)
	err = syscall.Close(fd)

	assertError(err, nil)
	err = syscall.Stat(path, &st)
	assertError(err, nil)

	tm := time.Now()
	timeVal := []syscall.Timeval{
		{Sec: tm.Unix() + 10},
		{Sec: tm.Unix() + 10},
	}
	mode := uint32(0777 | syscall.S_IFREG)
	fileSize := int64(16)

	err = syscall.Chmod(path, mode)
	assertError(err, nil)

	err = syscall.Truncate(path, fileSize)
	assertError(err, nil)

	err = syscall.Utimes(path, timeVal)
	assertError(err, nil)

	err = syscall.Stat(path, &st)
	assertError(err, nil)
	verifyAttrTesting(t, &st, mode, timeVal, fileSize)

	// test xattr
	testAttrData := "testattr_data"
	testAttr := "testattr"
	err = setXAttr(path, testAttr, []byte(testAttrData), 0)
	assertError(err, nil)

	attr, err := getXAttr(path, "testattr")
	assertError(err, nil)
	if string(attr) != testAttrData {
		t.Errorf("want xattr %s, have %s", testAttrData, string(attr))
	}

	attrs, err := listXAttr(path)
	assertError(err, nil)
	if len(attrs) != 1 {
		t.Errorf("want xattr count %d, have %d", 1, len(attrs))
	}
	if attrs[0] != testAttr {
		t.Errorf("want xattr %s, have %s", testAttr, attrs[0])
	}

	err = removeXAttr(path, testAttr)
	assertError(err, nil)
	attrs, err = listXAttr(path)
	if len(attrs) != 0 {
		t.Errorf("want xattr count %d, have %d", 0, len(attrs))
	}

}
