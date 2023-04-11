package pathfs

import (
	"container/list"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func newBridge() *rawBridge {
	b := &rawBridge{
		fs:            DefaultFileSystem(),
		root:          newInode(1, true),
		nodeCountHigh: 1,
	}

	b.nodes = map[uint64]*inode{1: b.root}
	b.root.lookupCount = 1
	b.nodeCountHigh = 1
	b.files = []*fileEntry{{}}

	return b
}

type simpleFileInfo struct {
	name  string
	ino   uint64
	isDir bool
}

func constructDirTree(b *rawBridge) {
	files := []simpleFileInfo{
		{}, {},
		{"l1_d1", 2, true},
		{"l1_d2", 3, true},
		{"l1_d3", 4, true},
		{"l1_r1", 5, false},
		{"l2_d1", 6, true},
		{"l2_r1", 7, false},
	}

	for i := 2; i <= 5; i++ {
		b.addChild(b.root, files[i].name, files[i].ino, files[i].isDir)
	}

	inode2 := b.inode(2)
	for i := 6; i <= 7; i++ {
		b.addChild(inode2, files[i].name, files[i].ino, files[i].isDir)
	}

	// add symlink
	b.addChild(b.inode(3), files[6].name, 6, files[6].isDir)
	b.addChild(b.inode(4), files[7].name, 7, files[7].isDir)

}

// print the directory tree in the format like
// level 0:1(0)
// level 1:4(1) 5(1) 2(1) 3(1)
// level 2:7(4,2) 6(3,2)
func printDirTree(root *inode) {
	type queueElement struct {
		node  *inode
		level int
	}
	q := list.New()
	q.PushBack(&queueElement{root, 0})
	level := -1
	var e *list.Element
	var qe *queueElement
	var parents inodeParents
	var parentStr strings.Builder

	// bfs
	for q.Len() > 0 {
		e = q.Front()
		qe = e.Value.(*queueElement)
		if qe.level > level {
			level++
			fmt.Printf("\n level %d:", level)
		}
		// process parents
		parents = qe.node.parents
		parentStr.Reset()
		if parents.newest.node != nil {
			parentStr.WriteString(strconv.FormatUint(parents.newest.node.ino, 10))
		} else {
			parentStr.WriteString("0")
		}
		for p := range parents.other {
			parentStr.WriteByte(',')
			parentStr.WriteString(strconv.FormatUint(p.node.ino, 10))
		}
		// "ino(parents' ino):filename"
		fmt.Printf("%d(%s):%s ", qe.node.ino, parentStr.String(), parents.newest.name)

		for _, child := range qe.node.children {
			if child.parents.newest.node == qe.node {
				q.PushBack(&queueElement{child, qe.level + 1})
			}
		}
		q.Remove(e)
	}
	print("\n")
}

func TestSerialization(t *testing.T) {
	senderBridge := newBridge()
	constructDirTree(senderBridge)
	printDirTree(senderBridge.root)

	// Output:
	// level 0:1(0)
	// level 1:4(1) 5(1) 2(1) 3(1)
	// level 2:7(4,2) 6(3,2)

	// simulate IPC
	inodeChan := make(chan *DumpInode)
	finish := make(chan struct{})

	dumpB, iter, err := senderBridge.Dump()
	if err != nil {
		panic(err)
	}

	go func(dumpB *DumpRawBridge, d chan *DumpInode, f chan struct{}) {
		receiverBridge := &rawBridge{}
		receiverBridge.nodes = map[uint64]*inode{}
		filler, err := receiverBridge.Restore(dumpB)
		if err != nil {
			panic(err)
		}
		for dumpInode, ok := <-d; ok; dumpInode, ok = <-d {
			err = filler.AddInode(dumpInode)
			if err != nil {
				panic(err)
			}
		}
		err = filler.Finished()
		if err != nil {
			panic(err)
		}
		printDirTree(receiverBridge.root)

		finish <- struct{}{}
	}(dumpB, inodeChan, finish)

	for i, err := iter.Next(); err == nil; i, err = iter.Next() {
		inodeChan <- i
	}

	close(inodeChan)
	<-finish

}
