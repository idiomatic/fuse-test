// Serve a minimally-useful volatile FUSE filesystem in memory.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type FS struct{}

type Node interface {
	fs.Node
	dirent(name string) fuse.Dirent
}

// attr differentiates from embedded type nodetype.fuse.Attr and
// function main.nodetype.Attr().
type attr fuse.Attr

type Dir struct {
	attr
	children map[string]Node
}

type File struct {
	attr
	content []byte
}

type Symlink struct {
	attr
	target string
}

var (
	lastInode fuse.NodeID = 0
	root                  = newDir(0777)
	mutex     sync.RWMutex
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
		fuse.FSName("mem"),
		fuse.Subtype("memfs"),
		fuse.LocalVolume(),
		fuse.VolumeName("Volatile Storage"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// gracefully shutdown on ctrl-c
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
		for {
			<-sigs
			fuse.Unmount(mountpoint)
		}
	}()

	// blocks until spontaneous or signalled unmount
	err = fs.Serve(c, FS{})
	if err != nil {
		log.Fatal(err)
	}

	<-c.Ready
	if err := c.MountError; err != nil {
		log.Fatal(err)
	}
}

func (FS) Root() (fs.Node, error) {
	return root, nil
}

func (d *Dir) dirent(name string) fuse.Dirent {
	return fuse.Dirent{
		Inode: uint64(d.Inode),
		Type:  fuse.DT_Dir,
		Name:  name,
	}
}

func (d *Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	mutex.RLock()
	defer mutex.RUnlock()

	*a = fuse.Attr(d.attr)
	return nil
}

func (d *Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	mutex.RLock()
	defer mutex.RUnlock()

	if child, ok := d.children[name]; ok {
		return child, nil
	}
	return nil, fuse.ENOENT
}

func (d *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	mutex.RLock()
	defer mutex.RUnlock()

	var dir []fuse.Dirent
	for name, child := range d.children {
		dir = append(dir, child.dirent(name))
	}
	return dir, nil
}

func (d *Dir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	mutex.Lock()
	defer mutex.Unlock()

	if _, found := d.children[req.Name]; found {
		return nil, fuse.EEXIST
	}
	child := newDir(req.Mode)
	d.children[req.Name] = child
	d.Mtime = time.Now()
	return child, nil
}

func (d *Dir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	mutex.Lock()
	defer mutex.Unlock()

	child := newFile(req.Mode)
	d.children[req.Name] = child
	d.Mtime = time.Now()
	return child, child, nil
}

func (d *Dir) Link(ctx context.Context, req *fuse.LinkRequest, old fs.Node) (fs.Node, error) {
	mutex.Lock()
	defer mutex.Unlock()

	if old, ok := old.(*File); ok {
		d.children[req.NewName] = old
		d.Mtime = time.Now()
	}
	return old, nil
}

func (d *Dir) Symlink(ctx context.Context, req *fuse.SymlinkRequest) (fs.Node, error) {
	mutex.Lock()
	defer mutex.Unlock()

	link := newSymlink(req.Target)
	d.children[req.NewName] = link
	d.Mtime = time.Now()
	return link, nil
}

func (d *Dir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	mutex.Lock()
	defer mutex.Unlock()

	if dir, isDir := d.children[req.Name].(*Dir); isDir {
		if len(dir.children) > 0 {
			// target is not empty
			return fuse.EEXIST
		}
	}
	delete(d.children, req.Name)
	d.Mtime = time.Now()
	return nil
}

func (d *Dir) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	mutex.Lock()
	defer mutex.Unlock()

	if destDir, ok := newDir.(*Dir); ok {
		target := d.children[req.OldName]
		delete(d.children, req.OldName)
		d.Mtime = time.Now()
		destDir.children[req.NewName] = target
		destDir.Mtime = time.Now()
		return nil
	}
	return fuse.ENOENT
}

