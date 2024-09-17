# goptimizer
Optimizes go code using betteralign.

This is a wrapper around the betteralign and go tooling. This will copy all files under the
current `go.mod` file to a temporary directory, go vendor all the coee, run `betteralign` on them
on all packages and then use `go` to build the binary. The binary is then copied back to the
original directory.

You must not have a binary in the current directory or nothing will be done.

You may pass flags to the `go` tool, however punctuation is slightly different.

## Installation
```bash
go install github.com/dkorunic/betteralign/cmd/betteralign@latest
go install github.com/johnsiilver/goptimizer@latest
```

## Running notes

This will ignore any package that imports `reflect`, as we have found it does not reliably work
for those packages. It will ignore generated files, though there is a flag that will allow this.

There is also a flag to make sure that tests are working.  This will run `go test` on the code.

This program is quite slow, so it should only be done as an optimization step before a release.

## Usage
```bash
goptimizer [flags]
```

Simply run `goptimizer` in the directory of your go main file. This only works with go modules.
