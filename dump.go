package pathfs

import (
	"errors"
	"github.com/hanwen/go-fuse/v2/fuse"
	"io"
	"sort"
	"time"
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
	NodeCountHigh int
	Files         []*DumpFileEntry
	FreeFiles     []uint32
}

type DumpInode struct {
	Ino         uint64
	Revision    uint32
	LookupCount uint32
	Parents     []DumpParentEntry
}

type DumpParentEntry struct {
	Name string
	Node uint64
}

// 流式编码接口
type InodeIterator interface {
	// 每调用一次，返回1个inode的编码数据
	// 当返回EOF时，说明inodes已经编码完毕
	// 会将inode转化为dumpinode后编码
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
	parents := node.parents
	n := parents.count()
	var parentEntries []DumpParentEntry

	if n > 0 {
		parentEntries = make([]DumpParentEntry, n)
		times := make([]int64, n) // for sorting

		// insert newest parent into slice
		times[0] = time.Now().Unix()
		parentEntries[0] = DumpParentEntry{
			Name: parents.newest.name,
		}
		if parents.newest.node != nil {
			parentEntries[0].Node = parents.newest.node.ino
		}

		i := 1
		for e, t := range parents.other {
			parentEntries[i].Node = e.node.ino
			parentEntries[i].Name = e.name
			times[i] = t.Unix()
			i++
		}
		sort.Slice(parentEntries, func(i, j int) bool {
			return times[i] > times[j]
		})
	}

	data = &DumpInode{
		node.ino,
		node.revision,
		node.lookupCount,
		parentEntries,
	}
	s.off++
	return data, nil
}

// 流式编码接口
type InodeFiller interface {
	// 每调用一次，内部解码传入的[]byte
	// 解码为dumpinode后转为inode
	AddInode(*DumpInode) error
	// 当所有inode都被解码后，该方法会给inode填充children
	Finished() error
}

type InodeRestorer struct {
	bridge *rawBridge
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
	} else {
		if ret.children == nil {
			ret.children = make(map[string]*inode)
		}
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

	dumpParents := dumpInode.Parents
	n := len(dumpParents)

	if n > 0 {
		var parInode *inode

		// process newest parent
		parInode = s.getDirInode(dumpParents[0].Node)
		parInode.children[dumpParents[0].Name] = curInode
		curInode.parents.newest = parentEntry{
			name: dumpParents[0].Name,
			node: parInode,
		}

		// process other parents
		if n > 1 {
			if curInode.parents.other == nil {
				curInode.parents.other = make(map[parentEntry]time.Time)
			}
			t := time.Now().Unix()
			for i := 1; i < n; i++ {
				parInode = s.getDirInode(dumpParents[i].Node)
				parInode.children[dumpParents[i].Name] = curInode
				// construct other parents' time according to the order in slice
				curInode.parents.other[parentEntry{name: dumpParents[i].Name, node: parInode}] = time.Unix(t-int64(i), 0)
			}
		}
	}

	return nil
}

// restore root inode
func (s *InodeRestorer) Finished() error {
	var found bool
	s.bridge.root, found = s.bridge.nodes[1]
	if !found {
		return errors.New("root inode not found")
	}
	s.bridge.root.parents.other = nil
	return nil
}
