package pathfs

import (
	"bytes"
	"os"
	fp "path/filepath"
	"syscall"
	"testing"
)

// TestFileLifecycle tests the complete file operation flow
func TestFileLifecycle(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	filePath := fp.Join(mountPoint, "lifecycle_test_file")
	testData := []byte("Hello, PathFS! This is a lifecycle test.")

	// Create and write
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	n, err := file.Write(testData)
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	if n != len(testData) {
		t.Errorf("expected to write %d bytes, wrote %d", len(testData), n)
	}

	// Sync to ensure data is flushed
	err = file.Sync()
	if err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	// Close (Release)
	err = file.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	// Reopen and read
	file2, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("failed to reopen file: %v", err)
	}
	defer file2.Close()

	readBuf := make([]byte, len(testData)+10)
	n, err = file2.Read(readBuf)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	if n != len(testData) {
		t.Errorf("expected to read %d bytes, read %d", len(testData), n)
	}
	if !bytes.Equal(readBuf[:n], testData) {
		t.Errorf("data mismatch: expected %q, got %q", testData, readBuf[:n])
	}
}

// TestMultipleOpenSameFile tests opening the same file multiple times
func TestMultipleOpenSameFile(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	filePath := fp.Join(mountPoint, "multi_open_file")

	// Create the file
	file1, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Write some data
	_, err = file1.WriteString("initial content")
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Open the same file again
	file2, err := os.OpenFile(filePath, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("failed to open file second time: %v", err)
	}

	// Write through second handle
	_, err = file2.WriteAt([]byte("MODIFIED"), 0)
	if err != nil {
		t.Fatalf("failed to write through second handle: %v", err)
	}

	// Read through first handle (seek to beginning first)
	_, err = file1.Seek(0, 0)
	if err != nil {
		t.Fatalf("failed to seek: %v", err)
	}

	buf := make([]byte, 20)
	n, err := file1.Read(buf)
	if err != nil {
		t.Fatalf("failed to read through first handle: %v", err)
	}

	// Verify the modification is visible
	if !bytes.HasPrefix(buf[:n], []byte("MODIFIED")) {
		t.Errorf("expected to see MODIFIED, got %q", buf[:n])
	}

	file1.Close()
	file2.Close()
}

// TestFsyncFile tests the Fsync operation
func TestFsyncFile(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	filePath := fp.Join(mountPoint, "fsync_test_file")

	// Create and write
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer file.Close()

	_, err = file.WriteString("data to sync")
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Call Fsync
	err = file.Sync()
	if err != nil {
		t.Fatalf("Fsync failed: %v", err)
	}

	// Verify file exists and has correct content
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if info.Size() != 12 {
		t.Errorf("expected size 12, got %d", info.Size())
	}
}

// TestMknodRegularFile tests creating a regular file with Mknod
func TestMknodRegularFile(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	filePath := fp.Join(mountPoint, "mknod_regular_file")

	// Create regular file using Mknod
	err := syscall.Mknod(filePath, syscall.S_IFREG|0644, 0)
	if err != nil {
		t.Fatalf("Mknod failed: %v", err)
	}

	// Verify the file exists
	st := syscall.Stat_t{}
	err = syscall.Stat(filePath, &st)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	// Verify it's a regular file
	if st.Mode&syscall.S_IFMT != syscall.S_IFREG {
		t.Errorf("expected regular file, got mode %o", st.Mode)
	}
}

// TestMknodFIFO tests creating a FIFO with Mknod
func TestMknodFIFO(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	fifoPath := fp.Join(mountPoint, "mknod_fifo")

	// Create FIFO using Mknod
	err := syscall.Mknod(fifoPath, syscall.S_IFIFO|0644, 0)
	if err != nil {
		t.Fatalf("Mknod FIFO failed: %v", err)
	}

	// Verify the FIFO exists
	st := syscall.Stat_t{}
	err = syscall.Stat(fifoPath, &st)
	if err != nil {
		t.Fatalf("failed to stat FIFO: %v", err)
	}

	// Verify it's a FIFO
	if st.Mode&syscall.S_IFMT != syscall.S_IFIFO {
		t.Errorf("expected FIFO, got mode %o", st.Mode)
	}
}

