# ControlMate Utils

A Go-based utility application for headless Debian systems to manage network interfaces and WiFi connections through a web interface.

## Features

- **Network Interface Management**: List all network interfaces with their IP addresses and status
- **WiFi Network Scanning**: Scan and display available WiFi networks with signal strength and security information
- **WiFi Connection**: Connect to WiFi networks with password and security type selection
- **Modern Web UI**: Clean, responsive interface built with Tailwind CSS and modern design patterns
- **Single Executable**: Compiles to a single binary file for easy deployment

## Requirements

### Development Dependencies
- **Node.js** (v22 or higher) - for Tailwind CSS compilation
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

# Development mode (watch CSS changes)
npm run dev

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

### Running the Application on Linux
```bash
# Add permissions for development
sudo setcap 'cap_net_raw,cap_net_admin=eip' ./release/cm-utils-linux-arm64

# Run the application
sudo ./release/cm-utils-linux-arm64

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
├── src/
│   ├── main.go                    # Main application code
│   ├── server/
│   │   ├── discovery/             # Discovery agent
│   │   │   └── agent.go
│   │   └── extension/             # Extension card management
│   │       ├── index.go
│   │       ├── manager.go
│   │       ├── models.go
│   │       └── port.go
│   └── web/
│       ├── static/
│       │   ├── app.js             # Frontend JavaScript
│       │   └── styles.css         # Tailwind CSS source
│       ├── templates/             # HTML templates
│       │   ├── extension.html
│       │   ├── index.html
│       │   ├── processes.html
│       │   └── system.html
│       └── styles.css
├── go.mod                          # Go module file
├── package.json                   # Node.js dependencies
└── README.md                      # This file
```

## Deployment

The application compiles to a single executable file (~9.8MB) that includes:
- All Go dependencies
- HTML templates (embedded)
- CSS and JavaScript assets (embedded)
- No external file dependencies required at runtime

The executable is completely self-contained - simply copy the `control-mate-utils` binary to your target system and run with appropriate permissions. No need to copy template or static files separately.

## Uninstallation

### If Installed via Install Script

If you installed ControlMate Utils using the install script, you can uninstall it using:

```bash
curl -sL https://raw.githubusercontent.com/controlx-io/control-mate-utils/refs/heads/main/scripts/install_to_linux.sh | sudo -E bash -s -- uninstall
```

This will:
- Stop and disable the systemd service
- Remove the service file
- Remove the binary from `/var/lib/cm-utils/`
- Remove the symlink from `/usr/local/bin/cm-utils`
- Remove polkit rule files
- Remove sudoers configuration
- Optionally remove the `cm-utils` system user (with confirmation)

### Manual Uninstallation

If you installed the binary manually (without the install script):

1. **Stop any running instances:**
   ```bash
   # If running as a systemd service
   sudo systemctl stop cm-utils
   sudo systemctl disable cm-utils
   sudo rm /etc/systemd/system/cm-utils.service
   sudo systemctl daemon-reload
   ```

2. **Remove the binary:**
   ```bash
   # Remove the binary file (adjust path if different)
   sudo rm /path/to/cm-utils
   
   # Remove symlink if created
   sudo rm /usr/local/bin/cm-utils
   ```

3. **Remove configuration files (if any):**
   ```bash
   # Remove application directory
   sudo rm -rf /var/lib/cm-utils
   
   # Remove polkit rules (if created)
   sudo rm /etc/polkit-1/rules.d/55-allow-full-network-management.rules
   sudo rm /etc/polkit-1/rules.d/56-allow-power-management.rules
   
   # Remove sudoers file (if created)
   sudo rm /etc/sudoers.d/99-cm-utils-reboot
   ```

4. **Remove system user (optional):**
   ```bash
   sudo userdel -r cm-utils
   ```

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
