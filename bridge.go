// Copyright 2022 someonegg. All rights reserscoreed.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pathfs

import (
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type rawBridge struct {
	fs      FileSystem
	options Options
	root    *inode

	mu sync.Mutex

	nodes map[uint64]*inode

	nodeCountHigh int

	files     []*fileEntry
	freeFiles []uint32
}

// NewPathFS creates a path based filesystem.
func NewPathFS(fs FileSystem, options *Options) fuse.RawFileSystem {
	if options == nil {
		oneSec := time.Second
		options = &Options{
			EntryTimeout: &oneSec,
			AttrTimeout:  &oneSec,
		}
	}

	b := &rawBridge{
		fs:      fs,
		options: *options,
		root:    newInode(1, true),
	}

	b.nodes = map[uint64]*inode{1: b.root}
	b.root.lookupCount = 1
	b.nodeCountHigh = 1

	// Fh 0 means no file handle.
	b.files = []*fileEntry{{}}

	return b
}

func (b *rawBridge) logf(format string, args ...interface{}) {
	if b.options.Logger != nil {
		b.options.Logger.Printf(format, args...)
	}
}

func (b *rawBridge) inode(ino uint64) *inode {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.nodes[ino]
	if n == nil {
		log.Panicf("unknown node %d", ino)
	}
	return n
}

func (b *rawBridge) inodeAndFile(ino uint64, fh uint32, ctx *Context) (*inode, *fileEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, f := b.nodes[ino], b.files[fh]
	if n == nil {
		log.Panicf("unknown node %d", ino)
	}
	if f == nil {
		log.Panicf("unknown file %d", fh)
	}
	if fh != 0 {
		ctx.Opener = &f.opener
	}
	return n, f
}

func (b *rawBridge) Init(s *fuse.Server) {}

func (b *rawBridge) String() string {
	return "pathfs"
}

func (b *rawBridge) NodeCount() int {
	b.mu.Lock()
	n := len(b.nodes)
	b.mu.Unlock()
	return n
}

func (b *rawBridge) SetDebug(debug bool) {}

func (b *rawBridge) Access(cancel <-chan struct{}, input *fuse.AccessIn) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n := b.inode(input.NodeId)
	path := b.pathOf(n)

	return b.fs.Access(ctx, path, input.Mask)
}

func (b *rawBridge) Lookup(cancel <-chan struct{}, header *fuse.InHeader, name string, out *fuse.EntryOut) fuse.Status {
	ctx := newContext(cancel, header.Caller)
	defer releaseContext(ctx)

	parent := b.inode(header.NodeId)
	path := childPathOf(b.pathOf(parent), name)

	code := b.lookup(ctx, path, parent, name, out)
	if !code.Ok() {
		b.rmChild(parent, name)
		if b.options.NegativeTimeout != nil {
			out.SetEntryTimeout(*b.options.NegativeTimeout)
		}
	}

	return code
}

func (b *rawBridge) lookup(ctx *Context, path string, parent *inode, name string, out *fuse.EntryOut) fuse.Status {
	code := b.fs.GetAttr(ctx, path, 0, &out.Attr)
	if !code.Ok() {
		return code
	}

	child := b.addChild(parent, name, out.Attr.Ino, out.Attr.IsDir())

	b.setEntryOut(child, out)
	b.setEntryOutTimeout(out)
	return fuse.OK
}

func (b *rawBridge) Forget(nodeid, nlookup uint64) {
	n := b.inode(nodeid)

	removed := b.removeRef(n, uint32(nlookup))
	if removed {
		b.compactMemory()
	}
}

func (b *rawBridge) compactMemory() {
	b.mu.Lock()

	if b.nodeCountHigh <= len(b.nodes)*100 {
		b.mu.Unlock()
		return
	}

	tmpNodes := make(map[uint64]*inode, len(b.nodes))
	for i, v := range b.nodes {
		tmpNodes[i] = v
	}
	b.nodes = tmpNodes

	b.nodeCountHigh = len(b.nodes)

	b.mu.Unlock()

	debug.FreeOSMemory()
}

func (b *rawBridge) GetAttr(cancel <-chan struct{}, input *fuse.GetAttrIn, out *fuse.AttrOut) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, f := b.inodeAndFile(input.NodeId, uint32(input.Fh()), ctx)
	path := b.fpathOf(n, f)

	return b.getAttr(ctx, path, f.uFh, out)
}

