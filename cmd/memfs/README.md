# HelloFS

A simple volatile FUSE filesystem.

To build:
```console
% go get
% go build
```

To run:
```console
% mkdir -p /tmp/mem
% ./memfs /tmp/mem
```

Verify:
```console
% ls -lia /tmp/mem
total 0
      1 drwxrwxrwx   0 root  wheel    0 Oct 27 20:47 .
2565401 drwxrwxrwt  11 root  wheel  352 Oct 27 20:47 ..
% echo "this is a test" > /tmp/mem/file
% mkdir /tmp/mem/dir
% mv /tmp/mem/file /tmp/mem/dir/file2
% cat /tmp/mem/dir/file2
this is a test
% ln /tmp/mem/dir/file2 /tmp/mem/file3
% echo "it gets around" >> /tmp/mem/dir/file2
% rm /tmp/mem/dir/file2
% ln -s ../file3 /tmp/mem/dir/file4
% cat /tmp/mem/dir/file4
this is a test
it gets around
```

Shutdown:
```console
% umount /tmp/mem
```

