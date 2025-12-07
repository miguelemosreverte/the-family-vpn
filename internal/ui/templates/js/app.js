        // State
        let bandwidthChart = null;
        let obsBandwidthChart = null;
        let bytesChart = null;
        let packetsChart = null;
        let peersChart = null;
        let currentBandwidthRange = '-5m';
        let currentMetricsRange = '-5m';
        let currentLogRange = '-15m';
        let refreshInterval = null;
        let vpnConnected = false;  // Whether tunnel is actually connected
        let vpnRouteAllEnabled = false;  // Whether route_all is requested
        let vpnToggleLoading = false;
        let isServerMode = false;  // True if viewing a server node (toggle not applicable)

        const HELSINKI_IP = '95.217.238.72';

        // Chart.js global defaults - prevent infinite growth
        Chart.defaults.maintainAspectRatio = false;
        Chart.defaults.responsive = true;

        // Load all dashboard data for single-page layout
        async function loadDashboard() {
            try {
                // Load status for header
                const status = await loadStatus();
                document.getElementById('home-node-name').textContent = status.node_name || '-';
                document.getElementById('home-vpn-ip').textContent = status.vpn_address || '-';
                document.getElementById('home-uptime').textContent = status.uptime_str || '-';
                document.getElementById('home-version').textContent = 'v' + (status.version || '0.1.0');
                document.getElementById('footer-version').textContent = 'v' + (status.version || '0.1.0');
                isServerMode = status.server_mode || false;

                // Load VPN connection status for footer
                await loadConnectionStatus();

                // Load topology (map + peers table)
                await loadPeers();

                // Load observability (metrics + logs)
                await loadObservability();

                // Load verify for footer IP
                await loadVerify();

            } catch (err) {
                console.error('Failed to load dashboard:', err);
            }
        }

        // Format bytes
        function formatBytes(bytes) {
            if (bytes === 0) return '0 B';
            const k = 1024;
            const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
        }

        // Load status
        async function loadStatus() {
            try {
                const res = await fetch('/api/status');
                if (!res.ok) throw new Error('Failed to fetch status');
                const data = await res.json();

                // Update status indicators
                document.querySelectorAll('[id$="-status-dot"]').forEach(dot => {
                    dot.classList.remove('offline');
                });
                document.querySelectorAll('[id$="-status-text"]').forEach(text => {
                    text.textContent = 'Connected';
                });

                return data;
            } catch (err) {
                document.querySelectorAll('[id$="-status-dot"]').forEach(dot => {
                    dot.classList.add('offline');
                });
                document.querySelectorAll('[id$="-status-text"]').forEach(text => {
                    text.textContent = 'Offline';
                });
                throw err;
            }
        }

        // Load home
        async function loadHome() {
            try {
                const status = await loadStatus();
                document.getElementById('home-node-name').textContent = status.node_name || '-';
                document.getElementById('home-vpn-ip').textContent = status.vpn_address || '-';
                document.getElementById('home-uptime').textContent = status.uptime_str || '-';
                document.getElementById('home-version').textContent = 'v' + (status.version || '0.1.0');

                // Also load network peers and handshakes
                loadNetworkPeers();
                loadHandshakes();
            } catch (err) {
                console.error('Failed to load home:', err);
            }
        }

        // Load network peers with SSH buttons
        async function loadNetworkPeers() {
            const container = document.getElementById('network-peers-container');

            try {
                const res = await fetch('/api/network_peers');
                if (!res.ok) throw new Error('Failed to fetch network peers');
                const data = await res.json();

                const peers = data.peers || [];

                if (peers.length === 0) {
                    container.innerHTML = '<div class="empty-state"><p>No peers in network</p></div>';
                    return;
                }

                // Get current node's VPN address to identify ourselves
                const statusRes = await fetch('/api/status');
                const status = await statusRes.json();
                const myVpnAddr = status.vpn_address;

                container.innerHTML = peers.map(peer => {
                    const isUs = peer.vpn_address === myVpnAddr;
                    const osBadge = peer.os ? `<span class="os-badge">${peer.os}</span>` : '';
                    const youBadge = isUs ? '<span class="you-badge">YOU</span>' : '';

                    // SSH command - uses VPN internal IP
                    // For server (linux), use root@. For clients (darwin), use miguel_lemos (family default)
                    const sshUser = peer.os === 'linux' ? 'root' : 'miguel_lemos';
                    const sshTarget = peer.vpn_address;
                    const sshCmd = `ssh ${sshUser}@${sshTarget}`;

                    // Disable SSH button for ourselves
                    const sshBtnClass = isUs ? 'ssh-btn disabled' : 'ssh-btn';
                    const sshBtnOnClick = isUs ? '' : `onclick="openSSHTerminal('${peer.name || 'Unknown'}', '${sshCmd}')"`;

                    return `                        <div class="peer-card ${isUs ? 'is-self' : ''}">
                            <div class="peer-card-header">
                                <div class="peer-card-avatar">${(peer.name || 'U')[0].toUpperCase()}</div>
                                <div class="peer-card-info">
                                    <div class="peer-card-name">
                                        ${peer.name || 'Unknown'}
                                        ${youBadge}
                                        ${osBadge}
                                    </div>
                                    <div class="peer-card-ip">${peer.vpn_address || '-'}</div>
                                </div>
                            </div>
                            <div class="peer-card-meta">
                                <span>Host: ${peer.hostname || '-'}</span>
                            </div>
                            <div class="peer-card-actions">
                                <button class="${sshBtnClass}" ${sshBtnOnClick} title="${isUs ? 'Cannot SSH to yourself' : sshCmd}">
                                    <span>&#128187;</span> SSH
                                </button>
                                <button class="ssh-btn" onclick="pingPeer('${peer.vpn_address}')" title="Ping ${peer.vpn_address}">
                                    <span>&#128246;</span> Ping
                                </button>
                            </div>
                        </div>
                    `;
                }).join('');
            } catch (err) {
                console.error('Failed to load network peers:', err);
                container.innerHTML = '<div class="empty-state"><p>Failed to load peers</p></div>';
            }
        }

        // Load install handshakes history
        async function loadHandshakes() {
            const tbody = document.getElementById('handshakes-tbody');
            const countEl = document.getElementById('handshakes-count');

            try {
                const res = await fetch('/api/handshakes');
                if (!res.ok) throw new Error('Failed to fetch handshakes');
                const data = await res.json();

                const entries = data.entries || [];
                countEl.textContent = entries.length + ' record' + (entries.length !== 1 ? 's' : '');

                if (entries.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="8" style="text-align:center; color: var(--text-secondary);">No handshakes recorded yet</td></tr>';
                    return;
                }

                tbody.innerHTML = entries.map(entry => {
                    const ts = new Date(entry.timestamp).toLocaleString();
                    const pingBadge = entry.ping_test_ok
                        ? `<span style="color: var(--success);">${entry.ping_test_ms}ms</span>`                        : '<span style="color: var(--error);">FAIL</span>';
                    const sshBadge = entry.ssh_test_ok
                        ? '<span style="color: var(--success);">OK</span>'
                        : '<span style="color: var(--error);">FAIL</span>';
                    const osDisplay = entry.os + '/' + entry.arch;

                    return `                        <tr>
                            <td>${ts}</td>
                            <td>${entry.node_name || '-'}</td>
                            <td>${entry.vpn_address || '-'}</td>
                            <td>${entry.public_ip || '-'}</td>
                            <td><code>${entry.version || '-'}</code></td>
                            <td>${osDisplay}</td>
                            <td>${pingBadge}</td>
                            <td>${sshBadge}</td>
                        </tr>
                    `;
                }).join('');
            } catch (err) {
                console.error('Failed to load handshakes:', err);
                tbody.innerHTML = '<tr><td colspan="8" style="text-align:center; color: var(--error);">Failed to load handshakes</td></tr>';
            }
        }

        // Copy SSH command to clipboard
        function copySSHCommand(cmd) {
            navigator.clipboard.writeText(cmd).then(() => {
                // Show brief notification
                const notification = document.createElement('div');
                notification.textContent = 'SSH command copied! Password: osopanda';
                notification.style.cssText = `                    position: fixed;
                    bottom: 20px;
                    right: 20px;
                    background: var(--success);
                    color: white;
                    padding: 12px 20px;
                    border-radius: 8px;
                    font-size: 14px;
                    z-index: 1000;
                    animation: fadeIn 0.3s ease;
                `;
                document.body.appendChild(notification);
                setTimeout(() => notification.remove(), 3000);
            }).catch(err => {
                console.error('Failed to copy:', err);
                alert('SSH command: ' + cmd + '\\nPassword: osopanda');
            });
        }

        // Ping a peer
        async function pingPeer(vpnAddr) {
            alert('Pinging ' + vpnAddr + '...\\n\\nRun in terminal:\\nping ' + vpnAddr);
        }

        // Terminal state
        let terminal = null;
        let fitAddon = null;
        let terminalWs = null;
        let currentSSHTarget = null;

        // Open SSH terminal modal with xterm.js
        function openSSHTerminal(peerName, sshCmd) {
            // Parse user and host from ssh command (ssh user@host)
            const match = sshCmd.match(/ssh\s+(\w+)@([\w\.\-]+)/);
            if (!match) {
                alert('Invalid SSH command format');
                return;
            }
            const user = match[1];
            const host = match[2];

            currentSSHTarget = { peerName, user, host };

            document.getElementById('terminal-title-text').textContent = 'SSH to ' + peerName + ' (' + host + ')';
            document.getElementById('terminal-modal').classList.add('open');

            // Initialize xterm.js
            initTerminal(user, host);
        }

        // Open Screen Sharing (VNC) to a macOS peer
        // Password is fetched from the server (loaded from .env file)
        async function openScreenShare(vpnAddress, user) {
            try {
                // Fetch VNC password from server (loaded from .env file)
                const response = await fetch('/api/vnc-config');
                if (!response.ok) {
                    throw new Error('Failed to get VNC configuration');
                }
                const config = await response.json();

                if (!config.password) {
                    alert('VNC password not configured. Please set VNC_PASSWORD in your .env file.');
                    return;
                }

                // Construct VNC URL: vnc://user:password@host
                // macOS Screen Sharing.app will handle this URL
                const vncUrl = 'vnc://' + user + ':' + config.password + '@' + vpnAddress;

                // Open VNC URL - this will launch Screen Sharing.app on macOS
                window.location.href = vncUrl;
            } catch (err) {
                console.error('Screen share error:', err);
                alert('Failed to open Screen Sharing: ' + err.message);
            }
        }

        // Initialize xterm.js terminal
        function initTerminal(user, host) {
            const container = document.getElementById('terminal-body');

            // Clean up previous terminal
            if (terminal) {
                terminal.dispose();
                terminal = null;
            }
            if (terminalWs) {
                terminalWs.close();
                terminalWs = null;
            }
            container.innerHTML = '';

            // Create new terminal
            terminal = new Terminal({
                cursorBlink: true,
                fontSize: 14,
                fontFamily: 'Monaco, Menlo, monospace',
                theme: {
                    background: '#0f172a',
                    foreground: '#f8fafc',
                    cursor: '#f8fafc',
                    cursorAccent: '#0f172a',
                    selection: 'rgba(59, 130, 246, 0.3)',
                    black: '#1e293b',
                    red: '#ef4444',
                    green: '#22c55e',
                    yellow: '#f59e0b',
                    blue: '#3b82f6',
                    magenta: '#8b5cf6',
                    cyan: '#06b6d4',
                    white: '#f8fafc',
                    brightBlack: '#475569',
                    brightRed: '#f87171',
                    brightGreen: '#4ade80',
                    brightYellow: '#fbbf24',
                    brightBlue: '#60a5fa',
                    brightMagenta: '#a78bfa',
                    brightCyan: '#22d3ee',
                    brightWhite: '#ffffff'
                }
            });

            // Create fit addon
            fitAddon = new FitAddon.FitAddon();
            terminal.loadAddon(fitAddon);

            // Mount terminal
            terminal.open(container);
            fitAddon.fit();

            // Update status dot
            const statusDot = document.getElementById('terminal-status-dot');
            statusDot.style.background = 'var(--warning)';

            terminal.writeln('Connecting to ' + user + '@' + host + '...');
            terminal.writeln('');

            // Connect WebSocket
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = protocol + '//' + window.location.host + '/ws/terminal';

            terminalWs = new WebSocket(wsUrl);

            terminalWs.onopen = () => {
                // Send connection request
                terminalWs.send(JSON.stringify({
                    host: host,
                    user: user,
                    password: 'osopanda'
                }));
                statusDot.style.background = 'var(--success)';

                // Send terminal size
                const dims = { cols: terminal.cols, rows: terminal.rows };
                terminalWs.send(JSON.stringify(dims));
            };

            terminalWs.onmessage = (event) => {
                if (event.data instanceof Blob) {
                    event.data.text().then(text => terminal.write(text));
                } else {
                    terminal.write(event.data);
                }
            };

            terminalWs.onerror = (error) => {
                console.error('WebSocket error:', error);
                terminal.writeln('\\r\\n\\x1b[31mConnection error\\x1b[0m');
                statusDot.style.background = 'var(--error)';
            };

            terminalWs.onclose = () => {
                terminal.writeln('\\r\\n\\x1b[33mConnection closed\\x1b[0m');
                statusDot.style.background = 'var(--error)';
            };

            // Handle input
            terminal.onData(data => {
                if (terminalWs && terminalWs.readyState === WebSocket.OPEN) {
                    terminalWs.send(data);
                }
            });

            // Handle resize
            terminal.onResize(({ cols, rows }) => {
                if (terminalWs && terminalWs.readyState === WebSocket.OPEN) {
                    terminalWs.send(JSON.stringify({ cols, rows }));
                }
            });

            // Fit on window resize
            window.addEventListener('resize', () => {
                if (fitAddon && terminal) {
                    fitAddon.fit();
                }
            });
        }

        // Close terminal modal
        function closeTerminal() {
            document.getElementById('terminal-modal').classList.remove('open');

            // Clean up
            if (terminalWs) {
                terminalWs.close();
                terminalWs = null;
            }
            if (terminal) {
                terminal.dispose();
                terminal = null;
            }
            currentSSHTarget = null;
        }

        // Close on backdrop click
        function closeTerminalOnBackdrop(event) {
            if (event.target.id === 'terminal-modal') {
                closeTerminal();
            }
        }

        // Close terminal on Escape key
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') {
                closeTerminal();
            }
        });

        // Load overview
        async function loadOverview() {
            try {
                const status = await loadStatus();

                document.getElementById('stat-name').textContent = status.node_name || '-';
                document.getElementById('stat-vpn-ip').textContent = status.vpn_address || '-';
                document.getElementById('stat-uptime').textContent = status.uptime_str || '-';

                // Check if VPN routing is enabled to determine what to show
                const connRes = await fetch('/api/connection');
                const connStatus = await connRes.json();
                const isVPNActive = connStatus.route_all; // VPN toggle is ON

                // Get topology data for peer count (more accurate than /api/peers which is server-only)
                const topoRes = await fetch('/api/topology');
                const topoData = await topoRes.json();

                // Network peers = ALL nodes in the network (including server, excluding ourselves)
                // When VPN is active, show all other network members
                // When VPN is off, show 0 peers
                let networkNodes = [];
                if (isVPNActive) {
                    networkNodes = (topoData.nodes || []).filter(n => !n.is_us);
                }

                document.getElementById('stat-peers').textContent = networkNodes.length;

                // Show bytes only if VPN is active
                if (isVPNActive) {
                    document.getElementById('stat-in').textContent = formatBytes(status.bytes_in || 0);
                    document.getElementById('stat-out').textContent = formatBytes(status.bytes_out || 0);
                } else {
                    document.getElementById('stat-in').textContent = '-';
                    document.getElementById('stat-out').textContent = '-';
                }

                // Render peers table from topology
                const tbody = document.getElementById('peers-tbody');
                if (isVPNActive && networkNodes.length > 0) {
                    tbody.innerHTML = networkNodes.map(p => `                        <tr>
                            <td>
                                <div class="peer-name">
                                    <div class="peer-avatar">${(p.name || 'U')[0].toUpperCase()}</div>
                                    ${p.name || 'Unknown'}
                                </div>
                            </td>
                            <td>${p.vpn_address || '-'}</td>
                            <td>${p.public_addr || '-'}</td>
                            <td>${p.connected_at ? new Date(p.connected_at).toLocaleString() : '-'}</td>
                        </tr>
                    `).join('');
                } else {
                    const message = isVPNActive
                        ? 'No other peers in network'
                        : 'VPN routing disabled - enable to see peers';
                    tbody.innerHTML = `<tr><td colspan="4" style="text-align:center;color:var(--text-secondary)">${message}</td></tr>`;
                }

                // Load bandwidth chart (only meaningful when VPN is active)
                loadBandwidthChart();
            } catch (err) {
                console.error('Failed to load overview:', err);
            }
        }

        // Load bandwidth chart
        async function loadBandwidthChart() {
            try {
                // Don't filter by specific metrics - get all and find the bandwidth ones
                const res = await fetch(`/api/stats?earliest=${currentBandwidthRange}&granularity=raw`);
                const data = await res.json();

                const ctx = document.getElementById('bandwidth-chart').getContext('2d');

                // Try current_bps first, fall back to avg_bps
                let txSeries = data.series?.find(s => s.name === 'bandwidth.tx_current_bps');
                let rxSeries = data.series?.find(s => s.name === 'bandwidth.rx_current_bps');

                if (!txSeries || !txSeries.points?.length) {
                    txSeries = data.series?.find(s => s.name === 'bandwidth.tx_avg_bps');
                }
                if (!rxSeries || !rxSeries.points?.length) {
                    rxSeries = data.series?.find(s => s.name === 'bandwidth.rx_avg_bps');
                }

                const labels = (txSeries?.points || []).map(p => new Date(p.timestamp).toLocaleTimeString());
                const txData = (txSeries?.points || []).map(p => p.value / 1024);
                const rxData = (rxSeries?.points || []).map(p => p.value / 1024);

                if (bandwidthChart) {
                    bandwidthChart.data.labels = labels;
                    bandwidthChart.data.datasets[0].data = txData;
                    bandwidthChart.data.datasets[1].data = rxData;
                    bandwidthChart.update('none');
                } else {
                    bandwidthChart = new Chart(ctx, {
                        type: 'line',
                        data: {
                            labels,
                            datasets: [{
                                label: 'TX (KB/s)',
                                data: txData,
                                borderColor: '#3b82f6',
                                backgroundColor: 'rgba(59, 130, 246, 0.1)',
                                fill: true,
                                tension: 0.4
                            }, {
                                label: 'RX (KB/s)',
                                data: rxData,
                                borderColor: '#22c55e',
                                backgroundColor: 'rgba(34, 197, 94, 0.1)',
                                fill: true,
                                tension: 0.4
                            }]
                        },
                        options: {
                            responsive: true,
                            maintainAspectRatio: false,
                            plugins: { legend: { labels: { color: '#94a3b8' } } },
                            scales: {
                                x: { ticks: { color: '#94a3b8', maxTicksLimit: 10 }, grid: { color: '#334155' } },
                                y: { ticks: { color: '#94a3b8' }, grid: { color: '#334155' } }
                            }
                        }
                    });
                }
            } catch (err) {
                console.error('Failed to load bandwidth chart:', err);
            }
        }

        // Load observability
        async function loadObservability() {
            loadMetricsCharts();
            loadPeerFilterOptions();
            loadLogs();
        }

        // Populate the peer filter dropdown with network peers
        async function loadPeerFilterOptions() {
            const select = document.getElementById('log-peer-filter');

            try {
                const res = await fetch('/api/network_peers');
                if (!res.ok) return;
                const data = await res.json();

                const peers = data.peers || [];

                // Keep the "All Peers" (local logs) option
                select.innerHTML = '<option value="">Local Node</option>';

                // Add an option for each peer - value is the VPN address
                // This allows us to connect to that peer's control socket
                peers.forEach(peer => {
                    const name = peer.name || peer.hostname || 'Unknown';
                    const vpnAddr = peer.vpn_address || '';

                    if (!vpnAddr) return; // Skip peers without VPN address

                    const option = document.createElement('option');
                    option.value = vpnAddr; // VPN address for remote connection
                    option.textContent = `${name} (${vpnAddr})`;
                    select.appendChild(option);
                });
            } catch (err) {
                console.error('Failed to load peer filter options:', err);
            }
        }

        // Load metrics charts
        async function loadMetricsCharts() {
            try {
                const res = await fetch(`/api/stats?earliest=${currentMetricsRange}&granularity=raw`);
                const data = await res.json();

                // Obs Bandwidth chart
                const txBwSeries = data.series?.find(s => s.name === 'bandwidth.tx_current_bps');
                const rxBwSeries = data.series?.find(s => s.name === 'bandwidth.rx_current_bps');
                updateChart('obs-bandwidth-chart', obsBandwidthChart, c => obsBandwidthChart = c,
                    txBwSeries, rxBwSeries, 'TX', 'RX', '#3b82f6', '#22c55e', v => (v/1024).toFixed(1) + ' KB/s');

                // Bytes chart
                const bytesSentSeries = data.series?.find(s => s.name === 'vpn.bytes_sent');
                const bytesRecvSeries = data.series?.find(s => s.name === 'vpn.bytes_recv');
                updateChart('bytes-chart', bytesChart, c => bytesChart = c,
                    bytesSentSeries, bytesRecvSeries, 'Sent', 'Received', '#3b82f6', '#22c55e', formatBytes);

                // Packets chart
                const packetsSentSeries = data.series?.find(s => s.name === 'vpn.packets_sent');
                const packetsRecvSeries = data.series?.find(s => s.name === 'vpn.packets_recv');
                updateChart('packets-chart', packetsChart, c => packetsChart = c,
                    packetsSentSeries, packetsRecvSeries, 'Sent', 'Received', '#f59e0b', '#8b5cf6', v => v.toLocaleString());

                // Peers chart
                const peersSeries = data.series?.find(s => s.name === 'vpn.active_peers');
                updateSingleChart('peers-chart', peersChart, c => peersChart = c,
                    peersSeries, 'Active Peers', '#8b5cf6');

            } catch (err) {
                console.error('Failed to load metrics charts:', err);
            }
        }

        function updateChart(canvasId, chart, setChart, series1, series2, label1, label2, color1, color2, formatFn) {
            const ctx = document.getElementById(canvasId).getContext('2d');
            const labels = (series1?.points || []).map(p => new Date(p.timestamp).toLocaleTimeString());
            const data1 = (series1?.points || []).map(p => p.value);
            const data2 = (series2?.points || []).map(p => p.value);

            if (chart) {
                chart.data.labels = labels;
                chart.data.datasets[0].data = data1;
                chart.data.datasets[1].data = data2;
                chart.update('none');
            } else {
                setChart(new Chart(ctx, {
                    type: 'line',
                    data: {
                        labels,
                        datasets: [{
                            label: label1,
                            data: data1,
                            borderColor: color1,
                            tension: 0.4
                        }, {
                            label: label2,
                            data: data2,
                            borderColor: color2,
                            tension: 0.4
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: { legend: { labels: { color: '#94a3b8' } } },
                        scales: {
                            x: { ticks: { color: '#94a3b8', maxTicksLimit: 8 }, grid: { color: '#334155' } },
                            y: { ticks: { color: '#94a3b8', callback: formatFn }, grid: { color: '#334155' } }
                        }
                    }
                }));
            }
        }

        function updateSingleChart(canvasId, chart, setChart, series, label, color) {
            const ctx = document.getElementById(canvasId).getContext('2d');
            const labels = (series?.points || []).map(p => new Date(p.timestamp).toLocaleTimeString());
            const data = (series?.points || []).map(p => p.value);

            if (chart) {
                chart.data.labels = labels;
                chart.data.datasets[0].data = data;
                chart.update('none');
            } else {
                setChart(new Chart(ctx, {
                    type: 'line',
                    data: {
                        labels,
                        datasets: [{
                            label: label,
                            data: data,
                            borderColor: color,
                            backgroundColor: color + '20',
                            fill: true,
                            tension: 0.4
                        }]
                    },
                    options: {
                        responsive: true,
                        maintainAspectRatio: false,
                        plugins: { legend: { labels: { color: '#94a3b8' } } },
                        scales: {
                            x: { ticks: { color: '#94a3b8', maxTicksLimit: 8 }, grid: { color: '#334155' } },
                            y: { ticks: { color: '#94a3b8' }, grid: { color: '#334155' }, beginAtZero: true }
                        }
                    }
                }));
            }
        }

        // Load logs
        async function loadLogs() {
            const search = document.getElementById('log-search').value;
            const level = document.getElementById('log-level-filter').value;
            const component = document.getElementById('log-component-filter').value;
            const peerFilter = document.getElementById('log-peer-filter').value;

            // Build the API URL
            // If peerFilter is set, it contains the VPN address of the remote peer
            // The backend will connect to that peer's control socket to fetch their logs
            let url = `/api/logs?earliest=${currentLogRange}`;
            if (peerFilter) url += `&peer=${encodeURIComponent(peerFilter)}`;
            if (search) url += `&search=${encodeURIComponent(search)}`;
            if (level) url += `&level=${level}`;
            if (component) url += `&component=${component}`;

            try {
                const res = await fetch(url);
                const data = await res.json();

                const container = document.getElementById('logs-list');
                const entries = data.entries || [];

                if (entries.length > 0) {
                    container.innerHTML = entries.map(e => `                        <div class="log-entry">
                            <span class="log-time">${e.timestamp?.substring(0, 19) || ''}</span>
                            <span class="log-level ${e.level}">${e.level}</span>
                            <span class="log-component">[${e.component}]</span>
                            <span class="log-message">${escapeHtml(e.message)}</span>
                        </div>
                    `).join('');
                } else {
                    const peerMsg = peerFilter ? ' from remote peer (may be unreachable)' : '';
                    container.innerHTML = `<div class="empty-state"><p>No logs found${peerMsg}</p></div>`;
                }
            } catch (err) {
                console.error('Failed to load logs:', err);
                document.getElementById('logs-list').innerHTML = '<div class="empty-state"><p>Failed to load logs</p></div>';
            }
        }

        function escapeHtml(str) {
            if (!str) return '';
            return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        }

        // Leaflet map instance
        let networkMap = null;
        let mapMarkers = [];
        let mapArcs = [];
        let topologyData = { nodes: [], edges: [] };
        let topologySortBy = 'distance';
        let topologySortAsc = true;
        let myVpnAddr = null; // Current node's VPN address (for correct "YOU" detection)

        // Map tile layer - standard OpenStreetMap
        const mapTile = L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
            maxZoom: 19
        });

        // Load peers/topology page
        async function loadPeers() {
            try {
                // First, get our own VPN address for correct "YOU" identification
                const statusRes = await fetch('/api/status');
                const statusData = await statusRes.json();
                myVpnAddr = statusData.vpn_address;

                // Check if VPN routing is enabled (this is what the toggle controls)
                const connRes = await fetch('/api/connection');
                const connStatus = await connRes.json();
                const isVPNActive = connStatus.route_all; // VPN toggle is ON

                // Then get topology
                const res = await fetch('/api/topology');
                const data = await res.json();

                // If VPN routing is not active, only show ourselves
                // This is consistent with Overview - only show peers when VPN is ON
                if (!isVPNActive) {
                    data.nodes = (data.nodes || []).filter(n => n.vpn_address === myVpnAddr);
                    data.edges = [];
                }

                topologyData = data;

                renderNetworkMap(data);
                renderTopologyTable(data.nodes || []);

                // Update node count with VPN status
                const statusText = isVPNActive ? '' : ' (VPN routing disabled)';
                document.getElementById('topology-node-count').textContent =
                    `${(data.nodes || []).length} nodes, ${(data.edges || []).length} connections${statusText}`;
            } catch (err) {
                console.error('Failed to load topology:', err);
                document.getElementById('all-peers-tbody').innerHTML =
                    '<tr><td colspan="7" style="text-align:center;color:var(--text-secondary)">Failed to load network topology</td></tr>';
            }
        }

        // Render Leaflet map with nodes and great circle arcs
        const HELSINKI_VPN_IP = '10.8.0.1';
        // Default Helsinki coordinates if geo not available
        const HELSINKI_DEFAULT = { lat: 60.1699, lon: 24.9384 };

        // Helper to create a location key for grouping nodes
        function locationKey(lat, lon) {
            // Round to 2 decimal places (~1km precision) for grouping nearby nodes
            return `${lat.toFixed(2)},${lon.toFixed(2)}`;
        }

        // Helper to check if a node is "us" (the viewing node)
        function isOurNode(n) {
            return n.vpn_address === myVpnAddr;
        }

        function renderNetworkMap(data) {
            const container = document.getElementById('network-graph');
            const nodes = data.nodes || [];

            // Initialize map if not exists
            if (!networkMap) {
                networkMap = L.map(container, {
                    center: [45, 10], // Center on Europe initially
                    zoom: 3,
                    zoomControl: true,
                    attributionControl: false
                });
                mapTile.addTo(networkMap);
            }

            // Clear existing markers and arcs
            mapMarkers.forEach(m => networkMap.removeLayer(m));
            mapArcs.forEach(a => networkMap.removeLayer(a));
            mapMarkers = [];
            mapArcs = [];

            // Find Helsinki (the hub)
            const helsinkiNode = nodes.find(n => n.vpn_address === HELSINKI_VPN_IP);
            const helsinkiCoords = helsinkiNode?.geo
                ? [helsinkiNode.geo.lat, helsinkiNode.geo.lon]
                : [HELSINKI_DEFAULT.lat, HELSINKI_DEFAULT.lon];

            // Group nodes by location for stacking
            const locationGroups = new Map();
            nodes.forEach(n => {
                let lat, lon;
                if (n.geo && n.geo.lat && n.geo.lon) {
                    lat = n.geo.lat;
                    lon = n.geo.lon;
                } else if (n.vpn_address === HELSINKI_VPN_IP) {
                    lat = HELSINKI_DEFAULT.lat;
                    lon = HELSINKI_DEFAULT.lon;
                } else {
                    return; // No geo data, skip
                }

                const key = locationKey(lat, lon);
                if (!locationGroups.has(key)) {
                    locationGroups.set(key, { lat, lon, nodes: [] });
                }
                locationGroups.get(key).nodes.push(n);
            });

            // Add markers for each location group
            const bounds = [];
            locationGroups.forEach((group, key) => {
                const { lat, lon, nodes: groupNodes } = group;
                bounds.push([lat, lon]);

                // Determine marker style based on nodes in group
                const hasHelsinki = groupNodes.some(n => n.vpn_address === HELSINKI_VPN_IP);
                const hasUs = groupNodes.some(n => isOurNode(n));
                const nodeCount = groupNodes.length;

                let color = '#3b82f6'; // Default blue
                let radius = 8 + (nodeCount > 1 ? Math.min(nodeCount * 2, 8) : 0); // Bigger for multiple nodes

                if (hasHelsinki) {
                    color = '#22c55e'; // Green for the hub
                    radius = Math.max(radius, 12);
                } else if (hasUs) {
                    color = '#8b5cf6'; // Purple for ourselves
                    radius = Math.max(radius, 10);
                }

                // Create circle marker
                const marker = L.circleMarker([lat, lon], {
                    radius: radius,
                    fillColor: color,
                    color: '#fff',
                    weight: 2,
                    opacity: 1,
                    fillOpacity: 0.9
                });

                // Build popup content showing all nodes at this location
                const location = groupNodes[0].geo
                    ? `${groupNodes[0].geo.city || ''}, ${groupNodes[0].geo.country || ''}`.replace(/^, |, $/g, '')
                    : 'Unknown';

                let popupContent = `<div class="node-popup">`;

                if (nodeCount > 1) {
                    popupContent += `<div style="font-size: 12px; color: var(--text-secondary); margin-bottom: 8px;">
                        ${nodeCount} nodes at ${location}
                    </div>`;
                }

                // List all nodes at this location
                groupNodes.forEach((n, idx) => {
                    const isUs = isOurNode(n);
                    const isHelsinkiNode = n.vpn_address === HELSINKI_VPN_IP;
                    const icon = isUs ? '⭐' : (isHelsinkiNode ? '🌐' : '💻');
                    const youBadge = isUs ? '<span class="you-badge">YOU</span>' : '';

                    if (idx > 0) {
                        popupContent += `<div style="border-top: 1px solid var(--border); margin: 8px 0;"></div>`;
                    }

                    popupContent += `                        <div class="node-popup-name">
                            ${icon} ${n.name || 'Unknown'} ${youBadge}
                        </div>
                        <div class="node-popup-info">
                            <strong>VPN IP:</strong> ${n.vpn_address || '-'}<br>
                            ${nodeCount === 1 ? '<strong>Location:</strong> ' + location + '<br>' : ''}
                            <strong>Distance:</strong> ${n.distance === 0 ? 'Local' : n.distance + ' hop(s)'}<br>
                            ${n.latency_ms ? '<strong>Latency:</strong> ' + n.latency_ms.toFixed(1) + ' ms<br>' : ''}
                            ${n.geo?.isp ? '<strong>ISP:</strong> ' + n.geo.isp + '<br>' : ''}
                        </div>
                    `;
                });

                popupContent += `</div>`;

                marker.bindPopup(popupContent);
                marker.addTo(networkMap);
                mapMarkers.push(marker);

                // Draw great circle arcs from each node to Helsinki hub (if not at Helsinki)
                if (!hasHelsinki && helsinkiCoords) {
                    // Use the most important node's color for the arc
                    const arcColor = hasUs ? '#8b5cf6' : '#3b82f6';
                    const arc = drawGreatCircleArc([lat, lon], helsinkiCoords, arcColor);
                    if (arc) {
                        arc.addTo(networkMap);
                        mapArcs.push(arc);
                    }
                }
            });

            // Fit map to show all nodes
            if (bounds.length > 1) {
                networkMap.fitBounds(bounds, { padding: [50, 50] });
            } else if (bounds.length === 1) {
                networkMap.setView(bounds[0], 5);
            }
        }

        // Draw a great circle arc between two points
        function drawGreatCircleArc(from, to, color) {
            const points = calculateGreatCircle(from, to, 50);

            const polyline = L.polyline(points, {
                color: color,
                weight: 2,
                opacity: 0.7,
                dashArray: '10, 5',
                className: 'leaflet-arc-path'
            });

            return polyline;
        }

        // Calculate points along a great circle arc
        function calculateGreatCircle(from, to, numPoints) {
            const points = [];
            const lat1 = from[0] * Math.PI / 180;
            const lon1 = from[1] * Math.PI / 180;
            const lat2 = to[0] * Math.PI / 180;
            const lon2 = to[1] * Math.PI / 180;

            for (let i = 0; i <= numPoints; i++) {
                const f = i / numPoints;

                // Spherical interpolation
                const d = 2 * Math.asin(Math.sqrt(
                    Math.pow(Math.sin((lat1 - lat2) / 2), 2) +
                    Math.cos(lat1) * Math.cos(lat2) * Math.pow(Math.sin((lon1 - lon2) / 2), 2)
                ));

                if (d === 0) {
                    points.push(from);
                    continue;
                }

                const A = Math.sin((1 - f) * d) / Math.sin(d);
                const B = Math.sin(f * d) / Math.sin(d);

                const x = A * Math.cos(lat1) * Math.cos(lon1) + B * Math.cos(lat2) * Math.cos(lon2);
                const y = A * Math.cos(lat1) * Math.sin(lon1) + B * Math.cos(lat2) * Math.sin(lon2);
                const z = A * Math.sin(lat1) + B * Math.sin(lat2);

                const lat = Math.atan2(z, Math.sqrt(x * x + y * y)) * 180 / Math.PI;
                const lon = Math.atan2(y, x) * 180 / Math.PI;

                points.push([lat, lon]);
            }

            return points;
        }

        // Fit map to show all nodes
        function fitNetworkMap() {
            if (networkMap && mapMarkers.length > 0) {
                const bounds = mapMarkers.map(m => m.getLatLng());
                networkMap.fitBounds(bounds, { padding: [50, 50] });
            }
        }


        // Render topology table
        function renderTopologyTable(nodes) {
            // Sort nodes
            const sorted = [...nodes].sort((a, b) => {
                let aVal, bVal;
                switch (topologySortBy) {
                    case 'distance':
                        aVal = a.distance || 999;
                        bVal = b.distance || 999;
                        break;
                    case 'name':
                        aVal = (a.name || '').toLowerCase();
                        bVal = (b.name || '').toLowerCase();
                        break;
                    case 'latency':
                        aVal = a.latency_ms || 9999;
                        bVal = b.latency_ms || 9999;
                        break;
                    case 'bandwidth':
                        aVal = a.bandwidth_bps || 0;
                        bVal = b.bandwidth_bps || 0;
                        break;
                    default:
                        return 0;
                }
                if (aVal < bVal) return topologySortAsc ? -1 : 1;
                if (aVal > bVal) return topologySortAsc ? 1 : -1;
                return 0;
            });

            const tbody = document.getElementById('all-peers-tbody');
            if (sorted.length > 0) {
                tbody.innerHTML = sorted.map(n => {
                    // Use isOurNode for correct "YOU" identification based on myVpnAddr
                    const isUs = isOurNode(n);
                    const distanceClass = `distance-${Math.min(n.distance || 0, 3)}`;
                    const distanceLabel = isUs ? 'You' :
                                          n.distance === 1 ? '1 hop' :
                                          `${n.distance} hops`;
                    const selfBadge = isUs ? '<span class="self-indicator">YOU</span>' : '';
                    const statusColor = isUs || n.is_direct ? 'var(--success)' : 'var(--text-secondary)';
                    const statusText = isUs ? 'Local' : (n.is_direct ? 'Direct' : 'Via Relay');

                    // SSH command - uses VPN internal IP, root for linux, miguel_lemos for darwin
                    const sshUser = n.os === 'linux' ? 'root' : 'miguel_lemos';
                    const sshCmd = `ssh ${sshUser}@${n.vpn_address}`;
                    const sshDisabled = isUs;

                    return `                        <tr>
                            <td><span class="distance-badge ${distanceClass}">${distanceLabel}</span></td>
                            <td>
                                <div class="peer-name">
                                    <div class="peer-avatar" style="${isUs ? 'background: linear-gradient(135deg, #8b5cf6, #3b82f6)' : ''}">${(n.name || 'U')[0].toUpperCase()}</div>
                                    ${n.name || 'Unknown'}${selfBadge}
                                </div>
                            </td>
                            <td>${n.vpn_address || '-'}</td>
                            <td>${n.public_addr || '-'}</td>
                            <td>${n.latency_ms ? n.latency_ms.toFixed(1) + ' ms' : '-'}</td>
                            <td>${n.bandwidth_bps ? formatBytes(n.bandwidth_bps) + '/s' : '-'}</td>
                            <td style="color: ${statusColor}">${statusText}</td>
                            <td class="ssh-cell">
                                ${sshDisabled ? '<span style="color: var(--text-secondary)">-</span>' : `                                    <button class="ssh-table-btn" onclick="openSSHTerminal('${n.name || 'Unknown'}', '${sshCmd}')" title="Open SSH terminal">
                                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                            <rect x="2" y="3" width="20" height="18" rx="2"/>
                                            <path d="M7 8l4 4-4 4"/>
                                            <line x1="13" y1="16" x2="17" y2="16"/>
                                        </svg>
                                    </button>
                                    <code class="ssh-cmd" onclick="copySSHCommand('${sshCmd}')" title="Click to copy">${sshCmd}</code>
                                `}
                            </td>
                            <td class="screen-cell">
                                ${(sshDisabled || n.os === 'linux') ? '<span style="color: var(--text-secondary)">-</span>' : `                                    <button class="screen-table-btn" onclick="openScreenShare('${n.vpn_address}', '${sshUser}')" title="Open Screen Sharing (VNC)">
                                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                            <rect x="2" y="3" width="20" height="14" rx="2"/>
                                            <line x1="8" y1="21" x2="16" y2="21"/>
                                            <line x1="12" y1="17" x2="12" y2="21"/>
                                        </svg>
                                    </button>
                                `}
                            </td>
                        </tr>
                    `;
                }).join('');
            } else {
                tbody.innerHTML = '<tr><td colspan="9" style="text-align:center;color:var(--text-secondary)">No nodes in network</td></tr>';
            }

            // Update sortable headers
            document.querySelectorAll('th.sortable').forEach(th => {
                th.classList.remove('asc', 'desc');
                if (th.dataset.sort === topologySortBy) {
                    th.classList.add(topologySortAsc ? 'asc' : 'desc');
                }
            });
        }

        // Handle sortable column clicks
        document.querySelectorAll('th.sortable').forEach(th => {
            th.addEventListener('click', () => {
                const sort = th.dataset.sort;
                if (topologySortBy === sort) {
                    topologySortAsc = !topologySortAsc;
                } else {
                    topologySortBy = sort;
                    topologySortAsc = true;
                }
                renderTopologyTable(topologyData.nodes || []);
            });
        });

        // Load verify page
        async function loadVerify() {
            try {
                const ipRes = await fetch('/api/verify');
                const ipData = await ipRes.json();

                const publicIp = ipData.public_ip || 'Unknown';
                document.getElementById('footer-public-ip').textContent = publicIp;

                const verifyStatus = document.getElementById('footer-verify-status');
                if (publicIp === HELSINKI_IP) {
                    verifyStatus.textContent = '(Routed)';
                    verifyStatus.style.color = 'var(--success)';
                } else {
                    verifyStatus.textContent = '(Direct)';
                    verifyStatus.style.color = 'var(--warning)';
                }
            } catch (err) {
                console.error('Failed to verify:', err);
                document.getElementById('footer-public-ip').textContent = 'Error';
                document.getElementById('footer-verify-status').textContent = '';
            }
        }

        // Event listeners for bandwidth chart range
        document.querySelectorAll('.bw-range').forEach(btn => {
            btn.addEventListener('click', () => {
                document.querySelectorAll('.bw-range').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                currentBandwidthRange = btn.dataset.range;
                loadBandwidthChart();
            });
        });

        // Event listeners for metrics chart range
        document.querySelectorAll('.metrics-range').forEach(btn => {
            btn.addEventListener('click', () => {
                document.querySelectorAll('.metrics-range').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                currentMetricsRange = btn.dataset.range;
                loadMetricsCharts();
            });
        });

        // Event listeners for log time range
        document.querySelectorAll('.time-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                document.querySelectorAll('.time-btn').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                currentLogRange = btn.dataset.range;
                loadLogs();
            });
        });

        // Event listeners for log filters
        document.getElementById('log-search').addEventListener('input', debounce(loadLogs, 300));
        document.getElementById('log-level-filter').addEventListener('change', loadLogs);
        document.getElementById('log-component-filter').addEventListener('change', loadLogs);
        document.getElementById('log-peer-filter').addEventListener('change', loadLogs);

        // Debounce helper
        function debounce(fn, delay) {
            let timeout;
            return function(...args) {
                clearTimeout(timeout);
                timeout = setTimeout(() => fn.apply(this, args), delay);
            };
        }

        // Auto refresh - single page layout refreshes all sections
        function startRefresh() {
            refreshInterval = setInterval(() => {
                loadDashboard();
            }, 5000);
        }

        // VPN Toggle functions
        async function loadConnectionStatus() {
            try {
                // First check if we're viewing a server node
                const statusRes = await fetch('/api/status');
                if (statusRes.ok) {
                    const statusData = await statusRes.json();
                    isServerMode = statusData.server_mode || false;
                }

                const res = await fetch('/api/connection');
                if (!res.ok) throw new Error('Failed to fetch connection status');
                const status = await res.json();

                // Track both connection and route_all state for truthful UI
                vpnConnected = status.connected;
                vpnRouteAllEnabled = status.route_all;
                updateToggleUI();

                return status;
            } catch (err) {
                console.error('Failed to load connection status:', err);
                document.getElementById('footer-vpn-status').textContent = 'Error';
            }
        }

        function updateToggleUI() {
            const toggle = document.getElementById('footer-vpn-toggle');
            const statusText = document.getElementById('footer-vpn-status');

            // Server mode: toggle is not applicable, disable it
            if (isServerMode) {
                toggle.classList.remove('on', 'loading');
                toggle.classList.add('disabled');
                toggle.style.opacity = '0.5';
                toggle.style.cursor = 'not-allowed';
                statusText.textContent = 'Server mode';
                statusText.style.color = 'var(--text-secondary)';
                return;
            }

            // Client mode: restore normal toggle behavior
            toggle.classList.remove('disabled');
            toggle.style.opacity = '1';
            toggle.style.cursor = 'pointer';

            if (vpnToggleLoading) {
                toggle.classList.add('loading');
                statusText.textContent = 'Updating...';
                return;
            }

            toggle.classList.remove('loading');

            // Toggle is "on" only when both connected AND routing through VPN
            // This ensures UI truth - toggle reflects actual state, not just config
            const vpnActive = vpnConnected && vpnRouteAllEnabled;

            if (vpnActive) {
                toggle.classList.add('on');
                statusText.textContent = 'All traffic through VPN';
                statusText.style.color = 'var(--success)';
            } else if (vpnRouteAllEnabled && !vpnConnected) {
                // route_all is set but not connected - show warning state
                toggle.classList.remove('on');
                statusText.textContent = 'Not connected';
                statusText.style.color = 'var(--warning)';
            } else {
                toggle.classList.remove('on');
                statusText.textContent = 'Direct traffic';
                statusText.style.color = 'var(--text-secondary)';
            }
        }

        async function toggleVPN() {
            // Prevent toggle in server mode - route-all not supported
            if (isServerMode) return;
            if (vpnToggleLoading) return;

            vpnToggleLoading = true;
            updateToggleUI();

            try {
                // Toggle based on current actual state (both connected AND route_all)
                const currentlyActive = vpnConnected && vpnRouteAllEnabled;
                const action = currentlyActive ? 'disconnect' : 'connect';
                const res = await fetch(`/api/connection?action=${action}`, {
                    method: 'POST'
                });

                if (!res.ok) {
                    const error = await res.text();
                    throw new Error(error);
                }

                const result = await res.json();

                if (result.success && result.status) {
                    vpnConnected = result.status.connected;
                    vpnRouteAllEnabled = result.status.route_all;
                } else if (!result.success) {
                    // Show error but don't change state
                    console.error('Toggle failed:', result.message);
                    alert('Failed to toggle VPN: ' + result.message);
                }
            } catch (err) {
                console.error('Failed to toggle VPN:', err);
                alert('Failed to toggle VPN: ' + err.message);
            } finally {
                vpnToggleLoading = false;
                updateToggleUI();
            }
        }

        // Theme toggle function
        function toggleTheme() {
            const html = document.documentElement;
            const currentTheme = html.getAttribute('data-theme');
            const newTheme = currentTheme === 'light' ? 'dark' : 'light';

            if (newTheme === 'dark') {
                html.removeAttribute('data-theme');
            } else {
                html.setAttribute('data-theme', 'light');
            }

            // Save preference
            localStorage.setItem('vpn-theme', newTheme);
        }

        // Load saved theme preference
        function loadTheme() {
            const saved = localStorage.getItem('vpn-theme');
            if (saved === 'light') {
                document.documentElement.setAttribute('data-theme', 'light');
            }
        }

        // Initialize
        loadTheme();
        loadDashboard();
        loadConnectionStatus();
        startRefresh();

        // Also refresh connection status periodically
        setInterval(loadConnectionStatus, 10000);

        // Version tracking for auto-refresh on deployment
        let currentVersion = null;

        // Update footer with version from status and check for version changes
        async function updateFooterVersion() {
            try {
                const resp = await fetch('/api/status');
                const data = await resp.json();
                const newVersion = data.version || '0.0.0';

                document.getElementById('footer-version').textContent = 'v' + newVersion;
                document.getElementById('footer-node').textContent = data.node_name || 'Unknown';
                document.getElementById('footer-status-dot').className = 'footer-status-dot';

                // Check if version changed (deployment happened)
                if (currentVersion !== null && currentVersion !== newVersion) {
                    console.log('Version changed from', currentVersion, 'to', newVersion, '- reloading page...');
                    showUpdateNotification(currentVersion, newVersion);
                    // Give user a moment to see the notification, then reload
                    setTimeout(() => {
                        window.location.reload();
                    }, 2000);
                }
                currentVersion = newVersion;
            } catch (e) {
                document.getElementById('footer-version').textContent = 'v?.?.?';
                document.getElementById('footer-status-dot').className = 'footer-status-dot offline';
            }
        }

        // Show a notification when update is detected
        function showUpdateNotification(oldVersion, newVersion) {
            const notification = document.createElement('div');
            notification.innerHTML = `                <div style="display: flex; align-items: center; gap: 12px;">
                    <span style="font-size: 24px;">🚀</span>
                    <div>
                        <div style="font-weight: 600;">New Version Deployed!</div>
                        <div style="font-size: 12px; opacity: 0.8;">v${oldVersion} → v${newVersion}</div>
                    </div>
                </div>
            `;
            notification.style.cssText = `                position: fixed;
                top: 20px;
                right: 20px;
                background: linear-gradient(135deg, #3b82f6, #8b5cf6);
                color: white;
                padding: 16px 24px;
                border-radius: 12px;
                font-size: 14px;
                z-index: 10000;
                box-shadow: 0 10px 40px rgba(59, 130, 246, 0.4);
                animation: slideIn 0.3s ease-out;
            `;

            // Add animation keyframes
            const style = document.createElement('style');
            style.textContent = `                @keyframes slideIn {
                    from { transform: translateX(100%); opacity: 0; }
                    to { transform: translateX(0); opacity: 1; }
                }
            `;
            document.head.appendChild(style);
            document.body.appendChild(notification);
        }

        updateFooterVersion();
        // Check for version changes every 10 seconds
        setInterval(updateFooterVersion, 10000);
