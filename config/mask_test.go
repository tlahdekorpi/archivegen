package config

import "testing"

func TestMaskMap(t *testing.T) {
	var err error
	mm := make(maskMap, 0)

	for _, v := range []entry{
		entry{"mm", "0", ".", "1", "-", "-"},
		entry{"mm", "-", ".", "-", "2", "-"},
		entry{"mm", "-", ".", "-", "-", "3"},
		entry{"mm", "1", ".", "-", "2", "-"},
	} {
		mm, err = mm.set(v)
		if err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	{
		e1 := &Entry{Dst: "foo"}
		e2 := &Entry{Dst: "foo", Mode: 1, User: 2, Group: 3}
		mm.apply(e1)
		if a, b := e1.Format(), e2.Format(); a != b {
			t.Fatalf("add, apply: \n%q\n%q", a, b)
		}
	}

	for _, v := range []entry{
		entry{"mc", "-"},
		entry{"mc", "1"},
	} {
		mm, err = mm.del(v)
		if err != nil {
			t.Fatalf("del: %v", err)
		}
	}

	{
		e1 := &Entry{Dst: "foo"}
		e2 := &Entry{Dst: "foo", Mode: 1}
		mm.apply(e1)
		if a, b := e1.Format(), e2.Format(); a != b {
			t.Fatalf("del, apply: \n%q\n%q", a, b)
		}
	}

	mm, err = mm.del(entry{"mc"})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}

	if len(mm) != 0 {
		t.Fatal("clear: mm not empty")
	}
}

func TestMaskIgnore(t *testing.T) {
	for i, v := range []struct {
		e entry
		E *Entry
		r bool
	}{
		{entry{"", "", "foo"}, &Entry{Dst: "foo"}, false},
		{entry{"", "", "foo"}, &Entry{Dst: "bar"}, true},
	} {
		f, err := regexIgnoreMask(v.e, v.r)
		if err != nil {
			t.Errorf("ignoreMask %d, %v: %v", i, v.r, err)
		}
		if !f(v.E) {
			t.Errorf("ignoreMask %d, %v", i, v.r)
		}
	}
}

func TestMaskReplace(t *testing.T) {
	for i, v := range []struct {
		e entry
		E *Entry
		r string
	}{
		{entry{"", "", "foo", "bar"}, &Entry{Dst: "foo/bar/baz"}, "bar/bar/baz"},
		{entry{"", "", "foo/bar/"}, &Entry{Dst: "foo/bar/baz"}, "baz"},
	} {
		f, err := regexReplaceMask(v.e)
		if err != nil {
			t.Errorf("replaceMask %d, %v: %v", i, v.r, err)
		}

		f(v.E)

		if v.E.Dst != v.r {
			t.Errorf("replaceMask %d, %q != %q", i, v.E.Dst, v.r)
		}
	}
}

func TestMaskMode(t *testing.T) {
	for i, v := range []struct {
		e entry
		E *Entry
		r Entry
	}{
		{entry{"", "", "", "1", "-", "-"}, &Entry{}, Entry{Mode: 1}},
		{entry{"", "", "", "-", "2", "-"}, &Entry{}, Entry{User: 2}},
		{entry{"", "", "", "-", "-", "3"}, &Entry{}, Entry{Group: 3}},
	} {
		f, err := regexModeMask(v.e)
		if err != nil {
			t.Errorf("replaceMode %d, %v: %v", i, v.r, err)
		}

		f(v.E)

		if v.E.Format() != v.r.Format() {
			t.Errorf("replaceMode %d", i)
		}
	}
}
