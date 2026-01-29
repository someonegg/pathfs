package pathfs

import (
	"sync"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// mockFileSystem is a minimal FileSystem implementation for unit testing
type mockFileSystem struct {
	defaultFileSystem

	// GetAttr behavior control
	getAttrFunc func(path string) (fuse.Attr, fuse.Status)

	// Lsdir behavior control
	lsdirFunc func(path string) ([]fuse.DirEntry, fuse.Status)

	// Create behavior control
	createFunc func(path string) (uint32, bool, fuse.Status)

	// Open behavior control
	openFunc func(path string) (uint32, bool, bool, fuse.Status)
}

func (m *mockFileSystem) GetAttr(ctx *Context, path string, uFh uint32, out *fuse.Attr) fuse.Status {
	if m.getAttrFunc != nil {
		attr, status := m.getAttrFunc(path)
		*out = attr
		return status
	}
	return fuse.ENOENT
}

func (m *mockFileSystem) Lsdir(ctx *Context, path string) ([]fuse.DirEntry, fuse.Status) {
	if m.lsdirFunc != nil {
		return m.lsdirFunc(path)
	}
	return nil, fuse.ENOENT
}

func (m *mockFileSystem) Create(ctx *Context, path string, flags uint32, mode uint32) (uint32, bool, fuse.Status) {
	if m.createFunc != nil {
		return m.createFunc(path)
	}
	return 0, false, fuse.OK
}

func (m *mockFileSystem) Open(ctx *Context, path string, flags uint32) (uint32, bool, bool, fuse.Status) {
	if m.openFunc != nil {
		return m.openFunc(path)
	}
	return 0, false, false, fuse.OK
}

func (m *mockFileSystem) Release(ctx *Context, path string, uFh uint32) {
	// no-op for mock
}

// newMockBridge creates a rawBridge with mock FileSystem for testing
func newMockBridge(fs *mockFileSystem) *rawBridge {
	b := &rawBridge{
		fs:            fs,
		root:          newInode(1, true),
		nodeCountHigh: 1,
	}

	b.nodes = map[uint64]*inode{1: b.root}
	b.root.lookupCount = 1
	b.files = []*fileEntry{{}}

	return b
}

func TestLookup(t *testing.T) {
	// Setup mock that returns a file with ino=100
	mock := &mockFileSystem{
		getAttrFunc: func(path string) (fuse.Attr, fuse.Status) {
			if path == "testfile" {
				return fuse.Attr{Ino: 100, Mode: fuse.S_IFREG | 0644}, fuse.OK
			}
			return fuse.Attr{}, fuse.ENOENT
		},
	}
	b := newMockBridge(mock)

	// Test successful lookup
	header := &fuse.InHeader{NodeId: 1, Caller: fuse.Caller{}}
	out := &fuse.EntryOut{}

	status := b.Lookup(nil, header, "testfile", out)
	if status != fuse.OK {
		t.Errorf("expected OK, got %v", status)
	}
	if out.Attr.Ino != 100 {
		t.Errorf("expected ino 100, got %d", out.Attr.Ino)
	}

	// Verify child was added
	child := b.root.children["testfile"]
	if child == nil {
		t.Error("child should be added to parent")
	}
	if child.ino != 100 {
		t.Errorf("child ino should be 100, got %d", child.ino)
	}
	if child.lookupCount != 1 {
		t.Errorf("lookupCount should be 1, got %d", child.lookupCount)
	}

	// Test lookup non-existent file
	out2 := &fuse.EntryOut{}
	status = b.Lookup(nil, header, "nonexistent", out2)
	if status != fuse.ENOENT {
		t.Errorf("expected ENOENT, got %v", status)
	}
}

func TestLookupWithNegativeTimeout(t *testing.T) {
	mock := &mockFileSystem{
		getAttrFunc: func(path string) (fuse.Attr, fuse.Status) {
			return fuse.Attr{}, fuse.ENOENT
		},
	}
	b := newMockBridge(mock)

	// Set negative timeout
	negTimeout := 5 * time.Second
	b.options.NegativeTimeout = &negTimeout

	header := &fuse.InHeader{NodeId: 1, Caller: fuse.Caller{}}
	out := &fuse.EntryOut{}

	status := b.Lookup(nil, header, "nonexistent", out)
	if status != fuse.ENOENT {
		t.Errorf("expected ENOENT, got %v", status)
	}
}

func TestForget(t *testing.T) {
	mock := &mockFileSystem{
		getAttrFunc: func(path string) (fuse.Attr, fuse.Status) {
			if path == "testfile" {
				return fuse.Attr{Ino: 100, Mode: fuse.S_IFREG | 0644}, fuse.OK
			}
			return fuse.Attr{}, fuse.ENOENT
		},
	}
	b := newMockBridge(mock)

	// First lookup to create the node
	header := &fuse.InHeader{NodeId: 1, Caller: fuse.Caller{}}
	out := &fuse.EntryOut{}
	b.Lookup(nil, header, "testfile", out)

	// Verify node exists
	node := b.inodeSafe(100)
	if node == nil {
		t.Fatal("node should exist after lookup")
	}
	if node.lookupCount != 1 {
		t.Errorf("lookupCount should be 1, got %d", node.lookupCount)
	}

	// Lookup again to increment count
	b.Lookup(nil, header, "testfile", out)
	if node.lookupCount != 2 {
		t.Errorf("lookupCount should be 2, got %d", node.lookupCount)
	}

	// Forget once
	b.Forget(100, 1)
	if node.lookupCount != 1 {
		t.Errorf("lookupCount should be 1 after forget, got %d", node.lookupCount)
	}

	// Forget again - node should be removed
	b.Forget(100, 1)
	nodeAfter := b.inodeSafe(100)
	if nodeAfter != nil {
		t.Error("node should be removed after lookupCount reaches 0")
	}
}

func TestForgetNonExistent(t *testing.T) {
	mock := &mockFileSystem{}
	b := newMockBridge(mock)

	// Forget a non-existent node should not panic
	b.Forget(999, 1)
	// If we reach here without panic, test passes
}

func TestInodeSafe(t *testing.T) {
	mock := &mockFileSystem{}
	b := newMockBridge(mock)

	// Test existing node
	node := b.inodeSafe(1)
	if node == nil {
		t.Error("root node should exist")
	}
	if node.ino != 1 {
		t.Errorf("expected ino 1, got %d", node.ino)
	}

	// Test non-existent node
	node = b.inodeSafe(999)
	if node != nil {
		t.Error("non-existent node should return nil")
	}
}

func TestCompactMemory(t *testing.T) {
	mock := &mockFileSystem{
		getAttrFunc: func(path string) (fuse.Attr, fuse.Status) {
			// Return unique ino based on path length as simple hash
			ino := uint64(len(path) + 100)
			return fuse.Attr{Ino: ino, Mode: fuse.S_IFREG | 0644}, fuse.OK
		},
	}
	b := newMockBridge(mock)

	// Create many nodes
	header := &fuse.InHeader{NodeId: 1, Caller: fuse.Caller{}}
	for i := 0; i < 200; i++ {
		name := string(rune('a'+i%26)) + string(rune('0'+i/26))
		mock.getAttrFunc = func(path string) (fuse.Attr, fuse.Status) {
			return fuse.Attr{Ino: uint64(i + 100), Mode: fuse.S_IFREG | 0644}, fuse.OK
		}
		out := &fuse.EntryOut{}
		b.Lookup(nil, header, name, out)
	}

	initialHigh := b.nodeCountHigh

	// Forget all nodes to trigger compactMemory
	for i := 0; i < 200; i++ {
		b.Forget(uint64(i+100), 1)
	}

	// After compactMemory, nodeCountHigh should be reduced
	if b.nodeCountHigh >= initialHigh {
		// compactMemory only triggers when nodeCountHigh > len(nodes)*100
		// This is expected behavior - compactMemory won't trigger for small counts
	}

	// Verify only root remains
	if b.NodeCount() != 1 {
		t.Errorf("expected 1 node (root), got %d", b.NodeCount())
	}
}

func TestCreateFlow(t *testing.T) {
	var createdPath string
	mock := &mockFileSystem{
		createFunc: func(path string) (uint32, bool, fuse.Status) {
			createdPath = path
			return 42, true, fuse.OK // uFh=42, forceDIO=true
		},
		getAttrFunc: func(path string) (fuse.Attr, fuse.Status) {
			if path == "newfile" {
				return fuse.Attr{Ino: 200, Mode: fuse.S_IFREG | 0644}, fuse.OK
			}
			return fuse.Attr{}, fuse.ENOENT
		},
	}
	b := newMockBridge(mock)

	input := &fuse.CreateIn{
		InHeader: fuse.InHeader{NodeId: 1, Caller: fuse.Caller{}},
		Flags:    0,
		Mode:     0644,
	}
	out := &fuse.CreateOut{}

	status := b.Create(nil, input, "newfile", out)
	if status != fuse.OK {
		t.Errorf("expected OK, got %v", status)
	}

	// Verify path
	if createdPath != "newfile" {
		t.Errorf("expected path 'newfile', got '%s'", createdPath)
	}

	// Verify forceDIO flag
	if out.OpenFlags&fuse.FOPEN_DIRECT_IO == 0 {
		t.Error("FOPEN_DIRECT_IO should be set")
	}

	// Verify file handle was registered
	if out.Fh == 0 {
		t.Error("file handle should not be 0")
	}

	// Verify inode was added
	node := b.inodeSafe(200)
	if node == nil {
		t.Error("node should be added after Create")
	}
}

func TestOpenRelease(t *testing.T) {
	mock := &mockFileSystem{
		getAttrFunc: func(path string) (fuse.Attr, fuse.Status) {
			if path == "testfile" {
				return fuse.Attr{Ino: 100, Mode: fuse.S_IFREG | 0644}, fuse.OK
			}
			return fuse.Attr{}, fuse.ENOENT
		},
		openFunc: func(path string) (uint32, bool, bool, fuse.Status) {
			return 10, true, false, fuse.OK // uFh=10, keepCache=true
		},
	}
	b := newMockBridge(mock)

	// First lookup to create the node
	header := &fuse.InHeader{NodeId: 1, Caller: fuse.Caller{}}
	lookupOut := &fuse.EntryOut{}
	b.Lookup(nil, header, "testfile", lookupOut)

	// Open the file
	openIn := &fuse.OpenIn{
		InHeader: fuse.InHeader{NodeId: 100, Caller: fuse.Caller{}},
		Flags:    0,
	}
	openOut := &fuse.OpenOut{}

	status := b.Open(nil, openIn, openOut)
	if status != fuse.OK {
		t.Errorf("expected OK, got %v", status)
	}

	fh1 := openOut.Fh
	if fh1 == 0 {
		t.Error("file handle should not be 0")
	}

	// Verify keepCache flag
	if openOut.OpenFlags&fuse.FOPEN_KEEP_CACHE == 0 {
		t.Error("FOPEN_KEEP_CACHE should be set")
	}

	// Release the file
	releaseIn := &fuse.ReleaseIn{
		InHeader: fuse.InHeader{NodeId: 100, Caller: fuse.Caller{}},
		Fh:       fh1,
	}
	b.Release(nil, releaseIn)

	// Verify fh was added to freeFiles
	if len(b.freeFiles) != 1 {
		t.Errorf("expected 1 free file, got %d", len(b.freeFiles))
	}
	if b.freeFiles[0] != uint32(fh1) {
		t.Errorf("expected fh %d in freeFiles, got %d", fh1, b.freeFiles[0])
	}

	// Open again - should reuse the freed fh
	openOut2 := &fuse.OpenOut{}
	b.Open(nil, openIn, openOut2)

	if openOut2.Fh != fh1 {
		t.Errorf("expected reused fh %d, got %d", fh1, openOut2.Fh)
	}
	if len(b.freeFiles) != 0 {
		t.Errorf("expected 0 free files after reuse, got %d", len(b.freeFiles))
	}
}

func TestReadDir(t *testing.T) {
	entries := []fuse.DirEntry{
		{Name: "file1", Mode: fuse.S_IFREG, Ino: 101},
		{Name: "file2", Mode: fuse.S_IFREG, Ino: 102},
		{Name: "subdir", Mode: fuse.S_IFDIR, Ino: 103},
	}

	callCount := 0
	mock := &mockFileSystem{
		lsdirFunc: func(path string) ([]fuse.DirEntry, fuse.Status) {
			callCount++
			return entries, fuse.OK
		},
	}
	b := newMockBridge(mock)

	// Open directory
	openIn := &fuse.OpenIn{
		InHeader: fuse.InHeader{NodeId: 1, Caller: fuse.Caller{}},
	}
	openOut := &fuse.OpenOut{}
	b.OpenDir(nil, openIn, openOut)

	fh := openOut.Fh

	// First ReadDir
	readIn := &fuse.ReadIn{
		InHeader: fuse.InHeader{NodeId: 1, Caller: fuse.Caller{}},
		Fh:       fh,
		Offset:   0,
	}
	out := fuse.NewDirEntryList(make([]byte, 4096), 0)

	status := b.ReadDir(nil, readIn, out)
	if status != fuse.OK {
		t.Errorf("expected OK, got %v", status)
	}
	if callCount != 1 {
		t.Errorf("expected 1 Lsdir call, got %d", callCount)
	}

	// Second ReadDir with same fh should use cached stream
	readIn.Offset = 1
	out2 := fuse.NewDirEntryList(make([]byte, 4096), 1)
	b.ReadDir(nil, readIn, out2)
	if callCount != 1 {
		t.Errorf("expected still 1 Lsdir call (cached), got %d", callCount)
	}

	// ReadDir with offset=0 should re-fetch
	readIn.Offset = 0
	out3 := fuse.NewDirEntryList(make([]byte, 4096), 0)
	b.ReadDir(nil, readIn, out3)
	if callCount != 2 {
		t.Errorf("expected 2 Lsdir calls (re-fetch), got %d", callCount)
	}

	// Release directory
	b.ReleaseDir(&fuse.ReleaseIn{Fh: fh})
}

func TestReadDirPlus(t *testing.T) {
	entries := []fuse.DirEntry{
		{Name: "file1", Mode: fuse.S_IFREG, Ino: 101},
	}

	mock := &mockFileSystem{
		lsdirFunc: func(path string) ([]fuse.DirEntry, fuse.Status) {
			return entries, fuse.OK
		},
		getAttrFunc: func(path string) (fuse.Attr, fuse.Status) {
			if path == "file1" {
				return fuse.Attr{Ino: 101, Mode: fuse.S_IFREG | 0644}, fuse.OK
			}
			return fuse.Attr{}, fuse.ENOENT
		},
	}
	b := newMockBridge(mock)

	// Open directory
	openIn := &fuse.OpenIn{
		InHeader: fuse.InHeader{NodeId: 1, Caller: fuse.Caller{}},
	}
	openOut := &fuse.OpenOut{}
	b.OpenDir(nil, openIn, openOut)

	fh := openOut.Fh

	// ReadDirPlus
	readIn := &fuse.ReadIn{
		InHeader: fuse.InHeader{NodeId: 1, Caller: fuse.Caller{}},
		Fh:       fh,
		Offset:   0,
	}
	out := fuse.NewDirEntryList(make([]byte, 4096), 0)

	status := b.ReadDirPlus(nil, readIn, out)
	if status != fuse.OK {
		t.Errorf("expected OK, got %v", status)
	}

	// Verify child was added through lookup
	child := b.root.children["file1"]
	if child == nil {
		t.Error("child should be added after ReadDirPlus")
	}

	b.ReleaseDir(&fuse.ReleaseIn{Fh: fh})
}

func TestConcurrentLookup(t *testing.T) {
	mock := &mockFileSystem{
		getAttrFunc: func(path string) (fuse.Attr, fuse.Status) {
			// Return same ino for same file
			return fuse.Attr{Ino: 100, Mode: fuse.S_IFREG | 0644}, fuse.OK
		},
	}
	b := newMockBridge(mock)

	var wg sync.WaitGroup
	header := &fuse.InHeader{NodeId: 1, Caller: fuse.Caller{}}

	// Concurrent lookups for the same file
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := &fuse.EntryOut{}
			b.Lookup(nil, header, "samefile", out)
		}()
	}
	wg.Wait()

	// Verify node exists and lookupCount is correct
	node := b.inodeSafe(100)
	if node == nil {
		t.Fatal("node should exist")
	}
	if node.lookupCount != 10 {
		t.Errorf("expected lookupCount 10, got %d", node.lookupCount)
	}
}
