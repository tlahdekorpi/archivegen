package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"syscall"
	"text/tabwriter"
	"unsafe"

	"github.com/tlahdekorpi/archivegen/archive"
	"github.com/tlahdekorpi/archivegen/archive/cpio"
	"github.com/tlahdekorpi/archivegen/archive/tar"
	"github.com/tlahdekorpi/archivegen/config"
	"github.com/tlahdekorpi/archivegen/elf"
	"github.com/tlahdekorpi/archivegen/tree"
)

var buildversion string = "v0"

func isterm(file *os.File) bool {
	var t [44]byte // sizeof(struct termios/termios2)
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), ioctl, uintptr(unsafe.Pointer(&t)))
	return err == 0
}

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

func loadTree(c *config.Config, files []string, stdin bool) (*tree.Node, error) {
	var m1, m2 *config.Map
	var err error

	if stdin {
		m1, err = c.FromReader(os.Stdin)
	}
	if err != nil {
		return nil, fmt.Errorf("stdin: %v", err)
	}

	m2, err = c.FromFiles(files...)
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
	Version       bool   `desc:"Version information"`
	Ldconf        string `desc:"Path to ld.so.conf" flag:"ld.so.conf"`
	Size          int    `desc:"Buffer size"`
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("archivegen: ")

	opt := opts{
		Format: "tar",
		Ldconf: "/etc/ld.so.conf",
		Size:   1 << 22,
	}
	buildflags(&opt, "")

	// Resolving all symlinks is required when symlinks inside the prefix
	// lead to outside of the prefix.
	config.Opt.Glob.Expand = true
	config.Opt.File.Expand = true

	// ELFs depending on libraries from rpath/runpath $ORIGIN will
	// fail to resolve since the symlink is used as $ORIGIN.
	config.Opt.ELF.Expand = true

	config.Opt.ELF.NumGoroutine = runtime.NumCPU() * 2
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

	c := &config.Config{
		Resolver: elf.NewResolver(opt.Rootfs),
		Prefix:   opt.Rootfs,
		Vars:     []string(varX),
	}
	if err := c.Resolver.ReadConfig(opt.Ldconf); err != nil {
		log.Fatalln("ld.so.conf:", err)
	}

	if config.Opt.Path == nil {
		config.Opt.Path = strings.Split(os.Getenv("PATH"), ":")
	}

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

	stdin, stdout := isterm(os.Stdin), isterm(os.Stdout)
	if flag.NArg() < 1 && stdin {
		log.Fatal("not enough arguments")
	}

	root, err := loadTree(c, flag.Args(), !stdin && flag.NArg() == 0)
	if err != nil {
		log.Fatal(err)
	}

	if opt.Print {
		printTree(root, opt.Base64)
		os.Exit(0)
	}

	var out *os.File = os.Stdout
	if opt.Out != "" {
		out = open(opt.Out)
	} else if stdout && !opt.Stdout {
		log.Fatal("stdout is terminal, use -stdout")
	}

	var (
		wr  io.Writer = out
		buf *bufio.Writer
	)
	if opt.Size > 0 {
		buf = bufio.NewWriterSize(out, opt.Size)
		wr = buf
	} else {
		buf = new(bufio.Writer)
	}

	var in archive.Writer
	switch opt.Format {
	case "tar":
		in = tar.NewWriter(wr)
	case "cpio":
		in = cpio.NewWriter(wr)
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
