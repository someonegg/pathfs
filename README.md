# pathfs

A Go library for implementing FUSE filesystems with path-based callbacks, built
on go-fuse v2. The bridge translates kernel inode requests into relative paths
and manages lookup references, hard links, file handles, and directory reads.

[API reference](https://pkg.go.dev/github.com/someonegg/pathfs)

## Design

- Bridge `fuse.RawFileSystem` directly to path callbacks, avoiding an intermediate
  nodefs layer. Paths let backends implement client-side routing and subtree
  migration controls; those policies belong to the backend.
- Reuse stable backend inode IDs. Track lookup references and parent/child
  relationships to reconstruct paths and retain ancestors while needed.
- For hard links, retain all known parents and prefer the most recently added
  relationship when reconstructing a path, falling back when it is removed.
- Use integer backend handles instead of requiring backend file objects.
  The bridge keeps bookkeeping; the backend owns file resources and I/O.

## Getting started

```sh
go get github.com/someonegg/pathfs
```

Mounting requires a working FUSE installation (Linux or macOS), permission to
mount, and an existing mount directory. Embed `pathfs.DefaultFileSystem()` to
implement only the callbacks you need; most defaults return `ENOSYS`.
This minimal backend exposes an empty directory:

```go
package main

import (
    "log"
    "os"

    "github.com/hanwen/go-fuse/v2/fuse"
    "github.com/someonegg/pathfs"
)

type emptyFS struct{ pathfs.FileSystem }

func (*emptyFS) GetAttr(_ *pathfs.Context, path string, _ uint32, out *fuse.Attr) fuse.Status {
    if path != "" { return fuse.ENOENT }
    *out = fuse.Attr{Ino: 1, Mode: fuse.S_IFDIR | 0755, Nlink: 2}
    return fuse.OK
}

func (*emptyFS) Lsdir(_ *pathfs.Context, path string) ([]fuse.DirEntry, fuse.Status) {
    if path != "" { return nil, fuse.ENOENT }
    return nil, fuse.OK
}

func main() {
    if len(os.Args) != 2 { log.Fatal("usage: emptyfs MOUNTPOINT") }
    backend := &emptyFS{FileSystem: pathfs.DefaultFileSystem()}
    server, err := pathfs.Mount(os.Args[1], backend, nil, nil)
    if err != nil { log.Fatal(err) }
    server.Wait() // Unmount from another terminal to stop serving.
}
```

For custom server setup, use `NewPathFS` with `fuse.NewServer`.
`NewTestFileSystem` is a local test backend with simulated, path-keyed xattrs;
use it for testing, not as a production loopback filesystem or sandbox.

## Callback contract

- Paths are relative; the root is `""`. `GetAttr` must return stable, unique inode
  numbers, with inode 1 reserved for the root.
- Callbacks run concurrently. Make your backend thread-safe and honor request
  cancellation by returning `EINTR`.
- Open/create return `uFh` (zero means no handle). Handle operations receive the
  opening path, which may become stale after rename/unlink; use the handle for
  file identity. `Context.Opener` identifies the original opener.
- `Options` controls cache timeouts; nil uses one-second attribute and entry
  timeouts. Open/create also control direct I/O and file data caching.

Directory listings are held in memory per open handle.
`Lseek`, `CopyFileRange`, directory fsync, and rename with nonzero flags return
`ENOSYS`.

## State transfer

`NewPathFS` also returns an implementation of `Copier`. Stop and drain requests
before `Dump`. On a destination created with `NewPathFS`, call `Restore`, feed
all inode records to `AddInode`, and call `Finished` before serving. Check every
error and discard a destination whose restore fails.

Snapshots contain bridge bookkeeping only. File contents, backend resources,
options, and the FUSE connection require separate handling for a hot upgrade.

## Tests

```sh
go test ./...
go test -race ./...
go test -tags=integration ./...  # Requires working FUSE and mount permission.
```

Mount tests use isolated temporary directories and unmount before cleanup.
