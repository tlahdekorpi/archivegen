package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/tlahdekorpi/archivegen/archive"
	"github.com/tlahdekorpi/archivegen/archive/cpio"
	"github.com/tlahdekorpi/archivegen/archive/tar"
	"github.com/tlahdekorpi/archivegen/config"
	"github.com/tlahdekorpi/archivegen/tree"
)

var buildversion string = "v0"

func open(file string) *os.File {
	r, err := os.OpenFile(
		file,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0644,
	)
	if err != nil {
		log.Fatal(err)
	}
	return r
}

func loadTree(rootfs string, vars, files []string, stdin bool) (*tree.Node, error) {
	var m1, m2 *config.Map
	var err error

	if stdin {
		m1, err = config.FromReaderRoot(rootfs, vars, os.Stdin)
	}
	if err != nil {
		return nil, fmt.Errorf("stdin: %v", err)
	}

	m2, err = config.FromFilesRoot(rootfs, vars, files...)
	if err != nil {
		return nil, err
	}

	if m1 != nil {
		err = m1.Merge(m2)
	} else {
		m1 = m2
	}
	if err != nil {
		return nil, err
	}

	t := tree.Render(m1)
	if len(t.Map) == 0 {
		return nil, fmt.Errorf("empty archive")
	}

	return t, nil
}

func printTree(t *tree.Node, b64 bool) {
	tw := tabwriter.NewWriter(os.Stdout, 1, 1, 2, ' ', 0)
	t.Print("", tw, os.Stdout, b64)
	tw.Flush()
}

func stdinPipe() bool {
	f, err := os.Stdin.Stat()
	if err != nil {
		log.Fatal(err)
	}
	return (f.Mode() & os.ModeCharDevice) == 0
}

type varValue []string

func (v *varValue) String() string { return "" }

func (v *varValue) Set(val string) error {
	x := strings.SplitN(val, "=", 2)
	if len(x) < 2 {
		return fmt.Errorf("foo=bar")
	}
	*v = append(*v, x...)
	return nil
}

type opts struct {
	ArchiveFormat bool   `desc:"Archive configuration file format" flag:"format"`
	Base64        bool   `desc:"Base64 encode all create types" flag:"b64"`
	Chdir         string `desc:"Change directory before doing anything" flag:"C"`
	Format        string `desc:"Output archive format" flag:"fmt"`
	Out           string `desc:"Output destination"`
	Print         bool   `desc:"Print the resolved tree in archivegen format"`
	Rootfs        string `desc:"Alternate root for relative and ELF types"`
	Stdout        bool   `desc:"Write archive to stdout"`
	Timestamp     bool   `desc:"Preserve file timestamps"`
	Version       bool   `desc:"Version information"`
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("archivegen: ")

	opt := opts{
		Out:    "out.archive",
		Format: "tar",
	}
	buildflags(&opt, "")
	buildflags(&config.Opt, "")

	var varX varValue
	flag.Var(&varX, "X", "Variable\n"+
		"e.g. '-X foo=bar -X a=b'",
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s %s\n", "archivegen", "[OPTIONS...] [FILES...]")
		flag.PrintDefaults()
	}
	flag.Parse()

	if opt.Chdir != "" {
		if err := os.Chdir(opt.Chdir); err != nil {
			log.Fatal(err)
		}
	}

	if opt.ArchiveFormat {
		fmt.Fprintln(os.Stderr, helpFormat[1:])
		return
	}

	if opt.Version {
		fmt.Printf("%s, %s\n", buildversion, runtime.Version())
		return
	}

	p := stdinPipe()
	if flag.NArg() < 1 && !p {
		log.Fatal("not enough arguments")
	}

	root, err := loadTree(opt.Rootfs, []string(varX), flag.Args(), p)
	if err != nil {
		log.Fatal(err)
	}

	if opt.Print {
		printTree(root, opt.Base64)
		os.Exit(0)
	}

	var out *os.File = os.Stdout
	if !opt.Stdout {
		out = open(opt.Out)
	}

	buf := bufio.NewWriterSize(out, 1<<24)

	var in archive.Writer
	switch opt.Format {
	case "tar":
		in = tar.NewWriter(buf, opt.Timestamp)
	case "cpio":
		in = cpio.NewWriter(buf, opt.Timestamp)
	default:
		log.Fatalln("invalid format:", opt.Format)
	}

	if err := root.Write("", in); err != nil {
		log.Fatalln("write:", err)
	}

	for k, v := range []func() error{
		in.Close, buf.Flush, out.Close,
	} {
		if err := v(); err != nil {
			log.Fatalf("error(%d): %v", k, err)
		}
	}
}
