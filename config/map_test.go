package config

import (
	"bytes"
	"io/ioutil"
	"log"
	"testing"
	"unsafe"
)

var testData1 = []byte(`
# tabs/spaces, collision
d         name				

# ignored, error.
# d	 name collision ignored.            

# collision
d name

# omitted.
# // bad entry.

f disk archive
# comment
f disk archive

# recursive lookup from disk.
# R src dst
c   dst   -	 -   -	 test		  test  
c   nodata

# does lookup from disk.
# L elf dst
# L /usr/bin/bash
# L /usr/bin/bash bash

# sh -> busybox
l busybox sh

# intentional omit
f omit_test1 -
f omit_test2

# no arguments
L	  

$ var1 testvar1
d $var1
$ $var1 testvar2
d $testvar1
d $x1
`)

var testData2 = []byte(`
# should replace
d name 0 0 0

d merge1
f merge2 test
d $testvar1
d $x2

l busybox {foo,bar,baz}
d multi{
	1,2

	# comment
	3
} - 1 2

l ../foo/{bar,baz} symlinksrc/
f multifile{1,2,3} multidst
`)

var testMap = Map{
	m: func() map[string]int {
		r := make(map[string]int)
		for k, v := range []string{
			"name",
			"archive",
			"dst",
			"nodata",
			"sh",
			"omit_test1",
			"omit_test2",
			"merge1",
			"test",
			"testvar1",
			"testvar2",
			"$testvar1",
			"global1",
			"global2",
			"foo",
			"bar",
			"baz",
			"multi1",
			"multi2",
			"multi3",
			"symlinksrc/bar",
			"symlinksrc/baz",
			"multidst/multifile1",
			"multidst/multifile2",
			"multidst/multifile3",
		} {
			r[v] = k
		}
		return r
	}(),
	A: []Entry{
		{"name", "name", 0, 0, 0, TypeDirectory, nil, ""},
		{"disk", "archive", 0, 0, 0644, TypeRegular, nil, ""},
		{"dst", "dst", 0, 0, 0644, TypeCreate, []byte("test		  test  \n"), ""},
		{"nodata", "nodata", 0, 0, 0644, TypeCreate, []byte{}, ""},
		{"busybox", "sh", 0, 0, 0777, TypeSymlink, nil, ""},
		{"omit_test1", "omit_test1", 0, 0, 0644, TypeRegular, nil, ""},
		{"omit_test2", "omit_test2", 0, 0, 0644, TypeRegular, nil, ""},
		{"merge1", "merge1", 0, 0, 0755, TypeDirectory, nil, ""},
		{"merge2", "test", 0, 0, 0644, TypeRegular, nil, ""},
		{"testvar1", "testvar1", 0, 0, 0755, TypeDirectory, nil, ""},
		{"testvar2", "testvar2", 0, 0, 0755, TypeDirectory, nil, ""},
		{"$testvar1", "$testvar1", 0, 0, 0755, TypeDirectory, nil, ""},
		{"global1", "global1", 0, 0, 0755, TypeDirectory, nil, ""},
		{"global2", "global2", 0, 0, 0755, TypeDirectory, nil, ""},
		{"busybox", "foo", 0, 0, 0777, TypeSymlink, nil, ""},
		{"busybox", "bar", 0, 0, 0777, TypeSymlink, nil, ""},
		{"busybox", "baz", 0, 0, 0777, TypeSymlink, nil, ""},
		{"multi1", "multi1", 1, 2, 0755, TypeDirectory, nil, ""},
		{"multi2", "multi2", 1, 2, 0755, TypeDirectory, nil, ""},
		{"multi3", "multi3", 1, 2, 0755, TypeDirectory, nil, ""},
		{"../foo/bar", "symlinksrc/bar", 0, 0, 0777, TypeSymlink, nil, ""},
		{"../foo/baz", "symlinksrc/baz", 0, 0, 0777, TypeSymlink, nil, ""},
		{"multifile1", "multidst/multifile1", 0, 0, 0644, TypeRegular, nil, ""},
		{"multifile2", "multidst/multifile2", 0, 0, 0644, TypeRegular, nil, ""},
		{"multifile3", "multidst/multifile3", 0, 0, 0644, TypeRegular, nil, ""},
	},

	// TODO: include elf
}

func dataBuf1() *bytes.Buffer {
	return bytes.NewBuffer(testData1)
}

func dataBuf2() *bytes.Buffer {
	return bytes.NewBuffer(testData2)
}

type e struct {
	src, dst string
	uid, gid int
	mode     int
	t        string
	// ignored data []byte
}

func equal(src, dst *Entry) bool {
	if *(*e)(unsafe.Pointer(src)) != *(*e)(unsafe.Pointer(dst)) {
		return false
	}
	return bytes.Equal(src.Data, dst.Data)
}

func TestMapResolve(t *testing.T) {
	log.SetFlags(log.Lshortfile)
	log.SetOutput(ioutil.Discard)

	vars := []string{"x", "global"}

	m1 := FromReader(vars, dataBuf1())
	m2 := FromReader(vars, dataBuf2())
	if err := m1.Merge(m2); err != nil {
		t.Fatal(err)
	}

	if w, r := len(testMap.A), len(m1.A); w != r {
		t.Fatalf("len w(%d) != r(%d)", w, r)
	}

	for k, v := range testMap.m {
		var (
			rI     int
			exists bool
		)
		if rI, exists = m1.m[k]; !exists {
			t.Fatalf("key %q does not exist", k)
		}
		if !equal(&m1.A[rI], &testMap.A[v]) {
			t.Fatalf("slice does not equal\n%v\n%v\n%q\n%q",
				m1.A[rI],
				testMap.A[v],
				m1.A[rI].Data,
				testMap.A[v].Data,
			)
		}
	}
}

func TestSrc(t *testing.T) {
	s := []struct {
		s string
		f entry
	}{
		{"t1", entry{TypeCreate, "t1", "data"}},
		{"t2", entry{TypeSymlink, "t2", "dst"}},
		{"t3", entry{TypeDirectory, "t3"}},
		{"t4", entry{TypeRegular, "t4", "dst"}},
		{"t5", entry{TypeRegular, "t5", "-"}},
		{"t6", entry{TypeRegular, "t6"}},
	}
	for k, v := range s {
		src, err := v.f.Src()
		if err != nil {
			t.Fatal(k, err)
		}
		if src != v.s {
			t.Fatalf("%s != %s", src, v.s)
		}
	}
}

func TestDst(t *testing.T) {
	s := []struct {
		d string
		f entry
	}{
		{"t1", entry{TypeCreate, "t1", "data"}},
		{"t2", entry{TypeSymlink, "src", "t2"}},
		{"t3", entry{TypeDirectory, "t3"}},
		{"t4", entry{TypeRegular, "src", "t4"}},
		{"t5", entry{TypeRegular, "t5", "-"}},
		{"t6", entry{TypeRegular, "t6"}},
	}
	for k, v := range s {
		dst, err := v.f.Dst()
		if err != nil {
			t.Fatal(k, err)
		}
		if dst != v.d {
			t.Errorf("%s != %s", dst, v.d)
		}
	}
}