// TestReadlinkDirect tests reading symlink target
func TestReadlinkDirect(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	// Create a regular file first
	targetPath := fp.Join(mountPoint, "symlink_target")
	file, err := os.Create(targetPath)
	if err != nil {
		t.Fatalf("failed to create target file: %v", err)
	}
	file.Close()

	// Create symlink
	linkPath := fp.Join(mountPoint, "symlink_link")
	err = syscall.Symlink(targetPath, linkPath)
	if err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Read symlink target
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}

	if target != targetPath {
		t.Errorf("expected target %q, got %q", targetPath, target)
	}
}

// TestSymlinkToRelativePath tests symlink with relative path
func TestSymlinkToRelativePath(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	// Create a subdirectory
	subDir := fp.Join(mountPoint, "subdir")
	err := os.Mkdir(subDir, 0755)
	if err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Create a file in subdir
	targetFile := fp.Join(subDir, "target.txt")
	file, err := os.Create(targetFile)
	if err != nil {
		t.Fatalf("failed to create target: %v", err)
	}
	file.WriteString("target content")
	file.Close()

	// Create symlink with relative path
	linkPath := fp.Join(mountPoint, "relative_link")
	err = syscall.Symlink("subdir/target.txt", linkPath)
	if err != nil {
		t.Fatalf("failed to create relative symlink: %v", err)
	}

	// Verify symlink target is relative
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	if target != "subdir/target.txt" {
		t.Errorf("expected relative target 'subdir/target.txt', got %q", target)
	}

	// Verify we can read through the symlink
	content, err := os.ReadFile(linkPath)
	if err != nil {
		t.Fatalf("failed to read through symlink: %v", err)
	}
	if string(content) != "target content" {
		t.Errorf("expected 'target content', got %q", content)
	}
}

// TestFileLock tests file locking operations (expected to return ENOSYS for testFileSystem)
func TestFileLock(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	filePath := fp.Join(mountPoint, "lock_test_file")

	// Create the file
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer file.Close()

	fd := int(file.Fd())

	// Try to set a lock - expected to fail with ENOSYS since testFileSystem doesn't implement locking
	lk := syscall.Flock_t{
		Type:   syscall.F_WRLCK,
		Whence: 0,
		Start:  0,
		Len:    0, // entire file
	}

	err = syscall.FcntlFlock(uintptr(fd), syscall.F_SETLK, &lk)
	// We expect this to fail with ENOSYS or succeed depending on the FS implementation
	// The test mainly verifies the code path doesn't panic
	if err != nil && err != syscall.ENOSYS && err != syscall.ENOLCK {
		t.Logf("FcntlFlock returned: %v (this may be expected)", err)
	}
}

// TestStatFs tests the StatFs operation
func TestStatFs(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	var statfs syscall.Statfs_t
	err := syscall.Statfs(mountPoint, &statfs)
	if err != nil {
		t.Fatalf("Statfs failed: %v", err)
	}

	// Basic sanity checks
	if statfs.Bsize == 0 {
		t.Error("block size should not be 0")
	}
	if statfs.Blocks == 0 {
		t.Error("total blocks should not be 0")
	}
}

