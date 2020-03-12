package elf

import (
	"debug/elf"
	"errors"
	"sort"
	"testing"
)

type ef [][]string

func get(s [][]string, n int) []string {
	var r []string
	if len(s)-1 >= n {
		r = s[n]
	}
	return r
}

func (e ef) DynString(tag elf.DynTag) ([]string, error) {
	switch tag {
	case elf.DT_NEEDED:
		return get(e, 0), nil
	case elf.DT_RUNPATH:
		return get(e, 1), nil
	case elf.DT_RPATH:
		return get(e, 2), nil
	}
	return nil, errors.New("no such tag: " + tag.String())
}

func (e ef) Close() error {
	return nil
}

func mapOpen(d map[string]ef) func(f string) (elfFile, error) {
	return func(f string) (elfFile, error) {
		r, exists := d[f]
		if !exists {
			return nil, errorNotFound(f)
		}
		return r, nil
	}
}

func testResolve(t *testing.T, f string, re []string, data map[string]ef) {
	open = mapOpen(data)

	r, err := resolve(f, nil, true, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	if w, r := len(re), len(r); w != r {
		t.Fatalf("len w(%d) != r(%d)", w, r)
	}

	sort.Strings(r)
	sort.Strings(re)

	for k, v := range re {
		if h := r[k]; v != h {
			t.Fatalf("have != want, %s != %s", v, h)
		}
	}
}

func TestResolveMPD(t *testing.T) {
	testResolve(t, "/usr/bin/mpd", mpdresolved, mpdfiles)
}

func TestResolveQemu(t *testing.T) {
	testResolve(t, "/usr/bin/qemu-system-x86_64", qemuresolved, qemufiles)
}
