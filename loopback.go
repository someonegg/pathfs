package pathfs

import (
	"github.com/hanwen/go-fuse/v2/fuse"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type loopbackFileSystem struct {
	defaultFileSystem
	root string
}

// NewLoopbackFileSystem construct A FileSystem that forward requests to native filesystem
// for the purpose of testing without having to build a complete filesystem
func NewLoopbackFileSystem(root string) FileSystem {
	if root[0] != '/' {
		panic("not a absolute path")
	}

	s := syscall.Stat_t{}
	err := syscall.Stat(root, &s)
	if err != nil {
		panic(err)
	}

	return &loopbackFileSystem{
		root: root,
	}
}

func (fs *loopbackFileSystem) absPath(relPath string) string {
	return filepath.Join(relPath)
}

func (fs *loopbackFileSystem) GetAttr(ctx *Context, path string, uFh uint32, out *fuse.Attr) fuse.Status {
	var err error = nil
	st := syscall.Stat_t{}
	if uFh > 3 {
		err = syscall.Fstat(int(uFh), &st)
	} else {
		absPath := fs.absPath(path)
		err = syscall.Stat(absPath, &st)
	}

	if err != nil {
		return fuse.ToStatus(err)
	}
	out = &fuse.Attr{}
	out.FromStat(&st)
	return fuse.OK
}

func (fs *loopbackFileSystem) Access(ctx *Context, path string, mask uint32) fuse.Status {
	return fuse.ToStatus(syscall.Access(fs.absPath(path), mask))
}

func (fs *loopbackFileSystem) Mknod(ctx *Context, path string, mode uint32, dev uint32) fuse.Status {
	return fuse.ToStatus(syscall.Mknod(fs.absPath(path), mode, int(dev)))
}

func (fs *loopbackFileSystem) Mkdir(ctx *Context, path string, mode uint32) (code fuse.Status) {
	return fuse.ToStatus(os.Mkdir(fs.absPath(path), os.FileMode(mode)))
}

func (fs *loopbackFileSystem) Unlink(ctx *Context, path string) (code fuse.Status) {
	return fuse.ToStatus(syscall.Unlink(fs.absPath(path)))
}

func (fs *loopbackFileSystem) Rmdir(ctx *Context, path string) (code fuse.Status) {
	return fuse.ToStatus(syscall.Rmdir(fs.absPath(path)))
}

func (fs *loopbackFileSystem) Rename(ctx *Context, path string, newPath string) fuse.Status {
	path = fs.absPath(path)
	newPath = fs.absPath(newPath)
	err := os.Rename(path, newPath)
	return fuse.ToStatus(err)
}

func (fs *loopbackFileSystem) Link(ctx *Context, path string, newPath string) fuse.Status {
	return fuse.ToStatus(os.Link(fs.absPath(path), fs.absPath(newPath)))
}

func (fs *loopbackFileSystem) Symlink(ctx *Context, path string, target string) fuse.Status {
	return fuse.ToStatus(os.Symlink(fs.absPath(path), fs.absPath(target)))
}

func (fs *loopbackFileSystem) Readlink(ctx *Context, path string) (target string, code fuse.Status) {
	f, err := os.Readlink(fs.absPath(path))
	return f, fuse.ToStatus(err)
}

func (fs *loopbackFileSystem) Create(ctx *Context, path string, flags uint32, mode uint32) (uFh uint32, forceDIO bool, code fuse.Status) {
	fd, err := syscall.Open(fs.absPath(path), int(flags)|os.O_CREATE, mode)
	if err != nil {
		return 0, false, fuse.ToStatus(err)
	}
	uFh = uint32(fd)
	return
}

func (fs *loopbackFileSystem) Open(ctx *Context, path string, flags uint32) (uFh uint32, keepCache, forceDIO bool, code fuse.Status) {
	fd, err := syscall.Open(fs.absPath(path), int(flags), 0)
	forceDIO = flags&syscall.O_DIRECT != 0
	if err != nil {
		code = fuse.ToStatus(err)
		return
	}
	uFh = uint32(fd)
	return
}

func (fs *loopbackFileSystem) Read(ctx *Context, path string, uFh uint32, dest []byte, off uint64) (result fuse.ReadResult, code fuse.Status) {
	var err error
	if uFh > 3 {
		_, err = syscall.Pread(int(uFh), dest, int64(off))
	} else {
		f, err := os.Open(path)
		defer f.Close()
		if err != nil {
			return nil, fuse.ToStatus(err)
		}
		_, err = f.ReadAt(dest, int64(off))
	}

	if err != nil && err != io.EOF {
		return nil, fuse.ToStatus(err)
	}

	return fuse.ReadResultData(dest), fuse.OK
}

func (fs *loopbackFileSystem) Write(ctx *Context, path string, uFh uint32, data []byte, off uint64) (written uint32, code fuse.Status) {
	var err error
	var n int
	if uFh > 3 {
		n, err = syscall.Pwrite(int(uFh), data, int64(off))
	} else {
		f, e := os.Open(fs.absPath(path))
		defer f.Close()
		if e != nil {
			return 0, fuse.ToStatus(e)
		}
		n, err = f.WriteAt(data, int64(off))
	}

	return uint32(n), fuse.ToStatus(err)
}

func (fs *loopbackFileSystem) Fallocate(ctx *Context, path string, uFh uint32, off uint64, size uint64, mode uint32) fuse.Status {
	var err error
	if uFh > 3 {
		err = syscall.Fallocate(int(uFh), mode, int64(off), int64(size))
	} else {
		fd, e := syscall.Open(fs.absPath(path), 0, 0)
		defer syscall.Close(fd)
		if e != nil {
			return fuse.ToStatus(e)
		}
		err = syscall.Fallocate(fd, mode, int64(off), int64(size))
	}
	return fuse.ToStatus(err)
}

func (fs *loopbackFileSystem) Fsync(ctx *Context, path string, uFh uint32, flags uint32) fuse.Status {
	if uFh > 3 {
		return fuse.ToStatus(syscall.Fsync(int(uFh)))
	} else {
		return fuse.OK
	}
}

func (fs *loopbackFileSystem) Release(ctx *Context, path string, uFh uint32) {
	if uFh > 3 {
		fuse.ToStatus(syscall.Close(int(uFh)))
	}
}

func (fs *loopbackFileSystem) Chmod(ctx *Context, path string, uFh uint32, mode uint32) fuse.Status {
	var err error
	if uFh > 3 {
		err = syscall.Fchmod(int(uFh), mode)
	} else {
		err = syscall.Chmod(fs.absPath(path), mode)
	}
	return fuse.ToStatus(err)
}

func (fs *loopbackFileSystem) Chown(ctx *Context, path string, uFh uint32, uid uint32, gid uint32) fuse.Status {
	var err error
	if uFh > 3 {
		err = syscall.Fchown(int(uFh), int(uid), int(gid))
	} else {
		err = syscall.Chown(path, int(uid), int(gid))
	}
	return fuse.ToStatus(err)
}

func (fs *loopbackFileSystem) Truncate(ctx *Context, path string, uFh uint32, size uint64) fuse.Status {
	var err error
	if uFh > 3 {
		err = syscall.Ftruncate(int(uFh), int64(size))
	} else {
		err = os.Truncate(path, int64(size))
	}
	return fuse.ToStatus(err)
}

func (fs *loopbackFileSystem) Utimens(ctx *Context, path string, uFh uint32, atime *time.Time, mtime *time.Time) fuse.Status {
	var err error
	timevals := []syscall.Timeval{
		{Sec: atime.Unix(), Usec: int64(atime.Nanosecond())},
		{Sec: mtime.Unix(), Usec: int64(mtime.Nanosecond())},
	}
	if uFh > 3 {
		err = syscall.Futimes(int(uFh), timevals)
	} else {
		err = syscall.Utimes(path, timevals)
	}
	return fuse.ToStatus(err)
}

func (fs *loopbackFileSystem) Lsdir(ctx *Context, path string) (stream []fuse.DirEntry, code fuse.Status) {
	f, err := os.Open(fs.absPath(path))
	if err != nil {
		return nil, fuse.ToStatus(err)
	}
	batch := 512
	stream = make([]fuse.DirEntry, 16)
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
			} else {
				log.Printf("ReadDir entry %q for %q has no stat info", name, path)
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
	f.Close()

	return stream, fuse.OK
}

func (fs *loopbackFileSystem) StatFs(ctx *Context, path string, out *fuse.StatfsOut) fuse.Status {
	s := syscall.Statfs_t{}
	err := syscall.Statfs(fs.absPath(path), &s)
	if err != nil {
		return fuse.ToStatus(err)
	}
	out = &fuse.StatfsOut{}
	out.FromStatfsT(&s)
	return fuse.OK
}
