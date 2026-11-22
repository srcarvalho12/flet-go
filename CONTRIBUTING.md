# Contributing to flet-go

Thanks for your interest! flet-go is in an early stage, so every kind of help
makes a difference:

- **Try it** and [open an issue](https://github.com/srcarvalho12/flet-go/issues)
  with bugs, rough edges or missing controls.
- **Improve the docs** or write an example.
- **Test on other platforms**: Windows and Linux need more testing.
- **Pick a roadmap item** from [PLAN.md](PLAN.md): the web production build,
  Linux client download, mobile, extension packages...

For bigger changes, open an issue first so we can agree on the approach.

## Development

```bash
git clone https://github.com/srcarvalho12/flet-go
cd flet-go
go test -race ./...
go run ./examples/gallery
```

Tests do not need the Flet client: `page_test.go` has a fake client that applies
patches like the real one.

## Generated code

Files ending in `_gen.go` and the `colors` and `icons` packages are generated. Do
not edit them by hand; change `tools/codegen` and run:

```bash
go generate ./...
```

When the Flet version changes, the spec is extracted again from a Flet checkout:

```bash
python3 -I tools/specgen/extract.py <flet-checkout> spec/flet.json
go generate ./...
```

## Guidelines

- Code, comments, commit messages and docs are in English.
- Run `gofmt`, `go vet ./...` and `go test -race ./...` before sending a pull
  request.
- Add a test for each change in behavior. Changes to what the client sees can be
  checked with the fake client (`assertInSync`).
- Keep the public API small and close to Flet's names.
