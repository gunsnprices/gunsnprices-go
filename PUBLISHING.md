# Publishing to pkg.go.dev

Module: `github.com/gunsnprices/gunsnprices-go`, package `gunsnprices`, command `cmd/gunsnprices`.
Source: https://github.com/gunsnprices/gunsnprices-go (public). Release tag: `v0.1.0`.

Go has no registry account or upload step. A version exists as soon as its tag is on GitHub; pkg.go.dev shows it after
the Go module proxy (proxy.golang.org) has fetched it once. The proxy fetches a version the first time anyone asks for
it (`go get`, `go install`, `go list -m`, or the "Request" button on pkg.go.dev). Nobody has asked yet: the repository
was created, `v0.1.0` was tagged and CI passed on 2026-11-16, and nothing was sent to proxy.golang.org or pkg.go.dev.

## What is permanent

- The proxy keeps every version it has fetched, and the checksum database (sum.golang.org) records the checksum of the
  module zip. After the first fetch, `v0.1.0` must never be moved or re-tagged: users would get checksum errors.
- A version can't be deleted. A broken version can be *retracted*: add `retract v0.1.0 // reason` to `go.mod`, release
  `v0.1.1`, and fetch it. `go get` then skips the retracted version and pkg.go.dev marks it.
- The module path is part of every import. Renaming the repository later means a new module path.

## Before the first fetch

1. Check that the tag points at the commit you want and that CI passed on it:

   ```bash
   git ls-remote https://github.com/gunsnprices/gunsnprices-go.git refs/tags/v0.1.0
   gh run list -R gunsnprices/gunsnprices-go -L 5
   ```

2. Optional: check what the module zip will contain (the repository at the tag, without `.git`; data and test files
   included, about 1.4 MB):

   ```bash
   git clone -q --branch v0.1.0 https://github.com/gunsnprices/gunsnprices-go.git /tmp/gnp-go && cd /tmp/gnp-go
   go test ./...
   ```

## Publish (the one step)

Run from a directory **outside** this module (inside it, `go list` refuses to look up the main module's own version):

```bash
cd "$(mktemp -d)" && GOPROXY=proxy.golang.org go list -m github.com/gunsnprices/gunsnprices-go@v0.1.0
```

It prints `github.com/gunsnprices/gunsnprices-go v0.1.0`. This is the command from the Go documentation
("Publishing a module", https://go.dev/doc/modules/publishing). pkg.go.dev usually shows the module within minutes:
https://pkg.go.dev/github.com/gunsnprices/gunsnprices-go. If the page says the module is not found after half an
hour, open https://pkg.go.dev/github.com/gunsnprices/gunsnprices-go@v0.1.0 and press "Request".

Then check, also outside the module:

```bash
go install github.com/gunsnprices/gunsnprices-go/cmd/gunsnprices@v0.1.0
gunsnprices "Glock 19 Gen 5 MOS 9mm 4.02in 15rd Pistol"
```

## What pkg.go.dev shows

- The README (rendered from `README.md`), the package documentation from `doc.go`, every exported identifier, and the
  `Example...` functions of `example_test.go` as runnable examples.
- **License: MIT**, "Redistributable license". pkg.go.dev hides the documentation of a module whose license it can't
  detect or that isn't redistributable. It scans files named like `LICENSE` (not `LICENSE-DATA`) with
  github.com/google/licensecheck and needs at least 75% of the file to be license text. Checked before release with
  licensecheck v0.3.1 and the rules of golang/pkgsite `internal/licenses`: `LICENSE` is MIT with 98.8% coverage; no
  other file in the module has a license file name. `LICENSE-DATA` (CC BY 4.0 for `data/*.json`) is not scanned and
  doesn't affect this.
- Version v0.1.0, the Go version from `go.mod` (1.26), the `cmd/gunsnprices` command and links to the repository. The
  module has no dependencies outside the standard library (`go.mod` has no `require` lines).

## Later releases

1. Update `Version` in `gunsnprices.go` and add a section to `CHANGELOG.md`.
2. Commit, push, wait for CI, then tag and push the tag:

   ```bash
   git tag -a v0.1.1 -m "v0.1.1" && git push origin v0.1.1
   ```

3. Run the publish command above with the new version.

New data (`data/*.json`) comes from the Python package: copy both files, run the golden test and the crosscheck
(README, "Development"), and release a new version.

## After the first release

- Add the Go module to https://gunsnprices.com/developers next to the other languages
  (`go get github.com/gunsnprices/gunsnprices-go`, link to pkg.go.dev).
