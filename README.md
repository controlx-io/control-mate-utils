# Control Mate Utils

A Go-based utility application for headless Debian systems to manage network interfaces and WiFi connections through a web interface.

## Features

- **Network Interface Management**: List all network interfaces with their IP addresses and status
- **WiFi Network Scanning**: Scan and display available WiFi networks with signal strength and security information
- **WiFi Connection**: Connect to WiFi networks with password and security type selection
- **Modern Web UI**: Clean, responsive interface built with Tailwind CSS and modern design patterns
- **Single Executable**: Compiles to a single binary file for easy deployment

## Requirements

### Development Dependencies
- **Node.js** (v16 or higher) - for Tailwind CSS compilation
- **npm** - for package management

### System Dependencies
- `nmcli` - NetworkManager command line interface (recommended)
- `iwlist` - for WiFi network scanning (fallback)
- `iwconfig` - for WiFi configuration (WEP networks)
- `wpa_supplicant` - for WPA/WPA2 networks
- `dhclient` - for DHCP client functionality

### Installation on Debian/Ubuntu
```bash
sudo apt update
sudo apt install wireless-tools wpasupplicant isc-dhcp-client
```

## Building

### Prerequisites
1. Install Node.js and npm (for Tailwind CSS compilation)
2. Install Go (v1.19 or higher)

### Development Setup
```bash
# Clone the repository
git clone <repository-url>
cd control-mate-utils

# Install Node.js dependencies
npm install

# Build everything (CSS + Go binary)
npm run build
```

### Build Commands
```bash
# Build everything (CSS + Go binary) - outputs to ./build/
npm run build

# Build CSS only (for development)
npm run build-css

# Build Go binary only
npm run build-go

# Development mode (watch CSS changes)
npm run dev

# Clean build artifacts
npm run clean
```

### Build Output
All built files are placed in the `./build/` directory:
```
build/
├── control-mate-utils-linux-arm64     # Go binary (Linux ARM64) - contains embedded static files
└── static/
    ├── css/
    │   └── styles.css     # Compiled Tailwind CSS (embedded in binary)
    └── js/
        └── app.js         # JavaScript file (embedded in binary)
```

**Note**: The static files (CSS and JS) are embedded into the Go binary during compilation, so the binary is completely self-contained and doesn't require external static files to run.

## Usage

### Running the Application
```bash
# Deploy: Copy only the binary to target system
scp ./build/control-mate-utils-linux-arm64 user@target:~

# Run the application (requires root privileges for WiFi operations)
sudo ./build/control-mate-utils-linux-arm64

# The web interface will be available at http://localhost:8080
```

**Deployment**: The binary is completely self-contained. You only need to copy the `control-mate-utils-linux-arm64` file to your target Linux ARM64 system - no additional static files are required.

### Web Interface

1. **Network Interfaces**: View all network interfaces, their IP addresses, and status
2. **WiFi Networks**: Scan for available WiFi networks and view their details
3. **Connect to WiFi**: Click "Connect" on any WiFi network to enter credentials

### API Endpoints

- `GET /` - Main web interface
- `GET /api/interfaces` - Get network interfaces (JSON)
- `GET /api/wifi/scan` - Scan WiFi networks (JSON)
- `POST /api/wifi/connect` - Connect to WiFi network (JSON)

#### Connect to WiFi API
```bash
curl -X POST http://localhost:8080/api/wifi/connect \
  -H "Content-Type: application/json" \
  -d '{
    "ssid": "NetworkName",
    "password": "password123",
    "security": "WPA2"
  }'
```

## Security Types Supported

- **Open**: No password required
- **WEP**: Wired Equivalent Privacy
- **WPA/WPA2**: Wi-Fi Protected Access
- **WPA2**: Wi-Fi Protected Access 2

## File Structure

```
control-mate-utils/
├── main.go                 # Main application code
├── go.mod                  # Go module file
├── templates/
│   └── index.html         # HTML template
├── static/
│   ├── css/
│   │   └── styles.css     # Shadcn UI styles
│   └── js/
│       └── app.js         # Frontend JavaScript
└── README.md              # This file
```

## Deployment

The application compiles to a single executable file (~9.8MB) that includes:
- All Go dependencies
- HTML templates (embedded)
- CSS and JavaScript assets (embedded)
- No external file dependencies required at runtime

The executable is completely self-contained - simply copy the `control-mate-utils` binary to your target system and run with appropriate permissions. No need to copy template or static files separately.

## Permissions

The application requires root privileges to:
- Access network interface information
- Scan WiFi networks
- Configure WiFi connections
- Modify network settings

## Troubleshooting

### WiFi Scanning Issues
- Ensure `iwlist` is installed
- Check that WiFi adapter is enabled
- Verify proper permissions (run as root)

### Connection Issues
- Ensure `wpa_supplicant` is installed for WPA networks
- Check that `dhclient` is available for IP assignment
- Verify network credentials are correct

### Interface Issues
- Ensure network interfaces are properly configured
- Check system network configuration

## License

This project is open source. Feel free to modify and distribute as needed.
