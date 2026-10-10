# Agent instructions

## Repository

- This is the Go module `go.osspkg.com/markdown` (Go 1.26), implemented in a
  single root package named `markdown`.
- `api.go` defines the public converter, token, option, rule, and handler APIs.
  `parser.go` and `inline.go` implement parsing; `renderer.go` implements
  built-in HTML rendering; `cursor.go` provides parser-rule context.
- `html_to_markdown.go` implements HTML5-to-GFM conversion and its custom
  HTML handler and rule APIs.
- Tests live beside the package files. `testdata/commonmark/` and
  `testdata/gfm/` contain Markdown-to-HTML golden pairs;
  `testdata/html-to-markdown/` contains HTML/Markdown golden pairs. All are
  used by `golden_test.go`.
- `.github/workflows/ci.yml` runs `make ci` with Go 1.26.

## Commands

Run commands from the repository root.

- Run focused package tests with `go test ./...`.
- Run `make lint` for `goppy lint`.
- Run `make tests` for `goppy test`, benchmarks, and the two 10-second fuzzing
  runs. This target is broader and slower than `go test ./...`.
- Run `make build` for `goppy build --arch=amd64`.
- Run `make ci` for the CI sequence: install/setup, license, lint, tests, and
  build. `make install` installs `goppy` at `latest` and runs
  `goppy setup-lib`.

## Change guidance

- Preserve the public API and built-in token semantics unless the requested
  change requires an API change. Update examples and tests when public behavior
  changes.
- Keep parsing rules in the block or inline phase as appropriate. Custom
  matched rules must consume positive UTF-8 byte lengths; block rules consume
  whole lines. Preserve deterministic priority ordering.
- Keep HTML escaping and URL validation behavior covered when changing the
  renderer. Raw HTML is escaped by default, and custom handlers are trusted
  code.
- Configure HTML-to-Markdown handlers and rules before first use. Rules run by
  descending priority, with later registrations first for ties.
- Preserve HTML conversion behavior: HTML5 recovery, head metadata frontmatter,
  resource limits, safe URLs, and omission of comments, scripts, and styles.
- Update the relevant golden `.md` and `.html` pair together when changing
  expected rendered output.
- Do not edit `testdata/fuzz/` corpus files as ordinary source; `.gitignore`
  excludes that directory because Go fuzzing writes there locally.
- Keep `README.md` examples and documented limits aligned with `api.go` and
  renderer behavior.

## Boundaries

- Do not add credentials, local environment values, or generated build output
  to repository documentation or source.
- `make install` and `make ci` can install tooling and access the network; use
  narrower commands when only a focused check is needed.
