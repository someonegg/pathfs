package pathfs

import (
	"testing"
)

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

	// add link
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

func TestDump(t *testing.T) {
	senderBridge := newTestBridge()
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
		t.Errorf("want: %d inodes, have: %d", oldNodeCnt, newNodeCnt)
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
		t.Errorf("want: %d directories, have: %d", oldDirCnt, newDirCnt)
	}

	// print directory tree if test failed
	if t.Failed() {
		printDirTree(senderBridge.root)
		printDirTree(receiverBridge.root)
	}
}
