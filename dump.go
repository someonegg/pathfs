package pathfs

import (
	"errors"
	"fmt"
	"io"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type DumpFileEntry struct {
	Opener fuse.Owner
	Path   string

	// file
	UFh uint32

	// dir
	Stream []fuse.DirEntry
}

type DumpRawBridge struct {
	NodeCount int
	Files     []*DumpFileEntry
	FreeFiles []uint32
}

type DumpInode struct {
	Ino         uint64
	Revision    uint32
	LookupCount uint32
	Parents     []DumpParentEntry
	IsDir       bool
}

type DumpParentEntry struct {
	Name string
	Node uint64
}

type InodeIterator interface {
	// returning io.EOF error means that all inodes has been dumped
	Next() (data *DumpInode, err error)
}

// Copier transfers bridge bookkeeping, not file contents or backend resources.
// Stop and drain requests before Dump or Restore. Dump captures an independent
// snapshot; its iterator may be consumed after source requests resume. Create the
// destination with NewPathFS to supply its backend and options, then call Restore,
// AddInode for every record, and Finished before serving requests. If any step
// fails, discard the destination. Backend handles and the FUSE connection must be
// preserved or transferred separately by the caller.
type Copier interface {
	Dump() (data *DumpRawBridge, iterator InodeIterator, err error)
	Restore(data *DumpRawBridge) (filler InodeFiller, err error)
}

type InodeDumper struct {
	inodes []*DumpInode
	off    int
}

func NewInodeDumper(inodesMap map[uint64]*inode) *InodeDumper {
	inodes := make([]*DumpInode, len(inodesMap))
	i := 0
	for _, v := range inodesMap {
		inodes[i] = &DumpInode{
			Ino: v.ino, Revision: v.revision, LookupCount: v.lookupCount,
			Parents: v.parents.Dump(), IsDir: v.isDir(),
		}
		i++
	}

	return &InodeDumper{
		inodes: inodes,
		off:    0,
	}
}

func (s *InodeDumper) Next() (data *DumpInode, err error) {
	if s.off >= len(s.inodes) {
		return nil, io.EOF
	}
	data = s.inodes[s.off]
	s.off++
	return data, nil
}

type InodeFiller interface {
	AddInode(*DumpInode) error
	// Finished validates completeness and installs the restored root.
	Finished() error
}

type InodeRestorer struct {
	nodeCount int
	seen      map[uint64]bool
	bridge    *rawBridge
}

// if not found in bridge's inodes, insert a new one and return it
// otherwise just return the existed one
func (s *InodeRestorer) getDirInode(ino uint64) *inode {
	inodes := s.bridge.nodes
	var ret *inode
	var found bool
	if ret, found = inodes[ino]; !found {
		ret = &inode{
			ino:      ino,
			children: make(map[string]*inode),
		}
		inodes[ino] = ret
	}
	return ret
}

func (s *InodeRestorer) AddInode(dumpInode *DumpInode) error {
	if dumpInode == nil {
		return errors.New("nil inode record")
	}
	if s.seen[dumpInode.Ino] {
		return fmt.Errorf("duplicate inode %d", dumpInode.Ino)
	}
	if s.seen == nil {
		s.seen = make(map[uint64]bool)
	}
	s.seen[dumpInode.Ino] = true

	inodes := s.bridge.nodes
	var curInode *inode
	var found bool
	if curInode, found = inodes[dumpInode.Ino]; !found {
		curInode = &inode{
			ino: dumpInode.Ino,
		}
		inodes[dumpInode.Ino] = curInode
	}

	// restore other fields
	curInode.revision = dumpInode.Revision
	curInode.lookupCount = dumpInode.LookupCount
	if dumpInode.IsDir && curInode.children == nil {
		curInode.children = make(map[string]*inode)
	}

	var parInode *inode
	for _, p := range dumpInode.Parents {
		parInode = s.getDirInode(p.Node)
		parInode.children[p.Name] = curInode
		curInode.parents.add(parentEntry{name: p.Name, node: parInode})
	}

	return nil
}

// Finished verifies that every inode, including referenced parents, was restored.
func (s *InodeRestorer) Finished() error {
	bridge := s.bridge
	root := bridge.nodes[1]
	if !s.seen[1] || root == nil || !root.isDir() {
		return errors.New("root directory inode not found")
	}
	if len(s.seen) != s.nodeCount || len(bridge.nodes) != s.nodeCount {
		return fmt.Errorf("expected %d inodes, got %d records and %d referenced inodes", s.nodeCount, len(s.seen), len(bridge.nodes))
	}
	bridge.root = root
	bridge.nodeCountHigh = len(bridge.nodes)
	return nil
}
