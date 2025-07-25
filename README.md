# K8s Node Diagnostor

A Kubernetes node diagnostic tool that collects and reports node-level diagnostic data with configurable data scope.

## Architecture

This project follows Clean Architecture principles with clear separation of concerns:

```
├── cmd/
│   └── node-diagnostor/          # Application entry point
├── internal/
│   ├── config/                   # Configuration management
│   ├── collector/                # Data collection interfaces
│   ├── processor/                # Data processing and reporting
│   ├── scheduler/                # Task scheduling
│   └── validator/                # Data scope validation
├── pkg/                          # Shared packages
├── configs/                      # Configuration files
├── test/                         # Test files
└── templates/                    # Configuration templates
```

## Quick Start

### Build
```bash
make build
```

### Run
```bash
sudo ./bin/node-diagnostor
```

### Test
```bash
make test
```

## Development

### Project Structure
- **Clean Architecture**: All business logic is in `internal/` with clear interfaces
- **Interface-driven**: All components communicate through well-defined interfaces
- **Testable**: Each component can be tested in isolation
- **Extensible**: New collectors and processors can be added easily

### Adding New Collectors
1. Implement the `collector.Collector` interface
2. Register in the collector registry
3. Add configuration in the config file

### Configuration
See `config.example.json` for configuration examples.

## Requirements
- Go 1.21+
- Linux (for system-level diagnostics)
- Root privileges (for accessing system information)