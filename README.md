# Cryptowatch

Cryptocurrency price monitoring tool with push notifications.

## Features

- Monitor multiple cryptocurrency prices via CoinMarketCap
- WebSocket real-time price updates
- Push notifications on configurable price thresholds
- Systemd service integration

## Installation

### Build from source
```bash
go build -o /usr/local/bin/cryptowatch
```

### System setup
```bash
# Create user and directories
sudo useradd -r -s /bin/false -d /var/lib/cryptowatch cryptowatch
sudo mkdir -p /var/lib/cryptowatch /var/log/cryptowatch /etc/cryptowatch
sudo chown cryptowatch:cryptowatch /var/lib/cryptowatch /var/log/cryptowatch

# Install service
sudo cp cryptowatch.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable cryptowatch
```

## Configuration

### Binary setup
```bash
# Install binary
sudo mv cryptowatch /usr/local/bin/
sudo chown root:root /usr/local/bin/cryptowatch
sudo chmod 755 /usr/local/bin/cryptowatch
```

### Environment configuration
Copy environment template:
```bash
sudo cp cryptowatch.env.example /etc/cryptowatch/cryptowatch.env
sudo chown root:cryptowatch /etc/cryptowatch/cryptowatch.env
sudo chmod 640 /etc/cryptowatch/cryptowatch.env
```

Edit `/etc/cryptowatch/cryptowatch.env`:
```bash
CRYPTOWATCH_COINS=1,1027,1839          # Bitcoin, Ethereum, BNB
CRYPTOWATCH_PUSH_TOKENS=o.YOUR_TOKEN
CRYPTOWATCH_DROP_THRESHOLD=5           # Alert on 5% drop
CRYPTOWATCH_RISE_THRESHOLD=10          # Alert on 10% rise
```

## Usage

```bash
# Start service
sudo systemctl start cryptowatch

# Check status
sudo systemctl status cryptowatch

# View logs
journalctl -u cryptowatch -f
```

## Development

```bash
# Version management
make version              # Show current version
make release-patch        # Bump patch version
make release-minor        # Bump minor version
make release-major        # Bump major version
```

## Service Configuration

The service runs as user `cryptowatch` with restricted permissions:
- Home: `/var/lib/cryptowatch`
- Logs: `/var/log/cryptowatch` 
- Config: `/etc/cryptowatch/cryptowatch.env`
- Security: No new privileges, private tmp, protected system/home