func (b *rawBridge) getAttr(ctx *Context, path string, uFh uint32, out *fuse.AttrOut) fuse.Status {
	code := b.fs.GetAttr(ctx, path, uFh, &out.Attr)
	if !code.Ok() {
		return code
	}

	b.setAttr(out)
	b.setAttrTimeout(out)
	return fuse.OK
}

func (b *rawBridge) SetAttr(cancel <-chan struct{}, input *fuse.SetAttrIn, out *fuse.AttrOut) (code fuse.Status) {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	fh, _ := input.GetFh()
	n, f := b.inodeAndFile(input.NodeId, uint32(fh), ctx)
	path := b.fpathOf(n, f)

	if perms, ok := input.GetMode(); ok {
		code = b.fs.Chmod(ctx, path, f.uFh, perms)
	}

	uid, uok := input.GetUID()
	gid, gok := input.GetGID()
	if code.Ok() && (uok || gok) {
		code = b.fs.Chown(ctx, path, f.uFh, uid, gid)
	}

	if sz, ok := input.GetSize(); code.Ok() && ok {
		code = b.fs.Truncate(ctx, path, f.uFh, sz)
	}

	atime, aok := input.GetATime()
	mtime, mok := input.GetMTime()
	if code.Ok() && (aok || mok) {
		var a, m *time.Time
		if aok {
			a = &atime
		}
		if mok {
			m = &mtime
		}
		code = b.fs.Utimens(ctx, path, f.uFh, a, m)
	}

	if !code.Ok() {
		return code
	}

	return b.getAttr(ctx, path, f.uFh, out)
}

func (b *rawBridge) Mknod(cancel <-chan struct{}, input *fuse.MknodIn, name string, out *fuse.EntryOut) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	parent := b.inode(input.NodeId)
	path := childPathOf(b.pathOf(parent), name)

	code := b.fs.Mknod(ctx, path, input.Mode, input.Rdev)
	if !code.Ok() {
		return code
	}

	return b.lookup(ctx, path, parent, name, out)
}

func (b *rawBridge) Mkdir(cancel <-chan struct{}, input *fuse.MkdirIn, name string, out *fuse.EntryOut) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	parent := b.inode(input.NodeId)
	path := childPathOf(b.pathOf(parent), name)

	code := b.fs.Mkdir(ctx, path, input.Mode)
	if !code.Ok() {
		return code
	}

	return b.lookup(ctx, path, parent, name, out)
}

func (b *rawBridge) Unlink(cancel <-chan struct{}, header *fuse.InHeader, name string) fuse.Status {
	ctx := newContext(cancel, header.Caller)
	defer releaseContext(ctx)

	parent := b.inode(header.NodeId)
	path := childPathOf(b.pathOf(parent), name)

	code := b.fs.Unlink(ctx, path)
	if !code.Ok() {
		return code
	}

	b.rmChild(parent, name)
	return fuse.OK
}

func (b *rawBridge) Rmdir(cancel <-chan struct{}, header *fuse.InHeader, name string) fuse.Status {
	ctx := newContext(cancel, header.Caller)
	defer releaseContext(ctx)

	parent := b.inode(header.NodeId)
	path := childPathOf(b.pathOf(parent), name)

	code := b.fs.Rmdir(ctx, path)
	if !code.Ok() {
		return code
	}

	b.rmChild(parent, name)
	return fuse.OK
}

func (b *rawBridge) Rename(cancel <-chan struct{}, input *fuse.RenameIn, name string, newName string) fuse.Status {
	if input.Flags != 0 {
		return fuse.ENOSYS
	}

	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	parent := b.inode(input.NodeId)
	path := childPathOf(b.pathOf(parent), name)

	newParent := b.inode(input.Newdir)
	newPath := childPathOf(b.pathOf(newParent), newName)

	code := b.fs.Rename(ctx, path, newPath)
	if !code.Ok() {
		return code
	}

	b.mvChild(parent, name, newParent, newName, true)
	return fuse.OK
}

