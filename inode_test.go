package pathfs

import (
	"fmt"
	"sync"
	"testing"
)

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

func TestAddAndRmChild(t *testing.T) {
	b := newTestBridge()
	files := []simpleFileInfo{
		{}, {},
		{"l1_d1", 2, true},
		{"l1_d2", 3, true},
		{"l1_d3", 4, true},
		{"l1_r1", 5, false},
		{"l2_d1", 6, true},
		{"l2_r1", 7, false},
		{"l2_d2", 8, true},
		{"l2_f2", 9, false},
	}

	var wg sync.WaitGroup

	// addChild testing
	// test parallel insertion
	wg.Add(4)
	for i := 2; i <= 5; i++ {
		go func(i int) {
			b.addChild(b.root, files[i].name, files[i].ino, files[i].isDir)
			wg.Done()
		}(i)
	}
	wg.Wait()

	if len(b.root.children) != 4 {
		t.Errorf("want inode 1 children's count: %d, have: %d", 4, len(b.root.children))
	}
	if b.inode(4).parents.get().node != b.root {
		t.Errorf("want inode 4' parent to be: %d, have: %d", b.root.ino, b.inode(4).parents.get().node.ino)
	}

	inode2 := b.inode(2)
	for i := 6; i <= 7; i++ {
		b.addChild(inode2, files[i].name, files[i].ino, files[i].isDir)
	}

	// addChild tasks in [parent, child] format
	tasks := [][2]int{
		{3, 7}, {4, 7}, {3, 8}, {6, 7}, {3, 9},
	}

	wg.Add(5)
	for _, task := range tasks {
		go func(task [2]int) {
			i := task[1]
			b.addChild(b.inode(uint64(task[0])), files[i].name, files[i].ino, files[i].isDir)
			wg.Done()
		}(task)
	}
	wg.Wait()

	if b.NodeCount() != 9 {
		t.Errorf("want inode's count: %d, have: %d", 9, b.NodeCount())
	}
	if len(b.inode(3).children) != 3 {
		t.Errorf("want inode 3 children's count: %d, have: %d", 3, len(b.inode(3).children))
	}
	if b.inode(7).parents.count() != 4 {
		t.Errorf("want inode 7 parent's count: %d, have: %d", 4, b.inode(7).parents.count())
	}

	// test rmChild
	b.inode(3).lookupCount = 0 // only for testing removeRef in rmChild
	tasks = append(tasks, [2]int{2, 7})
	wg.Add(6)
	for _, task := range tasks {
		go func(task [2]int) {
			b.rmChild(b.inode(uint64(task[0])), files[task[1]].name)
			wg.Done()
		}(task)
	}
	wg.Wait()

	// because inode 3 has been removed by removeRef
	if b.NodeCount() != 8 {
		t.Errorf("want inode's count: %d, have: %d", 8, b.NodeCount())
	}
	if b.nodes[3] != nil {
		t.Errorf("want inode 3 to be nil, have not nil")
	}
	if len(b.inode(2).children) != 1 {
		t.Errorf("want inode 2 children's count: %d, have: %d", 1, len(b.inode(2).children))
	}
	if b.inode(7).parents.count() != 0 {
		t.Errorf("want inode 7 parent's count: %d, have: %d", 0, b.inode(7).parents.count())
	}

}

func TestRemoveRef(t *testing.T) {
	b := newTestBridge()
	files := []simpleFileInfo{
		{}, {},
		{"l1_d1", 2, true},
		{"l1_d2", 3, true},
		{"l1_d3", 4, true},
		{"l1_r1", 5, false},
		{"l2_d1", 6, true},
		{"l2_r1", 7, false},
		{"l2_d2", 8, true},
		{"l2_r2", 9, false},
	}

	for i := 2; i <= 5; i++ {
		b.addChild(b.root, files[i].name, files[i].ino, files[i].isDir)
	}

	tasks := [][2]int{
		{2, 6}, {2, 7},
		{3, 7}, {3, 8}, {3, 9},
		{4, 7},
	}

	for _, task := range tasks {
		i := task[1]
		b.addChild(b.inode(uint64(task[0])), files[i].name, files[i].ino, files[i].isDir)
	}

	// removeRef tasks in [ino, nlookup] format
	removeTasks := [][2]int{
		{6, 1}, {7, 1},
		{7, 1}, {8, 1}, {9, 1},
	}

	b.inode(7).lookupCount-- // only for testing if one inode has parents but whose lookupCount is 0
	var wg sync.WaitGroup

	wg.Add(5)
	for i := range removeTasks {
		go func(i int) {
			task := tasks[i]
			b.rmChild(b.inode(uint64(task[0])), files[task[1]].name)
			removeTask := removeTasks[i]
			// simulate Forget
			b.removeRef(b.inode(uint64(removeTask[0])), uint32(removeTask[1]))
			wg.Done()
		}(i)
	}
	wg.Wait()

	if b.NodeCount() != 5 {
		t.Errorf("want inode count: %d, have %d", 5, b.NodeCount())
	}
	if len(b.inode(3).children) != 0 {
		t.Errorf("want inode 3 children's count: %d, have: %d", 1, len(b.inode(3).children))
	}
	if len(b.inode(4).children) != 0 {
		t.Errorf("want inode 4 children's count: %d, have: %d", 0, len(b.inode(4).children))
	}
	if b.nodes[7] != nil {
		t.Errorf("want inode 7 to be nil, have not nil")
	}

}

func TestMvChild(t *testing.T) {
	b := newTestBridge()
	files := []simpleFileInfo{
		{}, {},
		{"f1", 2, true},
		{"f2", 3, true},
		{"f3", 4, true},
		{"f4", 5, false},

		{"f5", 6, true},
		{"f6", 7, true},

		{"f6", 8, true},

		{"f5", 9, false},
	}

	addTasks := [][2]int{
		{1, 2}, {1, 3}, {1, 4}, {1, 5},
		{2, 6}, {2, 7},
		{3, 8},
		{8, 9},
	}

	for _, task := range addTasks {
		i := task[1]
		b.addChild(b.inode(uint64(task[0])), files[i].name, files[i].ino, files[i].isDir)
	}

	// move inode 5 to inode 8
	moved := b.mvChild(b.root, files[5].name, b.inode(8), files[9].name, false)
	if moved {
		t.Errorf("want inode 5 not to be moved, but moved")
	}

	// mvChild tasks in [parentIno, newParentIno, fileIno] format
	mvTasks := [][3]int{
		{1, 2, 4},
		{1, 2, 5},
		{3, 2, 8},
	}

	var wg sync.WaitGroup

	wg.Add(3)
	for _, task := range mvTasks {
		go func(t [3]int) {
			b.mvChild(b.inode(uint64(t[0])), files[t[2]].name, b.inode(uint64(t[1])), files[t[2]].name, true)
			wg.Done()
		}(task)
	}
	wg.Wait()

	if len(b.root.children) != 2 {
		t.Errorf("want root children's count to be: %d, have: %d", 2, len(b.root.children))
	}
	if len(b.inode(2).children) != 4 {
		t.Errorf("want inode 2 children's count to be: %d, have: %d", 4, len(b.inode(2).children))
	}
	if len(b.inode(3).children) != 0 {
		t.Errorf("want inode 3 children's count to be: %d, have: %d", 0, len(b.inode(3).children))
	}
	if b.inode(7).parents.count() != 0 {
		t.Errorf("want inode 7 parent's count to be: %d, have: %d", 0, b.inode(7).parents.count())
	}

}
