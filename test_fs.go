package pathfs

import (
	"github.com/hanwen/go-fuse/v2/fuse"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type testFileSystem struct {
	defaultFileSystem
	root string

	xattrs map[string]map[string][]byte
}

// NewTestFileSystem construct A FileSystem
// that forward most of the requests to native filesystem
// and process Extended attributes requests in case of some filesystems that don't support xattr operations
func NewTestFileSystem(root string) FileSystem {
	if root[0] != '/' {
		panic("not a absolute path")
	}

	s := syscall.Stat_t{}
	err := syscall.Stat(root, &s)
	if err != nil {
		panic(err)
	}

	return &testFileSystem{
		root:   root,
		xattrs: make(map[string]map[string][]byte),
	}
}

func (fs *testFileSystem) absPath(relPath string) string {
	return filepath.Join(fs.root, relPath)
}

func (fs *testFileSystem) GetAttr(ctx *Context, path string, uFh uint32, out *fuse.Attr) fuse.Status {
	st := syscall.Stat_t{}
	err := syscall.Lstat(fs.absPath(path), &st)
	if err != nil {
		return fuse.ToStatus(err)
	}

	out.FromStat(&st)
	return fuse.OK
}

func (fs *testFileSystem) Access(ctx *Context, path string, mask uint32) fuse.Status {
	return fuse.ToStatus(syscall.Access(fs.absPath(path), mask))
}

func (fs *testFileSystem) Mknod(ctx *Context, path string, mode uint32, dev uint32) fuse.Status {
	return fuse.ToStatus(syscall.Mknod(fs.absPath(path), mode, int(dev)))
}

func (fs *testFileSystem) Mkdir(ctx *Context, path string, mode uint32) (code fuse.Status) {
	return fuse.ToStatus(syscall.Mkdir(fs.absPath(path), mode))
}

func (fs *testFileSystem) Unlink(ctx *Context, path string) (code fuse.Status) {
	return fuse.ToStatus(syscall.Unlink(fs.absPath(path)))
}

func (fs *testFileSystem) Rmdir(ctx *Context, path string) (code fuse.Status) {
	return fuse.ToStatus(syscall.Rmdir(fs.absPath(path)))
}

func (fs *testFileSystem) Rename(ctx *Context, path string, newPath string) fuse.Status {
	return fuse.ToStatus(syscall.Rename(fs.absPath(path), fs.absPath(newPath)))
}

func (fs *testFileSystem) Link(ctx *Context, path string, newPath string) fuse.Status {
	return fuse.ToStatus(syscall.Link(fs.absPath(path), fs.absPath(newPath)))
}

func (fs *testFileSystem) Symlink(ctx *Context, path string, target string) fuse.Status {
	return fuse.ToStatus(syscall.Symlink(target, fs.absPath(path)))
}

func (fs *testFileSystem) Readlink(ctx *Context, path string) (target string, code fuse.Status) {
	f, err := os.Readlink(fs.absPath(path))
	return f, fuse.ToStatus(err)
}

func (fs *testFileSystem) Create(ctx *Context, path string, flags uint32, mode uint32) (uFh uint32, forceDIO bool, code fuse.Status) {
	flags = flags &^ syscall.O_APPEND
	fd, err := syscall.Open(fs.absPath(path), int(flags)|os.O_CREATE, mode)
	if err != nil {
		syscall.Close(fd)
		code = fuse.ToStatus(err)
		return
	}
	uFh = uint32(fd)
	return
}

func (fs *testFileSystem) Open(ctx *Context, path string, flags uint32) (uFh uint32, keepCache, forceDIO bool, code fuse.Status) {
	fd, err := syscall.Open(fs.absPath(path), int(flags), 0)
	forceDIO = true
	if err != nil {
		syscall.Close(fd)
		code = fuse.ToStatus(err)
		return
	}
	uFh = uint32(fd)
	return
}

func (fs *testFileSystem) Read(ctx *Context, path string, uFh uint32, dest []byte, off uint64) (result fuse.ReadResult, code fuse.Status) {
	var err error
	var sz int
	var fd int

	if uFh == 0 {
		fd, err = syscall.Open(fs.absPath(path), syscall.O_RDONLY, 0)
		defer syscall.Close(fd)
		if err != nil {
			return nil, fuse.ToStatus(err)
		}
	} else {
		fd = int(uFh)
	}

	sz, err = syscall.Pread(fd, dest, int64(off))

	return fuse.ReadResultData(dest[:sz]), fuse.ToStatus(err)
}

func (fs *testFileSystem) Write(ctx *Context, path string, uFh uint32, data []byte, off uint64) (written uint32, code fuse.Status) {
	var err error
	var sz int
	var fd int

	if uFh == 0 {
		fd, err = syscall.Open(fs.absPath(path), syscall.O_RDONLY, 0)
		defer syscall.Close(fd)
		if err != nil {
			return 0, fuse.ToStatus(err)
		}
	} else {
		fd = int(uFh)
	}

	sz, err = syscall.Pwrite(fd, data, int64(off))

	return uint32(sz), fuse.ToStatus(err)
}

func (fs *testFileSystem) Fsync(ctx *Context, path string, uFh uint32, flags uint32) fuse.Status {
	if uFh != 0 {
		return fuse.ToStatus(syscall.Fsync(int(uFh)))
	} else {
		return fuse.OK
	}
}

func (fs *testFileSystem) Release(ctx *Context, path string, uFh uint32) {
	if uFh != 0 {
		syscall.Close(int(uFh))
	}
}

func (fs *testFileSystem) Chmod(ctx *Context, path string, uFh uint32, mode uint32) fuse.Status {
	var err error
	if uFh != 0 {
		err = syscall.Fchmod(int(uFh), mode)
	} else {
		err = syscall.Chmod(fs.absPath(path), mode)
	}
	return fuse.ToStatus(err)
}

func (fs *testFileSystem) Chown(ctx *Context, path string, uFh uint32, uid uint32, gid uint32) fuse.Status {
	// for chown command can only be used by root, we ignore this operation
	return fuse.ENOSYS
}

func (fs *testFileSystem) Truncate(ctx *Context, path string, uFh uint32, size uint64) fuse.Status {
	var err error
	if uFh != 0 {
		err = syscall.Ftruncate(int(uFh), int64(size))
	} else {
		err = os.Truncate(fs.absPath(path), int64(size))
	}
	return fuse.ToStatus(err)
}

func (fs *testFileSystem) Lsdir(ctx *Context, path string) (stream []fuse.DirEntry, code fuse.Status) {
	f, err := os.Open(fs.absPath(path))
	defer f.Close()
	if err != nil {
		return nil, fuse.ToStatus(err)
	}
	batch := 256
	stream = make([]fuse.DirEntry, 0, batch)
	for {
		infos, err := f.Readdir(batch)
		for i := range infos {
			name := infos[i].Name()
			d := fuse.DirEntry{
				Name: name,
			}
			if s := fuse.ToStatT(infos[i]); s != nil {
				d.Mode = uint32(s.Mode)
				d.Ino = s.Ino
			}
			stream = append(stream, d)
		}
		if len(infos) < batch || err == io.EOF {
			break
		}
		if err != nil {
			code = fuse.ToStatus(err)
			break
		}
	}
	return
}

func (fs *testFileSystem) StatFs(ctx *Context, path string, out *fuse.StatfsOut) fuse.Status {
	s := syscall.Statfs_t{}
	err := syscall.Statfs(fs.absPath(path), &s)
	if err != nil {
		return fuse.ToStatus(err)
	}
	out.FromStatfsT(&s)
	return fuse.OK
}

func (fs *testFileSystem) Utimens(ctx *Context, path string, uFh uint32, atime *time.Time, mtime *time.Time) fuse.Status {
	var err error
	if uFh != 0 {
		err = fUtimes(int(uFh), atime, mtime)
	} else {
		err = utimes(fs.absPath(path), atime, mtime)
	}
	return fuse.ToStatus(err)
}

func (fs *testFileSystem) SetXAttr(ctx *Context, path string, attr string, data []byte, flags uint32) fuse.Status {
	var m map[string][]byte
	var ok bool
	if m, ok = fs.xattrs[path]; !ok {
		m = make(map[string][]byte)
		fs.xattrs[path] = m
	}
	m[attr] = data
	return fuse.OK
}

func (fs *testFileSystem) GetXAttr(ctx *Context, path string, attr string) (data []byte, code fuse.Status) {
	var m map[string][]byte
	var ok bool
	if m, ok = fs.xattrs[path]; !ok {
		return nil, fuse.ENODATA
	}
	if data, ok = m[attr]; !ok {
		return nil, fuse.ENODATA
	}
	return data, fuse.OK
}

func (fs *testFileSystem) ListXAttr(ctx *Context, path string) (attrs []string, code fuse.Status) {
	var m map[string][]byte
	var ok bool
	if m, ok = fs.xattrs[path]; !ok {
		return nil, fuse.ENODATA
	}
	for k := range m {
		attrs = append(attrs, k)
	}
	return attrs, fuse.OK
}

func (fs *testFileSystem) RemoveXAttr(ctx *Context, path string, attr string) fuse.Status {
	var m map[string][]byte
	var ok bool
	if m, ok = fs.xattrs[path]; !ok {
		return fuse.ENODATA
	}
	delete(m, attr)
	if len(m) == 0 {
		delete(fs.xattrs, path)
	}
	return fuse.OK
}
