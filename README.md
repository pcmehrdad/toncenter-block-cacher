# TON Center Block Cacher

A service that caches TON blockchain blocks from TON Center API, maintaining a configurable window of recent blocks with HTTP endpoints for data access.

## Features

- Continuous block syncing from TON Center API
- Configurable block window size
- Automatic cleanup of old blocks
- Rate limiting for API requests
- HTTP endpoints for block data access
- Automatic recovery and retry mechanisms
- Support for runtime configuration updates

## Quick Start

1. Clone the repository:
```bash
git clone https://github.com/pcmehrdad/toncenter-block-cacher.git
cd toncenter-block-cacher
```

2. Create environment file:
```bash
cp .env.example .env
```

3. Configure your environment:
```env
# Project Settings
COMPOSE_PROJECT_NAME=toncenter-block-cacher
UID=1000
GID=1000
DC_HTTP_PORT=8081

# Storage Configuration
BLOCKS_SAVE_PATH=./data/blocks

# TON Center API Configuration
API_TONCENTER_KEY=your_api_key_here
API_TONCENTER_BASE_URL=https://toncenter.com/api/v3/events
API_TONCENTER_RPS=8
API_FETCH_LIMIT=500

# Block Management
MAX_SAVED_BLOCKS=200           # Number of blocks to keep
MAX_PARALLEL_FETCHES=5

# Timing Configuration (milliseconds)
DELAY_ON_RETRY=100            # Delay before retrying failed fetches
DELAY_ON_BLOCKS=10            # Delay between block fetches
```

4. Create required directories:
```bash
mkdir -p data/blocks
```

5. Start the service:
```bash
docker-compose up -d
```

## Docker Compose Usage

### Start Service
```bash
docker-compose up -d
```

### View Logs
```bash
docker-compose logs -f
```

### Stop Service
```bash
docker-compose down
```

### Reload Configuration
To reload configuration after changing .env:
```bash
# Option 1: Send SIGHUP
docker kill -s SIGHUP $(docker-compose ps -q)

# Option 2: Restart container
docker-compose restart
```

## HTTP Endpoints

### Get Latest Block Number
```bash
curl http://localhost:8081/blocks/latest
```
Response:
```json
{
    "block": 42064764
}
```

### Get Specific Block
```bash
curl http://localhost:8081/blocks/42064764
```

### Get Available Blocks
```bash
curl http://localhost:8081/blocks/available
```
Response:
```json
{
    "blocks": [42064564, 42064565, ..., 42064764],
    "count": 200,
    "first": 42064564,
    "last": 42064764
}
```

## Configuration Details

### Block Management
- `MAX_SAVED_BLOCKS`: Number of most recent blocks to keep
    - Example: 200 for recent blocks
    - Example: 604800 for approximately 7 days of blocks

### Rate Limiting
- `API_TONCENTER_RPS`: Requests per second limit
- `API_FETCH_LIMIT`: Maximum events per request

### Timing
- `DELAY_ON_RETRY`: Milliseconds to wait before retrying failed requests
- `DELAY_ON_BLOCKS`: Milliseconds to wait between block fetches

### Docker Settings
- `UID` and `GID`: User/Group IDs for file permissions
- `DC_HTTP_PORT`: External HTTP port mapping

## Maintenance

### Adjusting Block Window
1. Update `MAX_SAVED_BLOCKS` in .env
2. Either:
    - Send SIGHUP to reload: `docker kill -s SIGHUP container_name`
    - Or restart: `docker-compose restart`

### Monitoring
View logs:
```bash
# All logs
docker-compose logs -f

# Filter for specific events
docker-compose logs -f | grep "Processed block"
```

### Data Management
Block data location:
```bash
./data/blocks/           # Default location
```

## Troubleshooting

1. If blocks are not being cleaned up:
    - Check current block count: `ls data/blocks | wc -l`
    - Verify MAX_SAVED_BLOCKS in .env
    - Restart service or send SIGHUP

2. Rate limiting issues:
    - Check logs for API errors
    - Adjust API_TONCENTER_RPS
    - Verify API key validity

3. Missing blocks:
    - Check /blocks/available endpoint
    - Verify file permissions
    - Check logs for fetch errors

## Contributing

1. Fork the repository
2. Create feature branch
3. Commit changes
4. Create Pull Request

## License

MIT License - see LICENSE file for details