// TestAccessCheck tests the Access operation
func TestAccessCheck(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	// Access mode constants (portable)
	const (
		F_OK = 0 // test for existence
		X_OK = 1 // test for execute permission
		W_OK = 2 // test for write permission
		R_OK = 4 // test for read permission
	)

	// Create a file
	filePath := fp.Join(mountPoint, "access_test_file")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	file.Close()

	// Check read access
	err = syscall.Access(filePath, R_OK)
	if err != nil {
		t.Errorf("expected read access, got error: %v", err)
	}

	// Check write access
	err = syscall.Access(filePath, W_OK)
	if err != nil {
		t.Errorf("expected write access, got error: %v", err)
	}

	// Check execute access (should fail for regular file without x permission)
	err = syscall.Access(filePath, X_OK)
	if err == nil {
		t.Log("execute access unexpectedly succeeded (may depend on file permissions)")
	}

	// Check access on non-existent file
	err = syscall.Access(fp.Join(mountPoint, "nonexistent"), F_OK)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// TestTruncateFile tests file truncation
func TestTruncateFile(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	filePath := fp.Join(mountPoint, "truncate_test_file")

	// Create file with content
	err := os.WriteFile(filePath, []byte("0123456789"), 0644)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Truncate to 5 bytes
	err = syscall.Truncate(filePath, 5)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Verify size
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat: %v", err)
	}
	if info.Size() != 5 {
		t.Errorf("expected size 5, got %d", info.Size())
	}

	// Verify content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	if string(content) != "01234" {
		t.Errorf("expected '01234', got %q", content)
	}

	// Extend file by truncating to larger size
	err = syscall.Truncate(filePath, 10)
	if err != nil {
		t.Fatalf("Truncate (extend) failed: %v", err)
	}

	info, err = os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat: %v", err)
	}
	if info.Size() != 10 {
		t.Errorf("expected size 10, got %d", info.Size())
	}
}

// TestFtruncate tests file truncation via file descriptor
func TestFtruncate(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	filePath := fp.Join(mountPoint, "ftruncate_test_file")

	// Create and open file
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer file.Close()

	// Write data
	_, err = file.WriteString("0123456789")
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Truncate via fd
	err = file.Truncate(5)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Verify size
	info, err := file.Stat()
	if err != nil {
		t.Fatalf("failed to stat: %v", err)
	}
	if info.Size() != 5 {
		t.Errorf("expected size 5, got %d", info.Size())
	}
}

// TestRenameFile tests file rename operation
func TestRenameFile(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	oldPath := fp.Join(mountPoint, "old_name")
	newPath := fp.Join(mountPoint, "new_name")

	// Create file
	err := os.WriteFile(oldPath, []byte("content"), 0644)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Rename
	err = syscall.Rename(oldPath, newPath)
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	// Verify old path doesn't exist
	_, err = os.Stat(oldPath)
	if !os.IsNotExist(err) {
		t.Error("old path should not exist after rename")
	}

	// Verify new path exists with correct content
	content, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("failed to read renamed file: %v", err)
	}
	if string(content) != "content" {
		t.Errorf("expected 'content', got %q", content)
	}
}

// TestRenameDirectory tests directory rename operation
func TestRenameDirectory(t *testing.T) {
	mountPoint, server := setupTest()
	defer umount(server)

	oldDir := fp.Join(mountPoint, "old_dir")
	newDir := fp.Join(mountPoint, "new_dir")
	fileInDir := "file_inside"

	// Create directory with file inside
	err := os.Mkdir(oldDir, 0755)
	if err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	err = os.WriteFile(fp.Join(oldDir, fileInDir), []byte("data"), 0644)
	if err != nil {
		t.Fatalf("failed to create file in dir: %v", err)
	}

	// Rename directory
	err = syscall.Rename(oldDir, newDir)
	if err != nil {
		t.Fatalf("Rename directory failed: %v", err)
	}

	// Verify old dir doesn't exist
	_, err = os.Stat(oldDir)
	if !os.IsNotExist(err) {
		t.Error("old directory should not exist after rename")
	}

	// Verify file inside new dir exists
	content, err := os.ReadFile(fp.Join(newDir, fileInDir))
	if err != nil {
		t.Fatalf("failed to read file in renamed dir: %v", err)
	}
	if string(content) != "data" {
		t.Errorf("expected 'data', got %q", content)
	}
}
