package utilities

import "testing"

func TestSet(t *testing.T) {
	set := NewSet[string]()
	set.Add("foo")

	if !set.Has("foo") {
		t.Errorf("expecting 'foo' to be in set")
	}

	if set.Len() != 1 {
		t.Errorf("expecting length of set to be 1, got: %d", set.Len())
	}

	set.Add("bar")
	if set.Len() != 2 {
		t.Errorf("expecting length of set to be 2, got: %d", set.Len())
	}

	set.Remove("foo")
	if set.Has("foo") {
		t.Errorf("expecting 'foo' NOT to be in set")
	}

	if set.Len() != 1 {
		t.Errorf("expecting length of set to be 1, got: %d", set.Len())
	}

	set.Clear()
	if set.Len() != 0 {
		t.Errorf("expecting length of set to be 0, got: %d", set.Len())
	}
}
