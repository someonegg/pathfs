package pathfs

import (
	"fmt"
	"github.com/hanwen/go-fuse/v2/fuse"
	"os"
	fp "path/filepath"
	"syscall"
)

func setupTest() (mountPoint string, svr *fuse.Server) {
	const (
		MountPoint = "/mnt"
		NativeRoot = "/home/"
	)
	server, err := Mount(MountPoint, NewLoopbackFileSystem(NativeRoot), nil, nil)
	if err != nil {
		panic(err)
	}
	return MountPoint, server
}

func assertError(err error, actualErr error) {
	if err != actualErr {
		panic(fmt.Errorf("error should be %s\n, not %s", actualErr, err))
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

func Example_testDir() {

	// 测试 mknod， mkdir， unlink，rmdir，rename，link, lsDir
	// 在测试的同时验证树结构的正确
	mountPoint, server := setupTest()

	testDirPath := fp.Join(mountPoint, "test_dir")

	defer func() {
		// remove testing directory
		err := syscall.Rmdir(testDirPath)
		if err != nil {
			fmt.Printf("unable to rm test directory, err:%s\n", err)
		}
		st := syscall.Stat_t{}
		err = syscall.Stat(testDirPath, &st)
		if err != syscall.ENOENT {
			fmt.Printf("unable to rm test directory.\n")
		}
		err = server.Unmount()
		if err != nil {
			fmt.Printf("unable to umount fs, err:%s\n", err)
		}
	}()

	dirPerm := uint32(0700)
	regPerm := uint32(0600)
	st := syscall.Stat_t{}

	err := syscall.Mkdir(testDirPath, dirPerm)
	assertError(err, nil)
	err = syscall.Stat(testDirPath, &st)
	assertError(err, nil)

	testFilePath := fp.Join(testDirPath, "test_file")
	// create a file in an existing directory
	err = syscall.Mknod(testFilePath, regPerm|fuse.S_IFREG, 0)
	assertError(err, nil)
	err = syscall.Stat(testFilePath, &st)
	assertError(err, nil)

	// create a file in a non-existent directory
	err = syscall.Mknod(fp.Join(mountPoint, "test_dir_1/test_file"), regPerm|fuse.S_IFREG, 0)
	assertError(err, syscall.ENOENT)

	// create a duplicate file
	err = syscall.Mknod(testFilePath, dirPerm|fuse.S_IFREG, 0)
	assertError(err, syscall.EEXIST)

	// create a sub directory
	testSubDirPath := fp.Join(testDirPath, "sub_dir")
	err = syscall.Mkdir(testSubDirPath, dirPerm)
	assertError(err, nil)
	err = syscall.Stat(testSubDirPath, &st)
	assertError(err, nil)

	// move test_file to sub_dir
	err = syscall.Rename(testFilePath, testSubDirPath)
	err = syscall.Stat(testFilePath, &st)
	assertError(err, syscall.ENOENT)
	err = syscall.Stat(fp.Join(testSubDirPath, "test_file"), &st)
	assertError(err, nil)

	err = syscall.Mknod(testFilePath, dirPerm|fuse.S_IFREG, 0)
	assertError(err, syscall.EEXIST)

	// link sub_dir to root
	err = syscall.Link(testSubDirPath, mountPoint)
	assertError(err, nil)
	printDir(mountPoint)

	// unlink sub_dir from initial directory
	err = syscall.Unlink(testSubDirPath)
	assertError(err, nil)
	err = syscall.Stat(testSubDirPath, &st)
	assertError(err, syscall.ENOENT)
	printDir(mountPoint)

	// Output:
	// test_dir
	// sub_dir
	//
	// test_dir
	// sub_dir

}

func Example_testIO() {
	// 测试 create open write fallocate fsync flush release
}

func Example_testAttr() {
	// 测试 chmod chown getattr setattr
}