func (b *rawBridge) Link(cancel <-chan struct{}, input *fuse.LinkIn, name string, out *fuse.EntryOut) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	old := b.inode(input.Oldnodeid)
	oldPath := b.pathOf(old)

	parent := b.inode(input.NodeId)
	path := childPathOf(b.pathOf(parent), name)

	code := b.fs.Link(ctx, oldPath, path)
	if !code.Ok() {
		return code
	}

	return b.lookup(ctx, path, parent, name, out)
}

func (b *rawBridge) Symlink(cancel <-chan struct{}, header *fuse.InHeader, target string, name string, out *fuse.EntryOut) fuse.Status {
	ctx := newContext(cancel, header.Caller)
	defer releaseContext(ctx)

	parent := b.inode(header.NodeId)
	path := childPathOf(b.pathOf(parent), name)

	code := b.fs.Symlink(ctx, path, target)
	if !code.Ok() {
		return code
	}

	return b.lookup(ctx, path, parent, name, out)
}

func (b *rawBridge) Readlink(cancel <-chan struct{}, header *fuse.InHeader) ([]byte, fuse.Status) {
	ctx := newContext(cancel, header.Caller)
	defer releaseContext(ctx)

	n := b.inode(header.NodeId)
	path := b.pathOf(n)

	target, code := b.fs.Readlink(ctx, path)
	return []byte(target), code
}

func (b *rawBridge) GetXAttr(cancel <-chan struct{}, header *fuse.InHeader, attr string, dest []byte) (uint32, fuse.Status) {
	ctx := newContext(cancel, header.Caller)
	defer releaseContext(ctx)

	n := b.inode(header.NodeId)
	path := b.pathOf(n)

	data, code := b.fs.GetXAttr(ctx, path, attr)
	if !code.Ok() {
		return 0, code
	}

	sz := len(data)
	if sz > len(dest) {
		return uint32(sz), fuse.ERANGE
	}

	copy(dest, data)
	return uint32(sz), fuse.OK
}

func (b *rawBridge) ListXAttr(cancel <-chan struct{}, header *fuse.InHeader, dest []byte) (uint32, fuse.Status) {
	ctx := newContext(cancel, header.Caller)
	defer releaseContext(ctx)

	n := b.inode(header.NodeId)
	path := b.pathOf(n)

	attrs, code := b.fs.ListXAttr(ctx, path)
	if !code.Ok() {
		return 0, code
	}

	sz := 0
	for _, v := range attrs {
		sz += len(v) + 1
	}
	if sz > len(dest) {
		return uint32(sz), fuse.ERANGE
	}

	dest = dest[:0]
	for _, v := range attrs {
		dest = append(dest, v...)
		dest = append(dest, 0)
	}
	return uint32(sz), fuse.OK
}

func (b *rawBridge) SetXAttr(cancel <-chan struct{}, input *fuse.SetXAttrIn, attr string, data []byte) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n := b.inode(input.NodeId)
	path := b.pathOf(n)

	return b.fs.SetXAttr(ctx, path, attr, data, input.Flags)
}

func (b *rawBridge) RemoveXAttr(cancel <-chan struct{}, header *fuse.InHeader, attr string) fuse.Status {
	ctx := newContext(cancel, header.Caller)
	defer releaseContext(ctx)

	n := b.inode(header.NodeId)
	path := b.pathOf(n)

	return b.fs.RemoveXAttr(ctx, path, attr)
}

func (b *rawBridge) Create(cancel <-chan struct{}, input *fuse.CreateIn, name string, out *fuse.CreateOut) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	parent := b.inode(input.NodeId)
	path := childPathOf(b.pathOf(parent), name)

	uFh, forceDIO, code := b.fs.Create(ctx, path, input.Flags, input.Mode)
	if !code.Ok() {
		return code
	}
	code = b.lookup(ctx, path, parent, name, &out.EntryOut)
	if !code.Ok() {
		return code
	}
	if forceDIO {
		out.OpenFlags |= fuse.FOPEN_DIRECT_IO
	}
	out.Fh = uint64(b.registerFile(input.Caller.Owner, path, uFh, nil))
	return fuse.OK
}

func (b *rawBridge) Open(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n := b.inode(input.NodeId)
	path := b.pathOf(n)

	uFh, keepCache, forceDIO, code := b.fs.Open(ctx, path, input.Flags)
	if !code.Ok() {
		return code
	}

	out.Fh = uint64(b.registerFile(input.Caller.Owner, path, uFh, nil))
	if forceDIO {
		out.OpenFlags |= fuse.FOPEN_DIRECT_IO
	} else if keepCache {
		out.OpenFlags |= fuse.FOPEN_KEEP_CACHE
	}
	return fuse.OK
}

