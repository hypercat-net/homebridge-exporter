# Contributing to homebridge-exporter

Thanks for taking the time to contribute!

## Code of Conduct

This project is governed by the [Code of Conduct](CODE_OF_CONDUCT.md). By
participating, you are expected to uphold this code.

## Questions and bugs

- Search [existing issues](https://github.com/hypercat-net/homebridge-exporter/issues)
  before opening a new one.
- For security issues, see [SECURITY.md](SECURITY.md) — do not file public
  issues for vulnerabilities.

## Development

```bash
go test ./...
go run ./cmd/homebridge-exporter
```

Pull requests should include tests when changing behaviour. Keep changes focused
and match existing code style.

## Pull requests

1. Fork the repo and create a branch from `main`.
2. Make your changes and ensure `go test ./...` passes.
3. Open a pull request with a clear description of the change and why it is
   needed.

## License

By contributing, you agree that your contributions will be licensed under the
project's [MIT License](LICENSE).
