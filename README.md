# TON Center Block Cacher

A service that caches TON blockchain blocks from TON Center API, maintains a specified window of recent blocks, and provides HTTP endpoints for block data access.

## Features

- Continuously syncs blocks from TON Center API
- Maintains a configurable window of recent blocks
- Automatically cleans up old blocks
- Provides HTTP endpoints for block data access
- Handles network issues and retries automatically
- Verifies block integrity periodically

## Configuration

Create a `.env` file based on `.env.example`:

```env
# TON Center API Configuration
API_TONCENTER_KEY=your_api_key_here
API_TONCENTER_BASE_URL=https://toncenter.com/api/v3/events
API_TONCENTER_RPS=8
API_FETCH_LIMIT=500

# Storage Configuration
BLOCKS_SAVE_PATH=./data/blocks
MAX_SAVED_BLOCKS=604800  # Keep approximately 7 days of blocks
MAX_PARALLEL_FETCHES=5

# Timing Configuration
DELAY_ON_RETRY=100  # milliseconds
DELAY_ON_BLOCKS=10  # milliseconds

# HTTP Server Configuration
HTTP_HOST=localhost
HTTP_PORT=8080
```

## Building and Running

### Using Docker

1. Build the image:
```bash
docker build -t toncenter-block-cacher .
```

2. Run the container:
```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  --name toncenter-block-cacher \
  toncenter-block-cacher
```

### Manual Build

1. Install dependencies:
```bash
go mod download
```

2. Build:
```bash
go build -o toncenter-block-cacher ./cmd/processor
```

3. Run:
```bash
./toncenter-block-cacher
```

## HTTP Endpoints

### Get Available Blocks
```bash
curl http://localhost:8080/blocks/available
```
Returns information about currently available blocks:
```json
{
  "blocks": [12345, 12346, 12347, ...],
  "count": 1000,
  "first": 12345,
  "last": 13345
}
```

### Get Specific Block
```bash
curl http://localhost:8080/blocks/12345
```
Returns the specified block if available.

### Get Block Range
```bash
curl "http://localhost:8080/blocks/range?start=12345&end=12350"
```
Returns blocks within the specified range (maximum 50 blocks).


## Error Handling

- The service automatically retries failed requests with exponential backoff
- Invalid blocks are automatically repaired during periodic verification
- HTTP endpoints return appropriate status codes and error messages

## Monitoring

The service logs important events including:
- Block processing status
- Sync progress
- Error conditions
- HTTP server status

## Limitations

- Maximum range request size is 50 blocks
- Maintains only the configured number of recent blocks
- Stays 5 blocks behind the chain head for finality
- Rate limited according to TON Center API limitations