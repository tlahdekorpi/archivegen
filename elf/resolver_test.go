package elf

import (
	"debug/elf"
	"sort"
	"testing"
)

type ef [][]string

func (e ef) get(n int) []string {
	if len(e)-1 >= n {
		return e[n]
	}
	return nil
}

func (e ef) Interpreter() (string, error) { return "", errNoInterp }

func (e ef) Class() elf.Class { return 0 }

func (e ef) Close() error { return nil }

func (e ef) Dynamic() (File, error) {
	return File{
		Runpath: e.get(2),
		Rpath:   e.get(1),
		Needed:  e.get(0),
	}, nil
}

func mapOpen(d map[string]ef) Loader {
	return func(f, _ string) (ELF, error) {
		if r, exists := d[f]; exists {
			return r, nil
		}
		return nil, errorNotFound(f)
	}
}

func testResolve(t *testing.T, f string, re []string, data map[string]ef) {
	resolver := &Resolver{Loader: mapOpen(data)}

	r, err := resolver.Resolve(f)
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
