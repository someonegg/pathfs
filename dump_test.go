package pathfs

import (
	"fmt"
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
		{"l4_r1", 10, false},
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

	b.addChild(b.inode(8), files[10].name, 10, files[10].isDir)

	// add symlink
	b.addChild(b.inode(3), files[7].name, 7, files[7].isDir)
	b.addChild(b.inode(4), files[7].name, 7, files[7].isDir)
	b.addChild(b.inode(6), files[7].name, 7, files[7].isDir)
	b.addChild(b.inode(8), files[7].name, 7, files[7].isDir)
	b.addChild(b.inode(8), files[5].name, 5, files[5].isDir)

	// remove inode 5 from root, but don't forget it
	b.rmChild(b.root, files[5].name)

	// remove inode 9
	b.rmChild(inode6, files[9].name)
	b.removeRef(b.inode(9), 1)

	// let inode 10 be orphan
	b.rmChild(b.inode(8), files[10].name)

}

func sameParentEntry(e1, e2 parentEntry) bool {
	if e1.name != e2.name {
		return false
	}
	if e1.node != nil && e2.node != nil {
		return e1.node.ino == e2.node.ino
	}
	return e1.node == e2.node
}

// failed if new inode isn't exactly the same with old inode
func assertSameInode(t *testing.T, old, new *inode) {
	if new == nil {
		t.Errorf("want inode %d, have nil", old.ino)
		return
	}
	if old.ino != new.ino {
		t.Errorf("want ino %d, have %d", old.ino, new.ino)
		return
	}
	prefix := fmt.Sprintf("inode %d want", old.ino)
	if old.lookupCount != new.lookupCount {
		t.Errorf("%s lookupCount %d, have %d", prefix, old.lookupCount, new.lookupCount)
	}
	if old.revision != new.revision {
		t.Errorf("%s revivion %d, have %d", prefix, old.revision, new.revision)
	}

	// check if two inodes have same children
	if len(old.children) != len(new.children) {
		t.Errorf("%s %d children, have %d", prefix, len(old.children), len(new.children))
	}
	for name, child1 := range old.children {
		if child2, found := new.children[name]; found {
			if child2.ino != child1.ino {
				t.Errorf("%s children inode %d, have %d", prefix, child1.ino, child2.ino)
			}
		} else {
			t.Errorf("%s children inode %d, have nil", prefix, child1.ino)
		}
	}

	// check if two inodes have same parents
	if old.parents.count() != new.parents.count() {
		t.Errorf("%s %d parents, have %d", prefix, old.parents.count(), new.parents.count())
	}
	parents1 := sortParents(&old.parents)
	parents2 := sortParents(&new.parents)
	for i := range parents1 {
		if !sameParentEntry(parents1[i], parents2[i]) {
			t.Errorf("%s inode %d to be parent %d, have inode %d", prefix, parents1[i].node.ino, i, parents2[i].node.ino)
		}
	}

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

	var walk func(node *inode, level int)
	walk = func(node *inode, level int) {
		if level >= len(lastInLevel) {
			lastInLevel = append(lastInLevel, false)
		}
		last := false
		cnt := 0
		for _, child := range node.children {
			cnt++
			last = cnt == len(node.children)
			lastInLevel[level] = last
			printBranch(level, last, child)
			// is dir
			if child.children != nil {
				walk(child, level+1)
			}
		}

	}

	fmt.Printf(".(%d)\n", root.ino)
	walk(root, 0)

}

func TestDump(t *testing.T) {
	senderBridge := newBridge()
	constructDirTree(senderBridge)

	// simulate IPC
	inodeChan := make(chan *DumpInode)
	finish := make(chan struct{})

	dumpB, iter, err := senderBridge.Dump()
	if err != nil {
		t.Error(err)
	}

	receiverBridge := &rawBridge{}
	go func(dumpB *DumpRawBridge, d chan *DumpInode, f chan struct{}) {
		filler, err := receiverBridge.Restore(dumpB)
		if err != nil {
			t.Error(err)
		}
		for dumpInode, ok := <-d; ok; dumpInode, ok = <-d {
			err = filler.AddInode(dumpInode)
			if err != nil {
				t.Error(err)
			}
		}
		err = filler.Finished()
		if err != nil {
			t.Error(err)
		}

		finish <- struct{}{}
	}(dumpB, inodeChan, finish)

	for i, err := iter.Next(); err == nil; i, err = iter.Next() {
		inodeChan <- i
	}

	close(inodeChan)
	<-finish
	close(finish)

	// check if all inodes in two bridges are exactly the same
	oldNodeCnt, newNodeCnt := len(senderBridge.nodes), len(receiverBridge.nodes)
	if oldNodeCnt != newNodeCnt {
		t.Errorf("want %d inodes, have %d inodes", oldNodeCnt, newNodeCnt)
	}
	for ino, old := range senderBridge.nodes {
		assertSameInode(t, old, receiverBridge.nodes[ino])
	}

	oldDirCnt, newDirCnt := 0, 0
	for _, node := range senderBridge.nodes {
		if node.isDir() {
			oldDirCnt++
		}
	}
	for _, node := range receiverBridge.nodes {
		if node.isDir() {
			newDirCnt++
		}
	}
	if oldDirCnt != newDirCnt {
		t.Errorf("want %d directories, have %d", oldDirCnt, newDirCnt)
	}

	// print directory tree if test failed
	if t.Failed() {
		printDirTree(senderBridge.root)
		printDirTree(receiverBridge.root)
	}
}
