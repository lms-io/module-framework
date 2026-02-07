# Module Framework

This is a standalone template for creating automation modules with built-in support for Starlark instances, SQLite persistence, and leveled logging.

## Standard Development

1. **Write Logic:** Implement your driver in `internal/logic/module.go`.
2. **Mock Testing:** Use the built-in mock device for local verification.
   ```bash
   make test  # Runs the public integration suite
   ```

## Private Hardware Testing

For testing against real physical devices with secret keys, use the `private_tests/` directory. This folder is **Git-ignored** to prevent leaking secrets.

### Setup a Private Test
1. Create a folder: `private_tests/my-device-test/`.
2. Add a `.env.test` for your IPs and Keys.
3. Add a `hardware_test.go` using the `//go:build hardware` tag.

### Running Private Tests
```bash
# Sourcing environment variables and running specialized tests
set -a && source private_tests/my-device-test/.env.test && set +a
go test -v -tags=hardware ./private_tests/my-device-test/...
```

## Structure
- `cmd/adapter/`: Entry point (Subprocess wrapper).
- `internal/framework/`: Core SDK/Framework logic.
- `internal/logic/`: Place your integration driver here.
- `tests/`: Public integration tests and Mock Hardware.
- `private_tests/`: Git-ignored sandbox for real hardware tests.