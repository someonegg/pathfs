package pathfs

import (
	"container/list"
	"fmt"
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
		{"l3_d1", 8, true},
		{"l3_r1", 9, false},
	}

	for i := 2; i <= 5; i++ {
		b.addChild(b.root, files[i].name, files[i].ino, files[i].isDir)
	}

	inode2 := b.inode(2)
	for i := 6; i <= 7; i++ {
		b.addChild(inode2, files[i].name, files[i].ino, files[i].isDir)
	}

	inode6 := b.inode(6)
	for i := 8; i <= 9; i++ {
		b.addChild(inode6, files[i].name, files[i].ino, files[i].isDir)
	}

	// add symlink
	b.addChild(b.inode(3), files[6].name, 6, files[6].isDir)
	b.addChild(b.inode(4), files[7].name, 7, files[7].isDir)
	b.addChild(b.inode(8), files[5].name, 5, files[5].isDir)

	// let inode 9 be orphan
	b.rmChild(inode6, files[9].name)
}

// print the directory tree in the format like "tree" command
func printDirTree(root *inode) {
	var lastInLevel []bool

	printBranch := func(level int, last bool, node *inode) {
		prefix := make([]rune, level*3)
		for i := 0; i < level; i++ {
			if lastInLevel[i] {
				prefix[3*i] = ' '
			} else {
				prefix[3*i] = '│'
			}
			prefix[3*i+1] = ' '
			prefix[3*i+2] = ' '
		}
		var symbol string
		if last {
			symbol = "└─ "
		} else {
			symbol = "├─ "
		}
		fmt.Printf("%s%s%s(%d)\n", string(prefix), symbol, node.parents.newest.name, node.ino)
	}

	fileCnt, dirCnt := 1, 1
	var walk func(node *inode, level int)
	walk = func(node *inode, level int) {
		if level >= len(lastInLevel) {
			lastInLevel = append(lastInLevel, false)
		}
		last := false
		cnt := 0
		for _, child := range node.children {
			cnt++
			fileCnt++
			last = cnt == len(node.children)
			lastInLevel[level] = last
			printBranch(level, last, child)
			// is dir
			if child.children != nil {
				dirCnt++
				walk(child, level+1)
			}
		}

	}

	fmt.Printf(".(%d)\n", root.ino)
	walk(root, 0)

}

// print some statistic data
func printStatistic(bridge *rawBridge) {
	type queueElement struct {
		node  *inode
		level int
	}
	q := list.New()
	q.PushBack(&queueElement{bridge.root, 0})
	level := -1
	var e *list.Element
	var qe *queueElement
	var s strings.Builder
	var node *inode
	fileCnt, dirCnt := 0, 0

	// bfs
	for q.Len() > 0 {
		fileCnt++
		e = q.Front()
		qe = e.Value.(*queueElement)
		if qe.level > level {
			level++
			fmt.Printf("\nlevel %d:", level)
		}
		node = qe.node
		s.Reset()
		// "ino(lookupCount, parentCount):filename"
		fmt.Printf(" %d(%d,%d):%s", node.ino, node.lookupCount, node.parents.count(), node.parents.newest.name)

		if node.children != nil {
			dirCnt++
			for _, child := range node.children {
				if child.parents.newest.node == qe.node {
					q.PushBack(&queueElement{child, qe.level + 1})
				}
			}
		}
		q.Remove(e)
	}
	fmt.Printf("\n\nfileCnt:%d, dirCnt:%d, orphanCnt:%d\n\n", fileCnt, dirCnt, len(bridge.nodes)-fileCnt)

}

func TestDump(t *testing.T) {
	senderBridge := newBridge()
	constructDirTree(senderBridge)
	printDirTree(senderBridge.root)
	printStatistic(senderBridge)

	// simulate IPC
	inodeChan := make(chan *DumpInode)
	finish := make(chan struct{})

	dumpB, iter, err := senderBridge.Dump()
	if err != nil {
		panic(err)
	}

	go func(dumpB *DumpRawBridge, d chan *DumpInode, f chan struct{}) {
		receiverBridge := &rawBridge{}
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
		printStatistic(receiverBridge)

		finish <- struct{}{}
	}(dumpB, inodeChan, finish)

	for i, err := iter.Next(); err == nil; i, err = iter.Next() {
		inodeChan <- i
	}

	close(inodeChan)
	<-finish
	close(finish)

}
