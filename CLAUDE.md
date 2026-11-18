# lmao

User-facing docs are README.md and the root command's long help
(`cmd/root.go`). Keep both in sync when keys, environment variables or the
Home Manager module change.

`nix build` of the checkout leaves out untracked files. With new files not
yet committed, build with `nix build "path:$PWD"`, or the `vendorHash` (and
the tests the build runs) won't include them.

Never add a `VERSION` file to `main`: only release commits, which the
Release workflow makes and only its tags reach, have one.