func (d *Dir) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	mutex.Lock()
	defer mutex.Unlock()

	const (
		handled = fuse.SetattrMode | fuse.SetattrMtime
		ignored = fuse.SetattrAtime | fuse.SetattrHandle
	)
	if req.Valid & ^handled & ^ignored != 0 {
		log.Fatal("dir setattr unrecognized ", req)
	}

	if req.Valid.Mode() {
		d.Mode = req.Mode
	}
	if req.Valid.Mtime() {
		d.Mtime = req.Mtime
	}
	return nil
}

func (f *File) dirent(name string) fuse.Dirent {
	return fuse.Dirent{
		Inode: uint64(f.Inode),
		Type:  fuse.DT_File,
		Name:  name,
	}
}

func (f *File) Attr(ctx context.Context, a *fuse.Attr) error {
	mutex.RLock()
	defer mutex.RUnlock()

	*a = fuse.Attr(f.attr)
	a.Size = uint64(len(f.content))
	return nil
}

func (f *File) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	mutex.RLock()
	defer mutex.RUnlock()

	resp.Data = f.content[req.Offset:][:req.Size]
	return nil
}

func (f *File) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	mutex.Lock()
	defer mutex.Unlock()

	contentLen := int64(len(f.content))

	padLen := req.Offset - contentLen
	if padLen > 0 {
		// pad with nuls
		pad := make([]byte, padLen)
		f.content = append(f.content, pad...)
		contentLen = req.Offset
	}

	// retain following data if random access writes
	var after []byte
	afterPos := req.Offset + int64(len(req.Data))
	if afterPos < contentLen {
		after = f.content[afterPos:]
	}

	f.content = append(f.content[:req.Offset], req.Data...)
	if after != nil {
		f.content = append(f.content, after...)
	}

	resp.Size = len(req.Data)
	f.Mtime = time.Now()
	return nil
}

func (f *File) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	mutex.Lock()
	defer mutex.Unlock()

	const (
		handled = fuse.SetattrSize | fuse.SetattrMode | fuse.SetattrMtime
		ignored = fuse.SetattrAtime | fuse.SetattrHandle | fuse.SetattrUid |
			fuse.SetattrGid | fuse.SetattrFlags
	)
	if req.Valid & ^handled & ^ignored != 0 {
		log.Fatal("file setattr unrecognized ", req)
	}

	if req.Valid.Size() {
		delta := int(req.Size) - len(f.content)
		if delta < 0 {
			// truncate file
			f.content = f.content[:req.Size]
		} else if delta > 0 {
			// pad with nuls
			pad := make([]byte, delta)
			f.content = append(f.content, pad...)
		}
	}
	if req.Valid.Mode() {
		f.Mode = req.Mode
	}
	if req.Valid.Mtime() {
		f.Mtime = req.Mtime
	}
	return nil
}

func (f *File) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
	//log.Printf("%T.Fsync(%s, %s)", f, f, req)
	return nil
}

func (l *Symlink) dirent(name string) fuse.Dirent {
	return fuse.Dirent{
		Inode: uint64(l.Inode),
		Type:  fuse.DT_Link,
		Name:  name,
	}
}

func (l *Symlink) Attr(ctx context.Context, a *fuse.Attr) error {
	mutex.RLock()
	defer mutex.RUnlock()

	*a = fuse.Attr(l.attr)
	return nil
}

func (l *Symlink) Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error) {
	return l.target, nil
}

//func newNode(mode os.FileMode) attr {
func newNode(mode os.FileMode) attr {
	// caller locks
	lastInode++
	now := time.Now()
	return attr{
		Inode: uint64(lastInode),
		Mode:  mode,
		Ctime: now,
		Mtime: now,
	}
}

func newDir(mode os.FileMode) *Dir {
	return &Dir{
		attr:     newNode(os.ModeDir | mode),
		children: make(map[string]Node),
	}
}

func newFile(mode os.FileMode) *File {
	return &File{
		attr: newNode(mode),
	}
}

func newSymlink(target string) *Symlink {
	return &Symlink{
		attr:   newNode(os.ModeSymlink | 0444),
		target: target,
	}
}
