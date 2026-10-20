package pathfs

import (
	"github.com/hanwen/go-fuse/v2/fuse"
	"io"
	"reflect"
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

func TestDumpSnapshotIsolation(t *testing.T) {
	b := newTestBridge()
	n := b.addChild(b.root, "old", 2, false)
	fh := b.registerFile(fuse.Owner{Uid: 12}, "old", 42, []fuse.DirEntry{{Name: "original"}})
	free := b.registerFile(fuse.Owner{}, "free", 0, nil)
	b.unregisterFile(free)
	data, iter, err := b.Dump()
	if err != nil {
		t.Fatal(err)
	}
	b.files[fh].stream[0].Name = "changed"
	b.freeFiles[0] = 99
	b.addChild(b.root, "new", n.ino, false)
	if data.Files[fh].Stream[0].Name != "original" || data.FreeFiles[0] != free {
		t.Error("dump shares mutable file state with source")
	}
	for {
		entry, err := iter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if entry.Ino == n.ino && (entry.LookupCount != 1 || len(entry.Parents) != 1 || entry.Parents[0].Name != "old") {
			t.Errorf("inode changed after Dump: %+v", entry)
		}
	}
}

func TestRestoreContinuedUse(t *testing.T) {
	source := newTestBridge()
	n := source.addChild(source.root, "first", 2, false)
	source.addChild(source.root, "latest", 2, false)
	source.addChild(source.root, "empty", 3, true)
	orphan := source.addChild(source.root, "orphan", 4, true)
	source.rmChild(source.root, "orphan")
	fh := source.registerFile(fuse.Owner{Uid: 12}, "first", 42, []fuse.DirEntry{{Name: "entry"}})
	free := source.registerFile(fuse.Owner{}, "free", 0, nil)
	source.unregisterFile(free)
	data, iter, err := source.Dump()
	if err != nil {
		t.Fatal(err)
	}
	dest := newTestBridge()
	filler, err := dest.Restore(data)
	if err != nil {
		t.Fatal(err)
	}
	for {
		entry, err := iter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := filler.AddInode(entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := filler.Finished(); err != nil {
		t.Fatal(err)
	}
	if dest.pathOf(dest.inode(n.ino)) != "latest" {
		t.Fatal("lost preferred hard link")
	}
	dest.rmChild(dest.root, "latest")
	if dest.pathOf(dest.inode(n.ino)) != "first" {
		t.Fatal("lost hard link fallback")
	}
	if !dest.inode(3).isDir() || !dest.inode(orphan.ino).isDir() {
		t.Fatal("lost empty/orphan directory type")
	}
	if !reflect.DeepEqual(dest.files[fh], source.files[fh]) {
		t.Fatal("lost file state")
	}
	dest.files[fh].stream[0].Name = "changed"
	if data.Files[fh].Stream[0].Name != "entry" {
		t.Error("restore aliases dump stream")
	}
	dest.freeFiles[0] = 77
	if data.FreeFiles[0] != free {
		t.Error("restore aliases free handles")
	}
	dest.freeFiles[0] = free
	if got := dest.registerFile(fuse.Owner{}, "reuse", 99, nil); got != free {
		t.Fatalf("reused %d, want %d", got, free)
	}
	dest.Forget(n.ino, 2)
	if dest.inodeSafe(n.ino) != nil {
		t.Error("restored references cannot be forgotten")
	}
}

func TestRestoreRejectsIncompleteInodes(t *testing.T) {
	tests := []struct {
		name    string
		count   int
		entries []*DumpInode
	}{
		{"duplicate", 2, []*DumpInode{{Ino: 1, IsDir: true}, {Ino: 1, IsDir: true}}},
		{"missing parent", 2, []*DumpInode{{Ino: 1, IsDir: true}, {Ino: 2, Parents: []DumpParentEntry{{Node: 3, Name: "file"}}}}},
		{"extra inode", 1, []*DumpInode{{Ino: 1, IsDir: true}, {Ino: 2}}},
		{"missing root record", 1, []*DumpInode{{Ino: 2, Parents: []DumpParentEntry{{Node: 1, Name: "file"}}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBridge()
			filler, err := b.Restore(&DumpRawBridge{NodeCount: tt.count, Files: []*DumpFileEntry{{}}})
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range tt.entries {
				if err = filler.AddInode(entry); err != nil {
					return
				}
			}
			if err = filler.Finished(); err == nil {
				t.Fatal("accepted incomplete or duplicate inode records")
			}
		})
	}
}
