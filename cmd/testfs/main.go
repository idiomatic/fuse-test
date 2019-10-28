package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

func Usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s MOUNTPOINT\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	flag.Usage = Usage
	flag.Parse()

	if flag.NArg() != 1 {
		Usage()
		os.Exit(2)
	}
	mountpoint := flag.Arg(0)

	c, err := fuse.Mount(
		mountpoint,
		fuse.FSName("test"),
		fuse.Subtype("testfs"),
		fuse.LocalVolume(),
		fuse.VolumeName("Experimental"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	go func() {
		sigs := make(chan os.Signal, 1)
		// "go run" and "killall -INT go" promotes these signals to uncatchable signals
		signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		fuse.Unmount(mountpoint)
	}()

	log.Println("serving")
	err = fs.Serve(c, FS{})
	if err != nil {
		log.Fatal(err)
	}

	<-c.Ready
	if err := c.MountError; err != nil {
		log.Fatal(err)
	}
}

var (
	rootDir     = NewDir(1, "/")
	subDir      = NewDir(2, "bar")
	helloFile   = NewFile(3, "hello")
	burriedFile = NewFile(4, "burried")
	content     = map[Node][]byte{
		helloFile:   []byte("hello from fuse\n"),
		burriedFile: []byte("nothing to see here\n"),
	}
	// cannot distribute children into Node else Node is unusable as a key
	children = map[Node][]Node{
		rootDir: []Node{
			subDir,
			helloFile,
		},
		subDir: []Node{
			burriedFile,
		},
	}
)

type FS struct{}

func (FS) Root() (fs.Node, error) {
	return rootDir, nil
}

/*
func (f FS) GenerateInode(parentInode uint64, name string) uint64 {
}
*/

type Node fuse.Dirent

func NewDir(inode uint64, name string) Node {
	return Node{Inode: inode, Type: fuse.DT_Dir, Name: name}
}

func NewFile(inode uint64, name string) Node {
	return Node{Inode: inode, Type: fuse.DT_File, Name: name}
}

/*
type Node struct {
	fuse.Dirent
}

func NewDir(inode uint64, name string) Node {
	return Node{fuse.Dirent{Inode: inode, Type: fuse.DT_Dir, Name: name}}
}

func NewFile(inode uint64, name string) Node {
	return Node{fuse.Dirent{Inode: inode, Type: fuse.DT_File, Name: name}}
}
*/

func (n Node) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = n.Inode
	if c, ok := content[n]; ok {
		a.Size = uint64(len(c))
		a.Mode = 0666
		return nil
	}
	if _, ok := children[n]; ok {
		a.Mode = os.ModeDir | 0777
		return nil
	}
	return fuse.ENOENT
}

func (n Node) Lookup(ctx context.Context, name string) (fs.Node, error) {
	if dir, ok := children[n]; ok {
		for _, child := range dir {
			if child.Name == name {
				return child, nil
			}
		}
	}
	return nil, fuse.ENOENT
}

func (n Node) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	var dir []fuse.Dirent

	if _, ok := children[n]; !ok {
		return nil, fuse.ENOENT
	}

	for _, child := range children[n] {
		//dir = append(dir, child.Dirent)
		dir = append(dir, fuse.Dirent(child))
	}

	return dir, nil
}

func (n Node) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	//log.Printf("read %s\n", req)

	resp.Data = content[n][req.Offset:][:req.Size]
	return nil
}

func (n Node) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) error {
	log.Printf("create %s\n", req)

	return nil
}

func (n Node) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	log.Printf("write %s\n", req)

	data := content[n]
	if data == nil {
		data = []byte{}
	}
	if int64(len(data)) < req.Offset {
		log.Println("XXX pad data")
	}
	before := data[:req.Offset]
	after := data[req.Offset+int64(len(req.Data)):]
	data = append(append(before, req.Data...), after...)

	content[n] = data

	return nil
}