func (b *rawBridge) Read(cancel <-chan struct{}, input *fuse.ReadIn, dest []byte) (fuse.ReadResult, fuse.Status) {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, f := b.inodeAndFile(input.NodeId, uint32(input.Fh), ctx)
	path := b.fpathOf(n, f)

	return b.fs.Read(ctx, path, f.uFh, dest, input.Offset)
}

func (b *rawBridge) Write(cancel <-chan struct{}, input *fuse.WriteIn, data []byte) (written uint32, status fuse.Status) {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, f := b.inodeAndFile(input.NodeId, uint32(input.Fh), ctx)
	path := b.fpathOf(n, f)

	return b.fs.Write(ctx, path, f.uFh, data, input.Offset)
}

func (b *rawBridge) Fallocate(cancel <-chan struct{}, input *fuse.FallocateIn) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, f := b.inodeAndFile(input.NodeId, uint32(input.Fh), ctx)
	path := b.fpathOf(n, f)

	return b.fs.Fallocate(ctx, path, f.uFh, input.Offset, input.Length, input.Mode)
}

func (b *rawBridge) Fsync(cancel <-chan struct{}, input *fuse.FsyncIn) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, f := b.inodeAndFile(input.NodeId, uint32(input.Fh), ctx)
	path := b.fpathOf(n, f)

	return b.fs.Fsync(ctx, path, f.uFh, input.FsyncFlags)
}

func (b *rawBridge) Flush(cancel <-chan struct{}, input *fuse.FlushIn) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, f := b.inodeAndFile(input.NodeId, uint32(input.Fh), ctx)
	path := b.fpathOf(n, f)

	return b.fs.Flush(ctx, path, f.uFh, input.LockOwner)
}

func (b *rawBridge) Release(cancel <-chan struct{}, input *fuse.ReleaseIn) {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, f := b.inodeAndFile(input.NodeId, uint32(input.Fh), ctx)
	path := b.fpathOf(n, f)

	b.fs.Release(ctx, path, f.uFh)

	b.unregisterFile(uint32(input.Fh))
}

func (b *rawBridge) GetLk(cancel <-chan struct{}, input *fuse.LkIn, out *fuse.LkOut) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, f := b.inodeAndFile(input.NodeId, uint32(input.Fh), ctx)
	path := b.fpathOf(n, f)

	return b.fs.GetLk(ctx, path, f.uFh, input.Owner, &input.Lk, input.LkFlags, &out.Lk)
}

func (b *rawBridge) SetLk(cancel <-chan struct{}, input *fuse.LkIn) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, f := b.inodeAndFile(input.NodeId, uint32(input.Fh), ctx)
	path := b.fpathOf(n, f)

	return b.fs.SetLk(ctx, path, f.uFh, input.Owner, &input.Lk, input.LkFlags)
}

func (b *rawBridge) SetLkw(cancel <-chan struct{}, input *fuse.LkIn) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, f := b.inodeAndFile(input.NodeId, uint32(input.Fh), ctx)
	path := b.fpathOf(n, f)

	return b.fs.SetLkw(ctx, path, f.uFh, input.Owner, &input.Lk, input.LkFlags)
}

func (b *rawBridge) OpenDir(cancel <-chan struct{}, input *fuse.OpenIn, out *fuse.OpenOut) fuse.Status {
	n := b.inode(input.NodeId)
	path := b.pathOf(n)

	out.Fh = uint64(b.registerFile(input.Caller.Owner, path, 0, nil))
	return fuse.OK
}

