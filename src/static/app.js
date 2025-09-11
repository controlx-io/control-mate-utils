class NetworkManager {
    constructor() {
        this.currentSSID = null;
        this.currentWiFi = null;
        this.nmcliAvailable = false;
        this.isDarkMode = localStorage.getItem('darkMode') === 'true';
        this.initializeEventListeners();
        this.initializeTheme();
        this.checkNmcliStatus();
    }

    async checkNmcliStatus() {
        try {
            const response = await fetch('/api/nmcli/status');
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const status = await response.json();
            this.nmcliAvailable = status.available;
            
            if (this.nmcliAvailable) {
                this.loadInterfaces();
                this.loadCurrentWiFi();
            } else {
                this.showNmcliError();
            }
        } catch (error) {
            console.error('Error checking nmcli status:', error);
            this.nmcliAvailable = false;
            this.showNmcliError();
        }
    }

    showNmcliError() {
        const wifiSection = document.querySelector('.wifi-section');
        if (wifiSection) {
            wifiSection.innerHTML = `
                <div class="flex items-center justify-between mb-4">
                    <h2 class="text-xl font-semibold">WiFi Networks</h2>
                </div>
                
                <div class="rounded-lg border bg-card text-card-foreground shadow-sm">
                    <div class="p-6">
                        <div class="text-center py-12 text-red-600">
                            <i data-lucide="alert-triangle" class="h-16 w-16 mx-auto mb-4"></i>
                            <h3 class="text-lg font-semibold mb-2">nmcli Not Available</h3>
                            <p class="text-sm text-muted-foreground mb-4">
                                NetworkManager command line interface (nmcli) is not installed or not available on this system.
                            </p>
                            <div class="bg-red-50 border border-red-200 rounded-lg p-4 text-left max-w-md mx-auto">
                                <h4 class="font-medium mb-2">To install nmcli:</h4>
                                <ul class="text-sm space-y-1">
                                    <li><strong>Ubuntu/Debian:</strong> <code>sudo apt install network-manager</code></li>
                                    <li><strong>CentOS/RHEL:</strong> <code>sudo yum install NetworkManager</code></li>
                                    <li><strong>Fedora:</strong> <code>sudo dnf install NetworkManager</code></li>
                                    <li><strong>Arch Linux:</strong> <code>sudo pacman -S networkmanager</code></li>
                                </ul>
                            </div>
                            <button onclick="location.reload()" class="mt-4 inline-flex items-center justify-center rounded-md text-sm font-medium ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 bg-primary text-primary-foreground hover:bg-primary/90 h-9 px-4 py-2">
                                <i data-lucide="refresh-cw" class="h-4 w-4 mr-2"></i>
                                Retry
                            </button>
                        </div>
                    </div>
                </div>
            `;
            lucide.createIcons();
        }
    }

    initializeEventListeners() {
        // Refresh interfaces button
        document.getElementById('refreshInterfaces').addEventListener('click', () => {
            this.loadInterfaces();
        });

        // Scan WiFi button
        document.getElementById('scanWiFi').addEventListener('click', () => {
            this.scanWiFiNetworks();
        });

        // Theme toggle button
        const themeToggle = document.getElementById('themeToggle');
        if (themeToggle) {
            themeToggle.addEventListener('click', () => {
                this.toggleTheme();
            });
        }

        // Connection modal events
        document.getElementById('cancelConnection').addEventListener('click', () => {
            this.hideConnectionModal();
        });

        document.getElementById('connectionForm').addEventListener('submit', (e) => {
            e.preventDefault();
            this.connectToWiFi();
        });

        // Close modal on background click
        document.getElementById('connectionModal').addEventListener('click', (e) => {
            if (e.target === e.currentTarget) {
                this.hideConnectionModal();
            }
        });
    }

    initializeTheme() {
        if (this.isDarkMode) {
            document.documentElement.classList.add('dark');
        }
        // Delay theme icon update to ensure DOM is ready
        setTimeout(() => {
            this.updateThemeIcon();
        }, 50);
    }

    toggleTheme() {
        this.isDarkMode = !this.isDarkMode;
        localStorage.setItem('darkMode', this.isDarkMode);
        
        if (this.isDarkMode) {
            document.documentElement.classList.add('dark');
        } else {
            document.documentElement.classList.remove('dark');
        }
        
        this.updateThemeIcon();
    }

    updateThemeIcon() {
        const themeToggle = document.getElementById('themeToggle');
        if (!themeToggle) {
            console.warn('Theme toggle button not found');
            return;
        }
        
        const icon = themeToggle.querySelector('i');
        const text = themeToggle.querySelector('span');
        
        if (!icon) {
            console.warn('Theme toggle icon not found');
            return;
        }
        
        if (!text) {
            console.warn('Theme toggle text not found');
            return;
        }
        
        try {
            if (this.isDarkMode) {
                icon.setAttribute('data-lucide', 'sun');
                text.textContent = 'Light Mode';
            } else {
                icon.setAttribute('data-lucide', 'moon');
                text.textContent = 'Dark Mode';
            }
            
            // Only recreate icons if lucide is available
            if (typeof lucide !== 'undefined' && lucide.createIcons) {
                lucide.createIcons();
            }
        } catch (error) {
            console.error('Error updating theme icon:', error);
        }
    }

    async loadInterfaces() {
        const loadingEl = document.getElementById('interfacesLoading');
        const listEl = document.getElementById('interfacesList');

        loadingEl.classList.remove('hidden');
        listEl.innerHTML = '';

        try {
            const response = await fetch('/api/interfaces');
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const interfaces = await response.json();
            this.renderInterfaces(interfaces);
        } catch (error) {
            console.error('Error loading interfaces:', error);
            listEl.innerHTML = `
                <div class="text-center py-8 text-red-600">
                    <i data-lucide="alert-circle" class="h-8 w-8 mx-auto mb-2"></i>
                    <p>Failed to load network interfaces</p>
                    <p class="text-sm text-muted-foreground">${error.message}</p>
                </div>
            `;
            lucide.createIcons();
        } finally {
            loadingEl.classList.add('hidden');
        }
    }

    renderInterfaces(interfaces) {
        const listEl = document.getElementById('interfacesList');
        
        if (interfaces.length === 0) {
            listEl.innerHTML = `
                <div class="text-center py-12 text-muted-foreground">
                    <div class="flex flex-col items-center space-y-4">
                        <div class="flex items-center justify-center w-16 h-16 rounded-full bg-muted">
                            <i data-lucide="network" class="h-8 w-8"></i>
                        </div>
                        <div class="text-center">
                            <p class="text-lg font-medium">No network interfaces found</p>
                            <p class="text-sm">Make sure your system has network adapters</p>
                        </div>
                    </div>
                </div>
            `;
            lucide.createIcons();
            return;
        }

        listEl.innerHTML = interfaces.map(iface => {
            const isUp = iface.status === 'up';
            const hasIPs = iface.ip_addresses && iface.ip_addresses.length > 0;
            const icon = this.getInterfaceIcon(iface.name);
            
            return `
                <div class="interface-card group h-full">
                    <div class="interface-header">
                        <div class="flex items-center space-x-2">
                            <div class="flex items-center justify-center w-8 h-8 rounded-md ${isUp ? 'bg-green-100 dark:bg-green-900/20' : 'bg-gray-100 dark:bg-gray-800'}">
                                <i data-lucide="${icon}" class="h-4 w-4 ${isUp ? 'text-green-600 dark:text-green-400' : 'text-gray-500 dark:text-gray-400'}"></i>
                            </div>
                            <div class="flex-1 min-w-0">
                                <h3 class="interface-name truncate text-sm">${this.escapeHtml(iface.name)}</h3>
                                <p class="text-xs text-muted-foreground">${this.getInterfaceType(iface.name)}</p>
                            </div>
                        </div>
                        <div class="flex items-center space-x-1">
                            <div class="w-1.5 h-1.5 rounded-full ${isUp ? 'bg-green-500' : 'bg-red-500'}"></div>
                            <span class="interface-status ${isUp ? 'status-up' : 'status-down'} text-xs">
                                ${isUp ? 'Up' : 'Down'}
                            </span>
                        </div>
                    </div>
                    <div class="interface-ips mt-2">
                        ${hasIPs ? `
                            <div class="space-y-1">
                                <div class="flex flex-wrap gap-1">
                                    ${iface.ip_addresses.slice(0, 2).map(ip => `
                                        <span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-primary/10 text-primary border border-primary/20">
                                            <i data-lucide="${this.getIPIcon(ip)}" class="h-2.5 w-2.5 mr-1"></i>
                                            ${this.escapeHtml(ip)}
                                        </span>
                                    `).join('')}
                                    ${iface.ip_addresses.length > 2 ? `
                                        <span class="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-muted text-muted-foreground">
                                            +${iface.ip_addresses.length - 2}
                                        </span>
                                    ` : ''}
                                </div>
                            </div>
                        ` : `
                            <div class="flex items-center space-x-1 text-muted-foreground">
                                <i data-lucide="alert-circle" class="h-3 w-3"></i>
                                <span class="text-xs">No IP addresses</span>
                            </div>
                        `}
                    </div>
                </div>
            `;
        }).join('');

        lucide.createIcons();
    }

    getInterfaceIcon(interfaceName) {
        if (interfaceName.startsWith('en')) return 'ethernet';
        if (interfaceName.startsWith('wlan') || interfaceName.startsWith('wifi')) return 'wifi';
        if (interfaceName.startsWith('lo')) return 'circle';
        if (interfaceName.startsWith('ppp')) return 'phone';
        if (interfaceName.startsWith('bridge')) return 'link';
        if (interfaceName.startsWith('utun')) return 'shield';
        if (interfaceName.startsWith('awdl')) return 'radio';
        if (interfaceName.startsWith('llw')) return 'radio';
        if (interfaceName.startsWith('gif')) return 'git-branch';
        if (interfaceName.startsWith('stf')) return 'git-branch';
        if (interfaceName.startsWith('anpi')) return 'usb';
        if (interfaceName.startsWith('ap')) return 'wifi';
        return 'network';
    }

    getInterfaceType(interfaceName) {
        if (interfaceName.startsWith('en')) return 'Ethernet';
        if (interfaceName.startsWith('wlan') || interfaceName.startsWith('wifi')) return 'WiFi';
        if (interfaceName.startsWith('lo')) return 'Loopback';
        if (interfaceName.startsWith('ppp')) return 'PPP';
        if (interfaceName.startsWith('bridge')) return 'Bridge';
        if (interfaceName.startsWith('utun')) return 'VPN Tunnel';
        if (interfaceName.startsWith('awdl')) return 'AirDrop';
        if (interfaceName.startsWith('llw')) return 'Low Latency WiFi';
        if (interfaceName.startsWith('gif')) return 'Generic Tunnel';
        if (interfaceName.startsWith('stf')) return '6to4 Tunnel';
        if (interfaceName.startsWith('anpi')) return 'Apple Network';
        if (interfaceName.startsWith('ap')) return 'Access Point';
        return 'Network Interface';
    }

    getIPIcon(ip) {
        if (ip.includes(':')) return 'globe'; // IPv6
        if (ip.startsWith('192.168.') || ip.startsWith('10.') || ip.startsWith('172.')) return 'home'; // Private IP
        return 'globe'; // Public IP
    }

    async loadCurrentWiFi() {
        try {
            const response = await fetch('/api/wifi/current');
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            this.currentWiFi = await response.json();
        } catch (error) {
            console.error('Error loading current WiFi:', error);
            this.currentWiFi = { connected: false };
        }
    }

    async scanWiFiNetworks() {
        const loadingEl = document.getElementById('wifiLoading');
        const listEl = document.getElementById('wifiList');

        loadingEl.classList.remove('hidden');
        listEl.innerHTML = '';

        try {
            // Load both current WiFi and scan for networks
            const [currentResponse, scanResponse] = await Promise.all([
                fetch('/api/wifi/current'),
                fetch('/api/wifi/scan')
            ]);

            if (!currentResponse.ok) {
                throw new Error(`HTTP error! status: ${currentResponse.status}`);
            }
            if (!scanResponse.ok) {
                throw new Error(`HTTP error! status: ${scanResponse.status}`);
            }

            this.currentWiFi = await currentResponse.json();
            const networks = await scanResponse.json();
            this.renderWiFiNetworks(networks);
        } catch (error) {
            console.error('Error scanning WiFi networks:', error);
            listEl.innerHTML = `
                <div class="text-center py-8 text-red-600">
                    <i data-lucide="wifi-off" class="h-8 w-8 mx-auto mb-2"></i>
                    <p>Failed to scan WiFi networks</p>
                    <p class="text-sm text-muted-foreground">${error.message}</p>
                    <p class="text-xs text-muted-foreground mt-2">Make sure you have nmcli installed and proper permissions</p>
                </div>
            `;
            lucide.createIcons();
        } finally {
            loadingEl.classList.add('hidden');
        }
    }

    renderWiFiNetworks(networks) {
        const listEl = document.getElementById('wifiList');
        
        if (networks.length === 0) {
            listEl.innerHTML = `
                <div class="text-center py-12 text-muted-foreground">
                    <div class="flex flex-col items-center space-y-4">
                        <div class="flex items-center justify-center w-16 h-16 rounded-full bg-muted">
                            <i data-lucide="wifi-off" class="h-8 w-8"></i>
                        </div>
                        <div class="text-center">
                            <p class="text-lg font-medium">No WiFi networks found</p>
                            <p class="text-sm">Make sure your WiFi adapter is enabled and try scanning again</p>
                        </div>
                    </div>
                </div>
            `;
            lucide.createIcons();
            return;
        }

        listEl.innerHTML = networks.map(network => {
            const isConnected = this.currentWiFi && this.currentWiFi.connected && 
                               this.currentWiFi.ssid === network.ssid;
            const signalStrength = this.getSignalStrength(network.signal);
            
            return `
                <div class="wifi-card group h-full ${isConnected ? 'wifi-connected' : ''}" data-ssid="${this.escapeHtml(network.ssid)}" data-security="${this.escapeHtml(network.security)}">
                    <div class="wifi-header">
                        <div class="flex items-center space-x-2">
                            <div class="flex items-center justify-center w-8 h-8 rounded-md ${isConnected ? 'bg-primary/10' : 'bg-muted'}">
                                <i data-lucide="wifi" class="h-4 w-4 ${isConnected ? 'text-primary' : 'text-muted-foreground'}"></i>
                            </div>
                            <div class="flex-1 min-w-0">
                                <div class="flex items-center space-x-1">
                                    <h3 class="wifi-ssid truncate text-sm">${this.escapeHtml(network.ssid)}</h3>
                                    ${isConnected ? '<span class="connected-badge text-xs">Connected</span>' : ''}
                                </div>
                                <div class="flex items-center space-x-1 mt-0.5">
                                    <div class="flex items-center space-x-0.5">
                                        ${this.renderSignalBars(signalStrength)}
                                    </div>
                                    <span class="text-xs text-muted-foreground">${this.escapeHtml(network.signal || 'Unknown')}</span>
                                </div>
                            </div>
                        </div>
                        <span class="wifi-security ${this.getSecurityClass(network.security)} text-xs">
                            ${this.escapeHtml(network.security || 'Unknown')}
                        </span>
                    </div>
                    <div class="flex items-center justify-between pt-2 border-t border-border/50">
                        <div class="flex items-center space-x-1 text-xs text-muted-foreground">
                            <i data-lucide="shield" class="h-3 w-3"></i>
                            <span class="truncate">${this.getSecurityDescription(network.security)}</span>
                        </div>
                        ${isConnected ? 
                            '<div class="flex items-center space-x-1 text-primary"><i data-lucide="check-circle" class="h-3 w-3" aria-hidden="true"></i><span class="connected-status text-xs">Connected</span></div>' :
                            '<button class="connect-btn btn-primary h-6 px-2 py-1 text-xs">Connect</button>'
                        }
                    </div>
                </div>
            `;
        }).join('');

        // Add click event listeners to connect buttons
        listEl.querySelectorAll('.connect-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const card = btn.closest('.wifi-card');
                const ssid = card.dataset.ssid;
                const security = card.dataset.security;
                this.showConnectionModal(ssid, security);
            });
        });

        lucide.createIcons();
    }

    getSignalStrength(signal) {
        if (!signal || signal === 'Unknown') return 0;
        const signalNum = parseInt(signal.replace('%', ''));
        if (signalNum >= 75) return 4;
        if (signalNum >= 50) return 3;
        if (signalNum >= 25) return 2;
        if (signalNum > 0) return 1;
        return 0;
    }

    renderSignalBars(strength) {
        const bars = [];
        for (let i = 1; i <= 4; i++) {
            const isActive = i <= strength;
            bars.push(`
                <div class="w-0.5 ${isActive ? 'bg-primary' : 'bg-muted-foreground/30'} rounded-full transition-colors duration-200" style="height: ${i * 2 + 1}px;"></div>
            `);
        }
        return bars.join('');
    }

    getSecurityDescription(security) {
        switch (security) {
            case 'Open':
                return 'No password required';
            case 'WEP':
                return 'WEP encryption (weak)';
            case 'WPA/WPA2':
                return 'WPA/WPA2 encryption';
            case 'WPA2':
                return 'WPA2 encryption';
            case 'WPA3':
                return 'WPA3 encryption (most secure)';
            default:
                return 'Security type unknown';
        }
    }

    getSecurityClass(security) {
        switch (security) {
            case 'Open':
                return 'security-open';
            case 'WEP':
                return 'security-wep';
            case 'WPA/WPA2':
            case 'WPA2':
                return 'security-wpa';
            default:
                return 'security-open';
        }
    }

    showConnectionModal(ssid, security) {
        this.currentSSID = ssid;
        document.getElementById('modalSSID').textContent = `Enter password for "${ssid}"`;
        document.getElementById('password').value = '';
        document.getElementById('security').value = security || 'WPA2';
        
        const modal = document.getElementById('connectionModal');
        modal.classList.remove('hidden');
        
        // Trigger animation
        setTimeout(() => {
            modal.querySelector('.animate-scale-in').classList.add('animate-scale-in');
        }, 10);
        
        document.getElementById('password').focus();
    }

    hideConnectionModal() {
        const modal = document.getElementById('connectionModal');
        const modalContent = modal.querySelector('.animate-scale-in');
        
        // Add exit animation
        modalContent.classList.remove('animate-scale-in');
        modalContent.classList.add('animate-scale-out');
        
        setTimeout(() => {
            modal.classList.add('hidden');
            modalContent.classList.remove('animate-scale-out');
            modalContent.classList.add('animate-scale-in');
        }, 150);
        
        this.currentSSID = null;
    }

    async connectToWiFi() {
        const password = document.getElementById('password').value;
        const security = document.getElementById('security').value;

        if (!this.currentSSID) {
            alert('No network selected');
            return;
        }

        const submitBtn = document.querySelector('#connectionForm button[type="submit"]');
        const originalText = submitBtn.textContent;
        
        submitBtn.disabled = true;
        submitBtn.textContent = 'Connecting...';

        try {
            const response = await fetch('/api/wifi/connect', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    ssid: this.currentSSID,
                    password: password,
                    security: security
                })
            });

            if (!response.ok) {
                const errorData = await response.json().catch(() => ({ error: 'Unknown error' }));
                throw new Error(errorData.error || `HTTP error! status: ${response.status}`);
            }

            const result = await response.json();
            
            // Show success message
            this.showNotification('Successfully connected to WiFi network!', 'success');
            this.hideConnectionModal();
            
            // Refresh interfaces and current WiFi status
            setTimeout(() => {
                this.loadInterfaces();
                this.loadCurrentWiFi();
            }, 2000);

        } catch (error) {
            console.error('Error connecting to WiFi:', error);
            this.showNotification(`Failed to connect: ${error.message}`, 'error');
        } finally {
            submitBtn.disabled = false;
            submitBtn.textContent = originalText;
        }
    }

    showNotification(message, type = 'info') {
        // Create notification element
        const notification = document.createElement('div');
        notification.className = `notification ${type}`;
        
        notification.innerHTML = `
            <div class="flex items-center space-x-3">
                <div class="flex-shrink-0">
                    <i data-lucide="${type === 'success' ? 'check-circle' : type === 'error' ? 'x-circle' : 'info'}" class="h-5 w-5" aria-hidden="true"></i>
                </div>
                <div class="flex-1">
                    <p class="text-sm font-medium">${this.escapeHtml(message)}</p>
                </div>
                <button class="flex-shrink-0 ml-2 text-current opacity-70 hover:opacity-100 transition-opacity" onclick="this.parentElement.parentElement.remove()" aria-label="Close notification">
                    <i data-lucide="x" class="h-4 w-4" aria-hidden="true"></i>
                </button>
            </div>
        `;

        document.body.appendChild(notification);
        lucide.createIcons();

        // Animate in - force reflow and then animate
        requestAnimationFrame(() => {
            notification.classList.add('animate-slide-in');
        });

        // Auto remove after 8 seconds
        setTimeout(() => {
            notification.classList.add('animate-slide-out');
            setTimeout(() => {
                if (notification.parentElement) {
                    notification.remove();
                }
            }, 300);
        }, 8000);
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Initialize the app when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    // Small delay to ensure all elements are fully rendered
    setTimeout(() => {
        new NetworkManager();
    }, 10);
});
