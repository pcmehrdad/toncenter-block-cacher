# TON Block Processor

A Go application for processing TON blockchain blocks and storing their events data.

## Setup

1. Clone the repository
2. Copy `.env.example` to `.env` and fill in your values
3. Install dependencies: `go mod download`
4. Build: `go build -o bin/processor cmd/processor/main.go`
5. Run: `./bin/processor`

## Configuration

The application uses environment variables for configuration. See `.env.example` for available options.

## Project Structure

- `cmd/`: Application entrypoints
- `internal/`: Internal packages
  - `config/`: Configuration handling
  - `models/`: Data structures
  - `api/`: API client
  - `processor/`: Block processing logic
  - `utils/`: Utility functions

## Future Improvements

- Add current block number fetching from external source
- Implement continuous processing of new blocks
- Add metrics and monitoring
- Add tests
- Add logging system
- Add CLI arguments for flexible configuration