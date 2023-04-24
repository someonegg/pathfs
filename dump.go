package pathfs

import (
	"errors"
	"fmt"
	"github.com/hanwen/go-fuse/v2/fuse"
	"io"
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

type Copier interface {
	Dump() (data *DumpRawBridge, iterator InodeIterator, err error)
	Restore(data *DumpRawBridge) (filler InodeFiller, err error)
}

type InodeDumper struct {
	inodes []*inode // flatted inodesMap
	off    int
}

func NewInodeDumper(inodesMap map[uint64]*inode) *InodeDumper {
	inodes := make([]*inode, len(inodesMap))
	i := 0
	for _, v := range inodesMap {
		inodes[i] = v
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
	node := s.inodes[s.off]

	data = &DumpInode{
		node.ino,
		node.revision,
		node.lookupCount,
		node.parents.Dump(),
		node.isDir(),
	}
	s.off++
	return data, nil
}

type InodeFiller interface {
	AddInode(*DumpInode) error
	// update bridge's root, may be removed
	Finished() error
}

type InodeRestorer struct {
	nodeCount    int
	addNodeCount int
	bridge       *rawBridge
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

	dumpParents := dumpInode.Parents
	n := len(dumpParents)
	var parInode *inode

	for i := 0; i < n; i++ {
		parInode = s.getDirInode(dumpParents[i].Node)
		parInode.children[dumpParents[i].Name] = curInode
		curInode.parents.add(parentEntry{name: dumpParents[i].Name, node: parInode})
	}

	s.addNodeCount++

	return nil
}

// Finished restore root inode and verify inode's count
func (s *InodeRestorer) Finished() error {
	bridge := s.bridge

	if root, found := s.bridge.nodes[1]; !found {
		return errors.New("root inode not found")
	} else {
		bridge.root = root
	}

	if s.addNodeCount < s.nodeCount {
		for _, n := range s.bridge.nodes {
			if n.revision == 0 {
				bridge.logf("warning: inode %d is lost.\n", n.ino)
			}
		}
		return fmt.Errorf("expected %d inodes, but only got %d inodes", s.nodeCount, s.addNodeCount)
	}

	s.bridge.nodeCountHigh = len(s.bridge.nodes)

	return nil
}
