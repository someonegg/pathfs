package pathfs

import (
	"testing"
)

func sameParentEntry(e1, e2 parentEntry) bool {
	if e1.name != e2.name {
		return false
	}
	if e1.node != nil && e2.node != nil {
		return e1.node.ino == e2.node.ino
	}
	return e1.node == e2.node
}

func TestInodeParents(t *testing.T) {
	var p inodeParents
	var ino1, ino2, ino3 inode

	// empty store should be empty without panicing
	if count := p.count(); count != 0 {
		t.Error(count)
	}
	if p.all() != nil {
		t.Error("empty store should return nil but did not")
	}

	// non-dupes should be stored
	all := []parentEntry{
		parentEntry{"foo", &ino1},
		parentEntry{"foo2", &ino1},
		parentEntry{"foo3", &ino1},
		parentEntry{"foo", &ino2},
		parentEntry{"foo", &ino3},
	}
	for i, v := range all {
		p.add(v)
		if count := p.count(); count != i+1 {
			t.Errorf("want=%d have=%d", i+1, count)
		}
		last := p.get()
		if last != v {
			t.Error("get did not give us last-known parent")
		}
	}

	// adding dupes should not cause the count to increase, but
	// must cause get() to return the most recently added dupe.
	for _, v := range all {
		p.add(v)
		if count := p.count(); count != len(all) {
			t.Errorf("want=%d have=%d", len(all), count)
		}
		last := p.get()
		if last != v {
			t.Error("get did not give us last-known parent")
		}
	}

	all2 := p.all()
	if len(all) != len(all2) {
		t.Errorf("want=%d have=%d", len(all), len(all2))
	}

	// test sortParents
	sorted := sortParents(&p)
	n := p.count()
	if sorted[n-1] != p.newest {
		t.Errorf("want inode %d to be newest parents, have inode %d", p.newest.node.ino, sorted[n-1].node.ino)
	}
	for i := 1; i < n-1; i++ {
		prev, cur := sorted[i-1], sorted[i]
		if !p.other[cur].After(p.other[prev]) {
			t.Errorf("want parent inode %d to be newer than %d", cur.node.ino, prev.node.ino)
		}
	}

	// test delete
	// swap entry 2 and 3 to cover all statements
	all[1], all[2] = all[2], all[1]
	for i := n - 1; i >= 0; i-- {
		p.delete(all[i])
		if count := p.count(); count != i {
			t.Errorf("want=%d have=%d", len(all)-i-1, count)
		}
	}

}
