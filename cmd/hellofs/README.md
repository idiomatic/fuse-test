# HelloFS

A trivial read-only FUSE filesystem with a greeting.

To build:
```console
% go get
% go build
```

To run:
```console
% mkdir -p /tmp/hello
% ./hellofs /tmp/hello
```

Verify:
```console
% ls -lia /tmp/hello
total 8
      1 dr-xr-xr-x   1 root  wheel    0 Oct 27 20:26 .
2565401 drwxrwxrwt  10 root  wheel  320 Oct 27 20:26 ..
      2 -r--r--r--   1 root  wheel   16 Oct 27 20:26 hello
% cat /tmp/hello/hello
hello from fuse
```

Shutdown:
```console
% umount /tmp/hello
% rmdir /tmp/hello
```

