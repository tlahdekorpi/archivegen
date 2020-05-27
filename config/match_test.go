package config

import "testing"

func TestResplit(t *testing.T) {
	for k, v := range []struct {
		from          string
		dir, re, next string
	}{
		{
			from: "/.*",
			dir:  "/", re: ".*", next: "",
		},
		{
			from: "/foo/bar",
			dir:  "/foo/bar", re: "", next: "",
		},
		{
			from: "/foo/bar/.*",
			dir:  "/foo/bar", re: ".*", next: "",
		},
		{
			from: ".*",
			dir:  "./", re: ".*", next: "",
		},
		{
			from: "/foo/bar/.*/baz",
			dir:  "/foo/bar", re: ".*", next: "baz",
		},
		{
			from: ".*/foo/.*/bar/.*baz$",
			dir:  "./", re: ".*", next: "foo/.*/bar/.*baz$",
		},
	} {
		dir, re, next := resplit(v.from)
		if a, b := [...]string{dir, re, next},
			[...]string{v.dir, v.re, v.next}; a != b {
			t.Fatalf("%d: %#v != %#v", k, a, b)
		}
	}
}

func elemEq(t *testing.T, from string, e1, e2 []elem) {
	if a, b := len(e1), len(e2); a != b {
		t.Errorf("%s: len: %d != %d", from, a, b)
		return
	}
	for k, v := range e1 {
		if v.p != e2[k].p {
			t.Errorf("%s: path: %s != %s", from, v.p, e2[k].p)
			continue
		}
		if v.n != e2[k].n {
			t.Errorf("%s: neg: %s(%v) != %s(%v)",
				from,
				v.p, v.n,
				e2[k].p, e2[k].n,
			)
			continue
		}
	}
}

func TestRe(t *testing.T) {
	for _, v := range []struct {
		from string
		e    []elem
	}{
		{
			from: "/foo/bar",
			e:    []elem{{p: "/foo/bar"}},
		},
		{
			from: "/foo/bar/.*/baz",
			e:    []elem{{p: "/foo/bar"}, {p: "baz"}},
		},
		{
			from: "/foo/bar/!.*/baz",
			e:    []elem{{p: "/foo/bar", n: true}, {p: "baz"}},
		},
		{
			from: ".*",
			e:    []elem{{p: "./"}},
		},
		{
			from: ".*/foo/!.*/bar/.*baz$",
			e: []elem{
				{p: "./"},
				{p: "foo", n: true},
				{p: "bar"},
			},
		},
	} {
		e, err := re(v.from)
		if err != nil {
			t.Fatal(err)
		}
		elemEq(t, v.from, e, v.e)
	}
}