func (b *rawBridge) ReadDir(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, d := b.inodeAndFile(input.NodeId, uint32(input.Fh), ctx)
	path := b.fpathOf(n, d)

	d.mu.Lock()
	defer d.mu.Unlock()

	// rewinddir() should be as if reopening directory.
	if d.stream == nil || input.Offset == 0 {
		stream, code := b.fs.Lsdir(ctx, path)
		if !code.Ok() {
			return code
		}
		d.stream = append(stream,
			fuse.DirEntry{Mode: fuse.S_IFDIR, Name: "."},
			fuse.DirEntry{Mode: fuse.S_IFDIR, Name: ".."})
	}

	if input.Offset > uint64(len(d.stream)) {
		// See https://github.com/hanwen/go-fuse/issues/297
		// This can happen for FUSE exported over NFS.  This
		// seems incorrect, (maybe the kernel is using offsets
		// from other opendir/readdir calls), it is harmless to reinforce that
		// we have reached EOF.
		return fuse.OK
	}

	for _, e := range d.stream[input.Offset:] {
		if e.Name == "" {
			b.logf("warning: got empty directory entry, mode %o.", e.Mode)
			continue
		}

		ok := out.AddDirEntry(e)
		if !ok {
			break
		}
	}
	return fuse.OK
}

func (b *rawBridge) ReadDirPlus(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n, d := b.inodeAndFile(input.NodeId, uint32(input.Fh), ctx)
	path := b.fpathOf(n, d)

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stream == nil || input.Offset == 0 {
		stream, code := b.fs.Lsdir(ctx, path)
		if !code.Ok() {
			return code
		}
		d.stream = append(stream,
			fuse.DirEntry{Mode: fuse.S_IFDIR, Name: "."},
			fuse.DirEntry{Mode: fuse.S_IFDIR, Name: ".."})
	}

	if input.Offset > uint64(len(d.stream)) {
		return fuse.OK
	}

	for _, e := range d.stream[input.Offset:] {
		if e.Name == "" {
			b.logf("warning: got empty directory entry, mode %o.", e.Mode)
			continue
		}

		// we have to be sure entry will fit if we try to add
		// it, or we'll mess up the lookup counts.
		entryOut := out.AddDirLookupEntry(e)
		if entryOut == nil {
			break
		}
		// No need to fill attributes for . and ..
		if e.Name == "." || e.Name == ".." {
			continue
		}

		b.lookup(ctx, childPathOf(path, e.Name), n, e.Name, entryOut)
	}
	return fuse.OK
}

func (b *rawBridge) ReleaseDir(input *fuse.ReleaseIn) {
	b.unregisterFile(uint32(input.Fh))
}

func (b *rawBridge) FsyncDir(cancel <-chan struct{}, input *fuse.FsyncIn) fuse.Status {
	return fuse.ENOSYS
}

func (b *rawBridge) Lseek(cancel <-chan struct{}, input *fuse.LseekIn, out *fuse.LseekOut) fuse.Status {
	return fuse.ENOSYS
}

func (b *rawBridge) CopyFileRange(cancel <-chan struct{}, input *fuse.CopyFileRangeIn) (written uint32, code fuse.Status) {
	return 0, fuse.ENOSYS
}

func (b *rawBridge) StatFs(cancel <-chan struct{}, input *fuse.InHeader, out *fuse.StatfsOut) fuse.Status {
	ctx := newContext(cancel, input.Caller)
	defer releaseContext(ctx)

	n := b.inode(input.NodeId)
	path := b.pathOf(n)

	return b.fs.StatFs(ctx, path, out)
}

func (b *rawBridge) Dump() (data *DumpRawBridge, iterator InodeIterator, err error) {
	files := make([]*DumpFileEntry, len(b.files))
	for i, f := range b.files {
		files[i] = &DumpFileEntry{
			Opener: f.opener,
			Path:   f.path,
			UFh:    f.uFh,
			Stream: f.stream,
		}
	}

	data = &DumpRawBridge{
		NodeCount: b.NodeCount(),
		Files:     files,
		FreeFiles: b.freeFiles,
	}

	inodeIterator := NewInodeDumper(b.nodes)

	return data, inodeIterator, nil

}

func (b *rawBridge) Restore(data *DumpRawBridge) (filler InodeFiller, err error) {
	b.nodes = map[uint64]*inode{}
	files := make([]*fileEntry, len(data.Files))
	for i, v := range data.Files {
		files[i] = &fileEntry{
			opener: v.Opener,
			path:   v.Path,
			uFh:    v.UFh,
			stream: v.Stream,
		}
	}
	b.files = files
	b.freeFiles = data.FreeFiles

	return &InodeRestorer{
		bridge:    b,
		nodeCount: data.NodeCount,
	}, nil
}
