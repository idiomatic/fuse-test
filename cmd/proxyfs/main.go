package main

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type FileSystem string
type Dir string
type File string
type Symlink string

func main() {
	var (
		mountpoint = os.Args[1]
		storage    = os.Args[2]
	)

	c, _ := fuse.Mount(
		mountpoint,
		fuse.FSName("proxy"),
		fuse.Subtype("proxyfs"),
		fuse.LocalVolume(),
		fuse.ReadOnly(),
		fuse.VolumeName("Proxy Filesystem"),
	)
	defer c.Close()

	_ = fs.Serve(c, FileSystem(storage))
}

func readonly(mode os.FileMode) os.FileMode {
	return mode & ^os.FileMode(0222)
}

func (fs FileSystem) Root() (fs.Node, error) {
	return Dir(fs), nil
}

func (d Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Mode = readonly(os.ModeDir | 0777)
	info, err := os.Stat(string(d))
	if err != nil {
		return err
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		a.Inode = stat.Ino
	}
	return nil
}

func (d Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	p := filepath.Join(string(d), name)
	info, err := os.Stat(string(p))
	if err != nil {
		return nil, err
	}
	switch mode := info.Mode(); {
	case mode.IsDir():
		return Dir(p), nil
	case mode&os.ModeSymlink != 0:
		return Symlink(p), nil
	case mode.IsRegular():
		return File(p), nil
	}
	return nil, fuse.ENOENT
}

func direntType(mode os.FileMode) fuse.DirentType {
	switch {
	case mode.IsDir():
		return fuse.DT_Dir
	case mode&os.ModeSymlink != 0:
		return fuse.DT_Link
	case mode.IsRegular():
		return fuse.DT_File
	}
	return fuse.DT_Unknown
}

func (d Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	entries, err := ioutil.ReadDir(string(d))
	if err != nil {
		return nil, err
	}
	var dir []fuse.Dirent
	for _, entry := range entries {
		dirent := fuse.Dirent{
			Type: direntType(entry.Mode()),
			Name: entry.Name(),
		}
		if stat, ok := entry.Sys().(*syscall.Stat_t); ok {
			dirent.Inode = stat.Ino
		}
		dir = append(dir, dirent)
	}
	return dir, nil
}

/*
func (d Dir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	f := File(filepath.Join(string(d), req.Name))
	// XXX touch?
	return f, f, nil
}
*/

func (f File) Attr(ctx context.Context, a *fuse.Attr) error {
	info, err := os.Stat(string(f))
	if err != nil {
		return err
	}
	a.Size = uint64(info.Size())
	a.Mtime = info.ModTime()
	a.Mode = readonly(info.Mode())
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		a.Inode = stat.Ino
		//a.Atime = stat.Atim
		//a.Ctime = stat.Ctim
		a.Nlink = uint32(stat.Nlink)
		a.Uid = stat.Uid
		a.Gid = stat.Gid
	}
	return nil
}

func (f File) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	actual, err := os.Open(string(f))
	if err != nil {
		return err
	}
	defer actual.Close()
	_, err = actual.Seek(req.Offset, os.SEEK_SET)
	if err != nil {
		return err
	}
	buf := make([]byte, req.Size)
	_, err = io.ReadFull(actual, buf)
	if err != nil {
		return err
	}
	resp.Data = buf
	return nil
}

func (f File) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	actual, err := os.Open(string(f))
	if err != nil {
		return err
	}
	defer actual.Close()
	_, err = actual.Seek(req.Offset, os.SEEK_SET)
	if err != nil {
		return err
	}
	_, err = actual.Write(req.Data)
	if err != nil {
		return err
	}
	return nil
}

/*
func (f File) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	return nil
}
*/

func (l Symlink) Attr(ctx context.Context, a *fuse.Attr) error {
	return nil
}

func (l Symlink) Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error) {
	return "", nil
}
