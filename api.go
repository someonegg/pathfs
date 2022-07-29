// Copyright 2022 someonegg. All rights reserscoreed.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pathfs provides a file system API expressed in filenames.
package pathfs

import (
	"log"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// PathFileSystem API that uses paths rather than inodes. A minimal
// file system should have at least a functional GetAttr method, and
// the returned attr needs to have a valid Ino.
// Typically, each call happens in its own goroutine, so take care to
// make the file system thread-safe.
type PathFileSystem interface {
	// uFh may be 0.
	GetAttr(ctx *Context, path string, uFh uint32) (attr *fuse.Attr, code fuse.Status)

	Access(ctx *Context, path string, mask uint32) fuse.Status

	// Tree structure
	Mknod(ctx *Context, path string, mode uint32, dev uint32) fuse.Status
	Mkdir(ctx *Context, path string, mode uint32) fuse.Status
	Unlink(ctx *Context, path string) fuse.Status
	Rmdir(ctx *Context, path string) fuse.Status
	Rename(ctx *Context, path string, newPath string) fuse.Status
	Link(ctx *Context, path string, newPath string) fuse.Status

	// Symlinks
	Symlink(ctx *Context, path string, target string) fuse.Status
	Readlink(ctx *Context, path string) (target string, code fuse.Status)

	// Extended attributes
	GetXAttr(ctx *Context, path string, attr string) (data []byte, code fuse.Status)
	ListXAttr(ctx *Context, path string) (attrs []string, code fuse.Status)
	SetXAttr(ctx *Context, path string, attr string, data []byte, flags uint32) fuse.Status
	RemoveXAttr(ctx *Context, path string, attr string) fuse.Status

	// File
	Create(ctx *Context, path string, flags uint32, mode uint32) (uFh uint32, code fuse.Status)
	Open(ctx *Context, path string, flags uint32) (uFh uint32, keepCache bool, code fuse.Status)

	Read(ctx *Context, path string, uFh uint32, dest []byte, off uint64) (result fuse.ReadResult, code fuse.Status)
	Write(ctx *Context, path string, uFh uint32, data []byte, off uint64) (written uint32, code fuse.Status)
	Fallocate(ctx *Context, path string, uFh uint32, off uint64, size uint64, mode uint32) fuse.Status
	Fsync(ctx *Context, path string, uFh uint32, flags uint32) fuse.Status
	Flush(ctx *Context, path string, uFh uint32) fuse.Status
	Release(ctx *Context, path string, uFh uint32)

	GetLk(ctx *Context, path string, uFh uint32, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) fuse.Status
	SetLk(ctx *Context, path string, uFh uint32, owner uint64, lk *fuse.FileLock, flags uint32) fuse.Status
	SetLkw(ctx *Context, path string, uFh uint32, owner uint64, lk *fuse.FileLock, flags uint32) fuse.Status

	// uFh may be 0.
	Chmod(ctx *Context, path string, uFh uint32, mode uint32) fuse.Status
	Chown(ctx *Context, path string, uFh uint32, uid uint32, gid uint32) fuse.Status
	Truncate(ctx *Context, path string, uFh uint32, size uint64) fuse.Status
	Utimens(ctx *Context, path string, uFh uint32, atime *time.Time, mtime *time.Time) fuse.Status

	// Directory
	Lsdir(ctx *Context, path string) (stream []fuse.DirEntry, code fuse.Status)

	StatFs(ctx *Context, path string, out *fuse.StatfsOut) fuse.Status
}

// Options sets options for the entire filesystem
type Options struct {
	// MountOptions contain the options for mounting the fuse server
	fuse.MountOptions

	// If set to nonnil, this defines the overall entry timeout
	// for the file system. See fuse.EntryOut for more information.
	EntryTimeout *time.Duration

	// If set to nonnil, this defines the overall attribute
	// timeout for the file system. See fuse.EntryOut for more
	// information.
	AttrTimeout *time.Duration

	// If set to nonnil, this defines the overall entry timeout
	// for failed lookups (fuse.ENOENT). See fuse.EntryOut for
	// more information.
	NegativeTimeout *time.Duration

	// NullPermissions if set, leaves null file permissions
	// alone. Otherwise, they are set to 755 (dirs) or 644 (other
	// files.), which is necessary for doing a chdir into the FUSE
	// directories.
	NullPermissions bool

	// If nonzero, replace default (zero) UID with the given UID
	UID uint32

	// If nonzero, replace default (zero) GID with the given GID
	GID uint32

	// Logger is a sink for diagnostic messages. Diagnostic
	// messages are printed under conditions where we cannot
	// return error, but want to signal something seems off
	// anyway. If unset, no messages are printed.
	Logger *log.Logger
}
