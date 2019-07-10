package tree

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tlahdekorpi/archivegen/archive"
	"github.com/tlahdekorpi/archivegen/config"
)

type Node struct {
	Map map[string]*Node
	E   config.Entry
}

type Writer interface {
	io.Writer
	Flush() error
}

func (n *Node) Print(p string, w Writer, bw io.Writer, b64 bool) {
	var (
		d []string
		m []string
		l int
	)

	m = mapsort(n.Map)
	l = len(m)

	for i := 0; i < l; i++ {
		v := m[i]

		if n.Map[v].E.Type == config.TypeDirectory {
			d = append(d, v)
			continue
		}

		E := n.Map[v].E

		if b64 {
			E = E.Base64()
		}

		if E.Heredoc == "" {
			fmt.Fprintln(w, E.Format())
		} else {
			fmt.Fprintln(w, E.Format())
			w.Flush()
			fmt.Fprintf(bw, "%s%s\n\n", E.Data, E.Heredoc)
		}

		if i >= l-1-len(d) {
			fmt.Fprintln(w)
		}
	}

	for _, v := range d {
		var dn string

		if p != "" {
			dn = p + "/" + v
		} else {
			dn = v
		}
		if dn == "" {
			panic("empty dn")
		}

		n.Map[v].E.Dst = dn
		fmt.Fprintln(w, n.Map[v].E.Format())

		n.Map[v].Print(dn, w, bw, b64)
	}
}

func (n *Node) Add(name string, file config.Entry) *Node {
	r, exists := n.Map[name]
	if exists {
		return r
	}

	x := &Node{
		Map: make(map[string]*Node),
		E:   file,
	}
	n.Map[name] = x

	return x
}

func mapsort(m map[string]*Node) []string {
	r := make([]string, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	sort.Strings(r)

	return r
}

func (n *Node) Write(p string, w archive.Writer) error {
	d := make([]string, 0)

	// write all non-directories
	for _, v := range mapsort(n.Map) {
		if n.Map[v].E.Type == config.TypeDirectory {
			d = append(d, v)
			continue
		}
		if err := Write(n.Map[v].E, w); err != nil {
			return err
		}
	}

	for _, v := range d {
		var dn string
		if p != "" {
			dn = p + "/" + v
		} else {
			dn = v
		}

		if dn == "" {
			// next entry
			if err := n.Map[v].Write(v, w); err != nil {
				return err
			}
			continue
		}

		de := n.Map[v].E
		de.Src = dn

		if err := Write(de, w); err != nil {
			return err
		}

		if err := n.Map[v].Write(dn, w); err != nil {
			return err
		}
	}

	return nil
}

func Render(cfg *config.Map) *Node {
	root := &Node{
		E: config.Entry{
			Src:   "/",
			Dst:   "/",
			User:  0,
			Group: 0,
			Mode:  0755,
			Type:  config.TypeDirectory,
		},
		Map: map[string]*Node{},
	}

	for _, v := range cfg.A {
		// path should already be clean.
		p := strings.Split(v.Dst, "/")

		if len(p) == 1 && p[0] != "" {
			root.Add(p[0], v)
		}
		if len(p) < 2 {
			continue
		}
		if p[0] == "" && p[1] == "" {
			continue
		}

		tree := root
		for i := 0; i < len(p); i++ {
			if i != len(p)-1 {
				d := v
				d.Src = p[i]

				if v.Type != config.TypeDirectory {
					d.Mode = 0755
				}

				d.Type = config.TypeDirectory
				tree = tree.Add(p[i], d)
				continue
			}

			tree = tree.Add(p[i], v)
			break
		}
	}

	return root
}
