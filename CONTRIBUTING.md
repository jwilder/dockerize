# Contributing

Thanks for your interest in contributing to `dockerize`.

## Project setup

This project is written in Go and produces the `dockerize` binary.

1. Install a supported Go toolchain.
2. Clone the repository.
3. From the repository root, build the project:

```sh
go build
```

If you use the provided `Makefile`, the available development targets include:

```sh
make lint
make test
make dockerize
make clean
```

The `make dockerize` target builds and installs the `dockerize` binary with version metadata by running:

```sh
go install -ldflags "$(LDFLAGS)"
```

## Running tests

Run the full test suite from the repository root with:

```sh
go test -v -race ./...
```

This is the same command used by:

```sh
make test
```

Before opening a pull request, please make sure your changes build cleanly and the relevant tests pass.

## Pull request expectations

When submitting a pull request:

- Keep changes focused and scoped to the problem being solved.
- Add or update tests when behavior changes or bugs are fixed.
- Ensure `go build ./...` succeeds.
- Ensure `go test ./...` or `make test` passes locally.
- Update documentation when user-facing behavior or usage changes.
- Include a clear description of the change and the motivation behind it.

Small, well-explained pull requests are easier to review and merge.
