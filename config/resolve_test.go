package config

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

const nulstr = string(0)

func join(a, b string) string {
	return a + "/" + b
}

type pwd string

func (p pwd) with(t *testing.T, T string, f func()) {
	defer func() {
		if err := os.Chdir(string(p)); err != nil {
			t.Fatal(err)
		}
	}()

	if err := os.Chdir(T); err != nil {
		t.Error(err)
		return
	}

	f()
}

type kv struct {
	k, v string
}

var expandTests = []kv{
	{"test", "test"},
	{"test/dir", "test/dir"},
	{"test/dir/../..", "."},
	{"test/link1", "test"},
	{"test/link2", "test/dir"},
	{"test/link1/dir", "test/dir"},
	{"test/link2/..", "test"},
	{"test/dir/link3", "."},
	{"test/link2/link3/test", "test"},
	{"test/linkabs", "/"},
	{"test/infinity1", "infinity"},
	{"test/link5/link4/link6", "test/link6"},
	{"test/dir/link7", nulstr + "/test"},
}

var expandTestData = []struct {
	t, src, dst string
}{
	{t: TypeDirectory, src: "test"},
	{t: TypeDirectory, src: "test/dir"},
	{t: TypeDirectory, src: "test/link6"},
	{t: TypeDirectory, src: "tt"},
	{t: TypeSymlink, src: "test/dir/link3", dst: "../../"},
	{t: TypeSymlink, src: "test/dir/link4", dst: "../dir"},
	{t: TypeSymlink, src: "test/link5", dst: "dir/link4"},
	{t: TypeSymlink, src: "test/dir/link6", dst: "../link6"},
	{t: TypeSymlink, src: "test/dir/link7", dst: nulstr + "/t/link"},
	{t: TypeSymlink, src: "t", dst: "tt"},
	{t: TypeSymlink, src: "tt/link", dst: "../test"},
	{t: TypeSymlink, src: "test/link1", dst: "../test"},
	{t: TypeSymlink, src: "test/link2", dst: "dir"},
	{t: TypeSymlink, src: "test/linkabs", dst: "/"},
	{t: TypeSymlink, src: "test/infinity1", dst: "infinity2"},
	{t: TypeSymlink, src: "test/infinity2", dst: "infinity1"},
}

var expandMapData = []Entry{
	Entry{Src: "../test", Dst: "tmp/" + nulstr + "/test/link1"},
	Entry{Src: "../test", Dst: "../test/link1"},
	Entry{Src: "../test", Dst: "test/link1"},
	Entry{Src: "dir", Dst: "tmp/" + nulstr + "/test/link2"},
	Entry{Src: "dir", Dst: "../test/link2"},
	Entry{Src: "dir", Dst: "test/link2"},
	Entry{Src: "../../", Dst: "tmp/" + nulstr + "/test/dir/link3"},
	Entry{Src: "../../", Dst: "../test/dir/link3"},
	Entry{Src: "../../", Dst: "test/dir/link3"},
	Entry{Src: "/", Dst: "tmp/" + nulstr + "/test/linkabs"},
	Entry{Src: "/", Dst: "../test/linkabs"},
	Entry{Src: "/", Dst: "test/linkabs"},
	Entry{Src: "infinity2", Dst: "tmp/" + nulstr + "/test/infinity1"},
	Entry{Src: "infinity1", Dst: "tmp/" + nulstr + "/test/infinity2"},
	Entry{Src: "dir/link4", Dst: "tmp/" + nulstr + "/test/link5"},
	Entry{Src: "../dir", Dst: "tmp/" + nulstr + "/test/dir/link4"},
	Entry{Src: "../link6", Dst: "tmp/" + nulstr + "/test/dir/link6"},
	Entry{Src: "dir/link4", Dst: "../test/link5"},
	Entry{Src: "../dir", Dst: "../test/dir/link4"},
	Entry{Src: "../link6", Dst: "../test/dir/link6"},
	Entry{Src: "dir/link4", Dst: "test/link5"},
	Entry{Src: "../dir", Dst: "test/dir/link4"},
	Entry{Src: "../link6", Dst: "test/dir/link6"},
	Entry{Src: "/tmp/" + nulstr + "/t/link", Dst: "tmp/" + nulstr + "/test/dir/link7"},
	Entry{Src: "tt", Dst: "tmp/" + nulstr + "/t"},
	Entry{Src: "../test", Dst: "tmp/" + nulstr + "/tt/link"},
	Entry{Src: "/tmp/" + nulstr + "/t/link", Dst: "../test/dir/link7"},
	Entry{Src: "/tmp/" + nulstr + "/t/link", Dst: "test/dir/link7"},
}

func TestExpand(t *testing.T) {
	tmp, err := ioutil.TempDir("", "test_expand")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			t.Fatal("expand: failed to remove", err)
		}
	}()

	for _, v := range expandTestData {
		p := join(tmp, v.src)

		var err error
		switch v.t {
		case TypeDirectory:
			err = os.Mkdir(p, 0755)
		case TypeSymlink:
			x := strings.Replace(v.dst, nulstr, tmp, -1)
			err = os.Symlink(x, p)
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	r, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	dir := pwd(r)

	var c Config
	m := c.newMap()

	for _, v := range expandTests {
		v.v = strings.Replace(v.v, nulstr, tmp, -1)

		p := join(tmp, v.k)
		d := join(tmp, v.v)

		if v.v[0] == '/' {
			d = v.v
		}

		if v.v == "infinity" {
			_, err := m.expand(p, nil)
			if err != errTooManyLinks {
				t.Fatal(err)
			}
			continue
		}

		x, err := m.expand(p, nil)
		if err != nil {
			t.Errorf("expand(%q): error: %v %q", v.k, err, x)
			continue
		}
		if path.Clean(x) != path.Clean(d) {
			t.Errorf("expand(%q): %q != %q", p, x, d)
			continue
		}

		dir.with(t, p, func() {
			p, err := m.expand(".", nil)
			if err != nil {
				t.Errorf(`expand("."): error: %q %v`, v.k, err)
			}
			if p != "." {
				t.Errorf(`expand("."): error: %q: %q != "."`, v.k, p)
			}
		})

		dir.with(t, join(tmp, "test"), func() {
			P := join("..", v.k)
			D := join("..", v.v)

			if v.v[0] == '/' {
				D = v.v
			}

			if p, err := m.expand(P, nil); err != nil {
				t.Errorf("expand(%q): error: %v", v.k, err)
			} else if path.Clean(p) != path.Clean(D) {
				t.Errorf("expand(%q): %q != %q", P, p, D)
			}
		})

		dir.with(t, tmp, func() {
			if p, err := m.expand(v.k, nil); err != nil {
				t.Errorf("expand(%q): error: %v", v.k, err)
			} else if path.Clean(p) != path.Clean(v.v) {
				t.Errorf("expand(%q): %q != %q", v.k, p, v.v)
			}
		})
	}

	if l1, l2 := len(m.A), len(expandMapData); l1 != l2 {
		t.Fatalf("map array len: %d != %d", l1, l2)
	}

	for k, v := range m.A {
		v1 := v
		v2 := expandMapData[k]
		_, td := path.Split(tmp)
		v2.Dst = strings.Replace(v2.Dst, nulstr, td, -1)
		v2.Src = strings.Replace(v2.Src, nulstr, td, -1)

		if v1.Src != v2.Src {
			t.Errorf("src: %d\n%q\n%q", k, v1.Src, v2.Src)
		}
		if v1.Dst != v2.Dst {
			t.Errorf("dst: %d\n%q\n%q", k, v1.Dst, v2.Dst)
		}
	}
}
