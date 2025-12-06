package ui

func init() {
	indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>VPN Dashboard</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css" integrity="sha256-p4NxAoJBhIIN+hmNHrzRCf9tD/miZyoHS5obTRR9BMY=" crossorigin=""/>
    <script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js" integrity="sha256-20nQCchB9co0qIjJZRGuk2/Z9VM+kNiyxNV1lvTlZBo=" crossorigin=""></script>
    <script src="https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.min.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.8.0/lib/xterm-addon-fit.min.js"></script>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.css" />
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        :root {
            --bg-primary: #0f172a;
            --bg-secondary: #1e293b;
            --bg-card: #334155;
            --text-primary: #f8fafc;
            --text-secondary: #94a3b8;
            --accent: #3b82f6;
            --accent-hover: #2563eb;
            --success: #22c55e;
            --warning: #f59e0b;
            --error: #ef4444;
            --border: #475569;
        }

        /* Light mode theme */
        [data-theme="light"] {
            --bg-primary: #f8fafc;
            --bg-secondary: #ffffff;
            --bg-card: #e2e8f0;
            --text-primary: #1e293b;
            --text-secondary: #64748b;
            --accent: #3b82f6;
            --accent-hover: #2563eb;
            --success: #16a34a;
            --warning: #d97706;
            --error: #dc2626;
            --border: #cbd5e1;
        }

        [data-theme="light"] .sticky-footer {
            box-shadow: 0 -4px 12px rgba(0, 0, 0, 0.1);
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            min-height: 100vh;
        }

        /* Header bar - scrolls with content */
        .top-header {
            background: var(--bg-secondary);
            border-bottom: 1px solid var(--border);
            padding: 16px 24px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .header-left {
            display: flex;
            align-items: center;
            gap: 16px;
        }

        .logo {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .logo-icon {
            width: 36px;
            height: 36px;
            background: linear-gradient(135deg, var(--accent), #8b5cf6);
            border-radius: 8px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 18px;
        }

        .logo-text {
            font-size: 18px;
            font-weight: 600;
        }

        .header-status {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 6px 12px;
            background: var(--bg-card);
            border-radius: 20px;
            font-size: 13px;
        }

        .header-status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: var(--success);
        }

        .header-status-dot.offline {
            background: var(--error);
        }

        /* Main content - single page */
        .main {
            flex: 1;
            padding: 24px;
            padding-bottom: 100px; /* Space for sticky footer */
            max-width: 1400px;
            margin: 0 auto;
        }

        /* Section headers */
        .section-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 16px;
        }

        .section-title {
            font-size: 18px;
            font-weight: 600;
        }

        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 24px;
        }

        .page-title {
            font-size: 24px;
            font-weight: 600;
        }

        .status-badge {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 8px 16px;
            background: var(--bg-card);
            border-radius: 20px;
            font-size: 14px;
        }

        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: var(--success);
        }

        .status-dot.offline {
            background: var(--error);
        }

        /* Dashboard pages */
        .page {
            display: none;
        }

        .page.active {
            display: block;
        }

        /* Cards grid */
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 16px;
            margin-bottom: 24px;
        }

        .stat-card {
            background: var(--bg-secondary);
            border-radius: 12px;
            padding: 20px;
            border: 1px solid var(--border);
        }

        .stat-label {
            font-size: 12px;
            color: var(--text-secondary);
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 8px;
        }

        .stat-value {
            font-size: 24px;
            font-weight: 600;
        }

        .stat-value.small {
            font-size: 18px;
        }

        .stat-change {
            font-size: 12px;
            color: var(--success);
            margin-top: 4px;
        }

        .stat-change.verified {
            color: var(--success);
        }

        .stat-change.not-verified {
            color: var(--error);
        }

        /* Charts - FIXED HEIGHT */
        .chart-container {
            background: var(--bg-secondary);
            border-radius: 12px;
            padding: 20px;
            border: 1px solid var(--border);
            margin-bottom: 24px;
        }

        .chart-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 16px;
        }

        .chart-title {
            font-size: 16px;
            font-weight: 600;
        }

        .chart-controls {
            display: flex;
            gap: 8px;
        }

        .chart-btn {
            padding: 6px 12px;
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 6px;
            color: var(--text-secondary);
            font-size: 12px;
            cursor: pointer;
            transition: all 0.2s;
        }

        .chart-btn:hover, .chart-btn.active {
            background: var(--accent);
            color: white;
            border-color: var(--accent);
        }

        /* FIXED HEIGHT CHART WRAPPER */
        .chart-wrapper {
            position: relative;
            height: 200px;
            width: 100%;
        }

        .chart-wrapper.small {
            height: 160px;
        }

        /* Peers table */
        .table-container {
            background: var(--bg-secondary);
            border-radius: 12px;
            border: 1px solid var(--border);
            overflow: hidden;
            margin-bottom: 24px;
        }

        .table-header {
            padding: 16px 20px;
            border-bottom: 1px solid var(--border);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .table-title {
            font-size: 16px;
            font-weight: 600;
        }

        table {
            width: 100%;
            border-collapse: collapse;
        }

        th, td {
            padding: 12px 20px;
            text-align: left;
            border-bottom: 1px solid var(--border);
        }

        th {
            font-size: 12px;
            color: var(--text-secondary);
            text-transform: uppercase;
            letter-spacing: 0.5px;
            font-weight: 500;
        }

        td {
            font-size: 14px;
        }

        tr:last-child td {
            border-bottom: none;
        }

        tr:hover {
            background: var(--bg-card);
        }

        .peer-name {
            display: flex;
            align-items: center;
            gap: 10px;
        }

        .peer-avatar {
            width: 32px;
            height: 32px;
            border-radius: 8px;
            background: linear-gradient(135deg, var(--accent), #8b5cf6);
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 14px;
        }

        /* Logs (Observability) */
        .logs-container {
            background: var(--bg-secondary);
            border-radius: 12px;
            border: 1px solid var(--border);
        }

        .logs-toolbar {
            padding: 16px 20px;
            border-bottom: 1px solid var(--border);
            display: flex;
            gap: 12px;
            flex-wrap: wrap;
        }

        .search-input {
            flex: 1;
            min-width: 200px;
            padding: 10px 14px;
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 8px;
            color: var(--text-primary);
            font-size: 14px;
        }

        .search-input:focus {
            outline: none;
            border-color: var(--accent);
        }

        .filter-select {
            padding: 10px 14px;
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 8px;
            color: var(--text-primary);
            font-size: 14px;
            cursor: pointer;
        }

        .time-range {
            display: flex;
            gap: 8px;
        }

        .time-btn {
            padding: 10px 14px;
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 8px;
            color: var(--text-secondary);
            font-size: 13px;
            cursor: pointer;
            transition: all 0.2s;
        }

        .time-btn:hover, .time-btn.active {
            background: var(--accent);
            color: white;
            border-color: var(--accent);
        }

        .logs-list {
            max-height: 500px;
            overflow-y: auto;
        }

        .log-entry {
            padding: 12px 20px;
            border-bottom: 1px solid var(--border);
            font-family: 'Monaco', 'Menlo', monospace;
            font-size: 12px;
            display: flex;
            gap: 16px;
        }

        .log-entry:hover {
            background: var(--bg-card);
        }

        .log-time {
            color: var(--text-secondary);
            white-space: nowrap;
        }

        .log-level {
            padding: 2px 8px;
            border-radius: 4px;
            font-size: 10px;
            font-weight: 600;
            text-transform: uppercase;
        }

        .log-level.DEBUG { background: var(--bg-card); color: var(--text-secondary); }
        .log-level.INFO { background: rgba(59, 130, 246, 0.2); color: var(--accent); }
        .log-level.WARN { background: rgba(245, 158, 11, 0.2); color: var(--warning); }
        .log-level.ERROR { background: rgba(239, 68, 68, 0.2); color: var(--error); }

        .log-component {
            color: #8b5cf6;
        }

        .log-message {
            flex: 1;
            word-break: break-word;
        }

        /* Metrics charts for observability */
        .metrics-grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 16px;
            margin-bottom: 24px;
        }

        @media (max-width: 1200px) {
            .metrics-grid {
                grid-template-columns: 1fr;
            }
        }

        /* Empty state */
        .empty-state {
            text-align: center;
            padding: 60px 20px;
            color: var(--text-secondary);
        }

        .empty-icon {
            font-size: 48px;
            margin-bottom: 16px;
        }

        /* Loading spinner */
        .loading {
            display: flex;
            justify-content: center;
            align-items: center;
            padding: 40px;
        }

        .spinner {
            width: 40px;
            height: 40px;
            border: 3px solid var(--border);
            border-top-color: var(--accent);
            border-radius: 50%;
            animation: spin 1s linear infinite;
        }

        @keyframes spin {
            to { transform: rotate(360deg); }
        }

        /* Network Graph / Map */
        .network-graph-container {
            background: var(--bg-secondary);
            border-radius: 12px;
            border: 1px solid var(--border);
            margin-bottom: 24px;
            overflow: hidden;
        }

        .graph-header {
            padding: 16px 20px;
            border-bottom: 1px solid var(--border);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .graph-title {
            font-size: 16px;
            font-weight: 600;
        }

        #network-graph {
            width: 100%;
            height: 450px;
            background: var(--bg-primary);
        }

        /* Leaflet customizations for dark theme */
        .leaflet-container {
            background: #1a1a2e;
        }

        .leaflet-popup-content-wrapper {
            background: var(--bg-secondary);
            color: var(--text-primary);
            border-radius: 8px;
        }

        .leaflet-popup-tip {
            background: var(--bg-secondary);
        }

        .leaflet-popup-content {
            margin: 12px;
        }

        .node-popup {
            min-width: 180px;
        }

        .node-popup-name {
            font-weight: 600;
            font-size: 14px;
            margin-bottom: 8px;
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .node-popup-info {
            font-size: 12px;
            color: var(--text-secondary);
            line-height: 1.6;
        }

        .node-popup-info strong {
            color: var(--text-primary);
        }

        /* Great circle arc animation */
        .leaflet-arc-path {
            stroke-dasharray: 10, 5;
            animation: dash-flow 20s linear infinite;
        }

        @keyframes dash-flow {
            to {
                stroke-dashoffset: -1000;
            }
        }

        /* Distance badges */
        .distance-badge {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            padding: 4px 10px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 600;
        }

        .distance-badge.distance-0 {
            background: rgba(139, 92, 246, 0.2);
            color: #a78bfa;
        }

        .distance-badge.distance-1 {
            background: rgba(34, 197, 94, 0.2);
            color: var(--success);
        }

        .distance-badge.distance-2 {
            background: rgba(245, 158, 11, 0.2);
            color: var(--warning);
        }

        .distance-badge.distance-3 {
            background: rgba(239, 68, 68, 0.2);
            color: var(--error);
        }

        .self-indicator {
            background: linear-gradient(135deg, #8b5cf6, #3b82f6);
            color: white;
            padding: 2px 8px;
            border-radius: 4px;
            font-size: 10px;
            margin-left: 8px;
        }

        /* Sortable table headers */
        th.sortable {
            cursor: pointer;
            user-select: none;
        }

        th.sortable:hover {
            background: var(--bg-card);
        }

        th.sortable::after {
            content: ' ↕';
            opacity: 0.5;
        }

        th.sortable.asc::after {
            content: ' ↑';
            opacity: 1;
        }

        th.sortable.desc::after {
            content: ' ↓';
            opacity: 1;
        }

        /* Verify card */
        .verify-card {
            background: var(--bg-secondary);
            border-radius: 12px;
            padding: 20px;
            border: 1px solid var(--border);
            margin-bottom: 24px;
        }

        .verify-title {
            font-size: 16px;
            font-weight: 600;
            margin-bottom: 16px;
        }

        .verify-row {
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            border-bottom: 1px solid var(--border);
        }

        .verify-row:last-child {
            border-bottom: none;
        }

        .verify-label {
            color: var(--text-secondary);
        }

        .verify-value {
            font-weight: 500;
        }

        .verify-value.success {
            color: var(--success);
        }

        .verify-value.error {
            color: var(--error);
        }

        /* Action button */
        .action-btn {
            padding: 10px 20px;
            background: var(--accent);
            border: none;
            border-radius: 8px;
            color: white;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            transition: background 0.2s;
        }

        .action-btn:hover {
            background: var(--accent-hover);
        }

        .action-btn:disabled {
            background: var(--bg-card);
            cursor: not-allowed;
        }

        /* SSH button */
        .ssh-btn {
            padding: 6px 12px;
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 6px;
            color: var(--text-primary);
            font-size: 12px;
            cursor: pointer;
            transition: all 0.2s;
            display: inline-flex;
            align-items: center;
            gap: 6px;
            text-decoration: none;
        }

        .ssh-btn:hover {
            background: var(--accent);
            border-color: var(--accent);
        }

        .ssh-btn.disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }

        /* SSH table cell */
        .ssh-cell {
            white-space: nowrap;
        }

        .ssh-table-btn {
            padding: 4px 8px;
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 4px;
            color: var(--text-primary);
            cursor: pointer;
            transition: all 0.2s;
            display: inline-flex;
            align-items: center;
            vertical-align: middle;
            margin-right: 8px;
        }

        .ssh-table-btn:hover {
            background: var(--accent);
            border-color: var(--accent);
            color: white;
        }

        /* Screen Share table cell */
        .screen-cell {
            white-space: nowrap;
        }

        .screen-table-btn {
            padding: 4px 8px;
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 4px;
            color: var(--text-primary);
            cursor: pointer;
            font-size: 12px;
            display: inline-flex;
            align-items: center;
            vertical-align: middle;
        }

        .screen-table-btn:hover {
            background: #8b5cf6;
            border-color: #8b5cf6;
            color: white;
        }

        .ssh-cmd {
            font-family: 'Fira Code', 'Monaco', 'Consolas', monospace;
            font-size: 11px;
            background: var(--bg-card);
            padding: 4px 8px;
            border-radius: 4px;
            color: var(--text-secondary);
            cursor: pointer;
            transition: all 0.2s;
            vertical-align: middle;
        }

        .ssh-cmd:hover {
            background: var(--accent);
            color: white;
        }

        /* Network peers grid */
        .peers-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
            gap: 16px;
            margin-bottom: 24px;
        }

        .peer-card {
            background: var(--bg-secondary);
            border-radius: 12px;
            padding: 16px;
            border: 1px solid var(--border);
            display: flex;
            flex-direction: column;
            gap: 12px;
        }

        .peer-card.is-self {
            border-color: var(--accent);
            background: linear-gradient(135deg, rgba(59, 130, 246, 0.1), rgba(139, 92, 246, 0.1));
        }

        .peer-card-header {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .peer-card-avatar {
            width: 40px;
            height: 40px;
            border-radius: 10px;
            background: linear-gradient(135deg, var(--accent), #8b5cf6);
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 16px;
            font-weight: 600;
        }

        .peer-card-info {
            flex: 1;
        }

        .peer-card-name {
            font-weight: 600;
            font-size: 14px;
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .peer-card-ip {
            font-size: 12px;
            color: var(--text-secondary);
            font-family: monospace;
        }

        .peer-card-meta {
            display: flex;
            gap: 12px;
            font-size: 12px;
            color: var(--text-secondary);
        }

        .peer-card-actions {
            display: flex;
            gap: 8px;
            margin-top: auto;
        }

        .you-badge {
            font-size: 10px;
            padding: 2px 6px;
            background: var(--accent);
            border-radius: 4px;
            color: white;
        }

        .os-badge {
            font-size: 10px;
            padding: 2px 6px;
            background: var(--bg-card);
            border-radius: 4px;
            color: var(--text-secondary);
        }

        /* VPN Toggle Switch */
        .vpn-toggle-container {
            display: flex;
            align-items: center;
            gap: 12px;
            padding: 12px 16px;
            background: var(--bg-secondary);
            border: 1px solid var(--border);
            border-radius: 12px;
            margin-bottom: 16px;
        }

        .vpn-toggle-label {
            font-size: 14px;
            font-weight: 500;
        }

        .vpn-toggle-status {
            font-size: 12px;
            color: var(--text-secondary);
        }

        .vpn-toggle-status.on {
            color: var(--success);
        }

        .vpn-toggle-status.off {
            color: var(--text-secondary);
        }

        .toggle-switch {
            position: relative;
            width: 56px;
            height: 28px;
            background: var(--bg-card);
            border-radius: 14px;
            cursor: pointer;
            transition: background 0.3s;
            border: 2px solid var(--border);
        }

        .toggle-switch.on {
            background: var(--success);
            border-color: var(--success);
        }

        .toggle-switch.loading {
            opacity: 0.6;
            cursor: wait;
        }

        .toggle-knob {
            position: absolute;
            top: 2px;
            left: 2px;
            width: 20px;
            height: 20px;
            background: white;
            border-radius: 50%;
            transition: transform 0.3s;
            box-shadow: 0 2px 4px rgba(0,0,0,0.2);
        }

        .toggle-switch.on .toggle-knob {
            transform: translateX(28px);
        }

        /* Sidebar status indicator */
        .sidebar-status {
            padding: 16px;
            border-top: 1px solid var(--border);
            margin-top: auto;
        }

        .sidebar-status-row {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 8px;
        }

        .sidebar-status-label {
            font-size: 12px;
            color: var(--text-secondary);
        }

        .sidebar-mini-toggle {
            width: 36px;
            height: 18px;
            background: var(--bg-card);
            border-radius: 9px;
            cursor: pointer;
            position: relative;
            transition: background 0.3s;
        }

        .sidebar-mini-toggle.on {
            background: var(--success);
        }

        .sidebar-mini-toggle .mini-knob {
            position: absolute;
            top: 2px;
            left: 2px;
            width: 14px;
            height: 14px;
            background: white;
            border-radius: 50%;
            transition: transform 0.3s;
        }

        .sidebar-mini-toggle.on .mini-knob {
            transform: translateX(18px);
        }

        /* Terminal Modal */
        .terminal-modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0, 0, 0, 0.8);
            z-index: 1000;
            justify-content: center;
            align-items: center;
        }

        .terminal-modal.open {
            display: flex;
        }

        .terminal-container {
            width: 90%;
            max-width: 1000px;
            height: 80%;
            background: var(--bg-secondary);
            border-radius: 12px;
            overflow: hidden;
            display: flex;
            flex-direction: column;
            border: 1px solid var(--border);
            box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
        }

        .terminal-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 12px 16px;
            background: var(--bg-card);
            border-bottom: 1px solid var(--border);
        }

        .terminal-title {
            display: flex;
            align-items: center;
            gap: 10px;
            font-size: 14px;
            font-weight: 500;
        }

        .terminal-title-dot {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            background: var(--success);
        }

        .terminal-close-btn {
            background: transparent;
            border: none;
            color: var(--text-secondary);
            font-size: 24px;
            cursor: pointer;
            padding: 4px 8px;
            border-radius: 4px;
            transition: all 0.2s;
        }

        .terminal-close-btn:hover {
            background: var(--error);
            color: white;
        }

        .terminal-body {
            flex: 1;
            padding: 8px;
            overflow: hidden;
        }

        .terminal-body .xterm {
            height: 100%;
        }

        .terminal-info {
            display: flex;
            align-items: center;
            justify-content: center;
            height: 100%;
            color: var(--text-secondary);
            font-size: 14px;
            flex-direction: column;
            gap: 16px;
        }

        .terminal-info-icon {
            font-size: 48px;
        }

        .terminal-info-cmd {
            font-family: monospace;
            background: var(--bg-card);
            padding: 12px 20px;
            border-radius: 8px;
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .terminal-copy-btn {
            padding: 6px 12px;
            background: var(--accent);
            border: none;
            border-radius: 6px;
            color: white;
            font-size: 12px;
            cursor: pointer;
        }

        .terminal-copy-btn:hover {
            background: var(--accent-hover);
        }

        /* Sticky Footer with VPN controls - always on top */
        .sticky-footer {
            position: fixed;
            bottom: 0;
            left: 0;
            right: 0;
            padding: 12px 24px;
            background: var(--bg-secondary);
            border-top: 1px solid var(--border);
            display: flex;
            justify-content: space-between;
            align-items: center;
            z-index: 99999;
            box-shadow: 0 -4px 12px rgba(0, 0, 0, 0.2);
        }

        .footer-left {
            display: flex;
            align-items: center;
            gap: 20px;
        }

        .footer-vpn-control {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .footer-vpn-label {
            font-size: 13px;
            font-weight: 500;
        }

        .footer-vpn-status {
            font-size: 12px;
            padding: 4px 10px;
            border-radius: 12px;
        }

        .footer-vpn-status.on {
            background: rgba(34, 197, 94, 0.2);
            color: var(--success);
        }

        .footer-vpn-status.off {
            background: rgba(239, 68, 68, 0.2);
            color: var(--error);
        }

        .footer-center {
            display: flex;
            align-items: center;
            gap: 16px;
            font-size: 12px;
            color: var(--text-secondary);
        }

        .footer-ip {
            font-family: monospace;
            padding: 4px 10px;
            background: var(--bg-card);
            border-radius: 6px;
        }

        .footer-right {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .footer-btn {
            padding: 8px 16px;
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 8px;
            color: var(--text-primary);
            font-size: 13px;
            cursor: pointer;
            transition: all 0.2s;
            display: flex;
            align-items: center;
            gap: 6px;
        }

        .footer-btn:hover {
            background: var(--accent);
            border-color: var(--accent);
        }

        .footer-btn.primary {
            background: var(--accent);
            border-color: var(--accent);
        }

        .footer-btn.primary:hover {
            background: var(--accent-hover);
        }

        .footer-btn:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }

        .footer-version {
            font-family: monospace;
            font-size: 11px;
            color: var(--text-secondary);
        }

        /* Theme toggle button */
        .theme-toggle {
            width: 36px;
            height: 36px;
            border-radius: 50%;
            border: 1px solid var(--border);
            background: var(--bg-card);
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            transition: all 0.3s ease;
        }

        .theme-toggle:hover {
            background: var(--accent);
            border-color: var(--accent);
        }

        .theme-toggle svg {
            width: 18px;
            height: 18px;
            fill: var(--text-primary);
            transition: transform 0.3s ease;
        }

        .theme-toggle:hover svg {
            transform: rotate(15deg);
        }

        /* Hide sun in dark mode, show moon */
        .theme-toggle .sun-icon { display: none; }
        .theme-toggle .moon-icon { display: block; }

        /* Light mode: show sun, hide moon */
        [data-theme="light"] .theme-toggle .sun-icon { display: block; }
        [data-theme="light"] .theme-toggle .moon-icon { display: none; }
    </style>
</head>
<body>
    <!-- Top Header -->
    <header class="top-header">
        <div class="header-left">
            <div class="logo">
                <div class="logo-icon">V</div>
                <span class="logo-text">VPN Mesh</span>
            </div>
            <div class="header-status">
                <span class="header-status-dot" id="header-status-dot"></span>
                <span id="header-status-text">Connecting...</span>
            </div>
        </div>
        <div class="stats-grid" style="margin:0; gap:12px;">
            <div class="stat-card" style="padding:10px 16px; margin:0;">
                <div class="stat-label">Node</div>
                <div class="stat-value small" id="home-node-name">-</div>
            </div>
            <div class="stat-card" style="padding:10px 16px; margin:0;">
                <div class="stat-label">VPN IP</div>
                <div class="stat-value small" id="home-vpn-ip">-</div>
            </div>
            <div class="stat-card" style="padding:10px 16px; margin:0;">
                <div class="stat-label">Uptime</div>
                <div class="stat-value small" id="home-uptime">-</div>
            </div>
            <div class="stat-card" style="padding:10px 16px; margin:0;">
                <div class="stat-label">Version</div>
                <div class="stat-value small" id="home-version">-</div>
            </div>
        </div>
    </header>

    <!-- Main content - single scrollable page -->
    <main class="main">
        <!-- Section 1: Network Map -->
        <div class="section-header">
            <h2 class="section-title">Network Map</h2>
            <div class="chart-controls">
                <button class="chart-btn" onclick="fitNetworkMap()">Fit to Nodes</button>
            </div>
        </div>
        <div class="network-graph-container" style="margin-bottom: 24px;">
            <div id="network-graph"></div>
        </div>

        <!-- Section 2: Peers Table -->
        <div class="section-header">
            <h2 class="section-title">Network Nodes</h2>
            <span style="color: var(--text-secondary); font-size: 12px;" id="topology-node-count"></span>
        </div>
        <div class="table-container" style="margin-bottom: 24px;">
            <table>
                <thead>
                    <tr>
                        <th class="sortable" data-sort="distance">Distance</th>
                        <th class="sortable" data-sort="name">Name</th>
                        <th>VPN IP</th>
                        <th>Public IP</th>
                        <th class="sortable" data-sort="latency">Latency</th>
                        <th class="sortable" data-sort="bandwidth">Bandwidth</th>
                        <th>Status</th>
                        <th>SSH</th>
                        <th>Screen</th>
                    </tr>
                </thead>
                <tbody id="all-peers-tbody">
                </tbody>
            </table>
        </div>

        <!-- Section 2b: Install Handshakes History -->
        <div class="section-header">
            <h2 class="section-title">Install Handshakes</h2>
            <span style="color: var(--text-secondary); font-size: 12px;" id="handshakes-count"></span>
        </div>
        <div class="table-container" style="margin-bottom: 24px;">
            <table>
                <thead>
                    <tr>
                        <th class="sortable" data-sort="timestamp">Timestamp</th>
                        <th class="sortable" data-sort="node_name">Node</th>
                        <th>VPN IP</th>
                        <th>Public IP</th>
                        <th>Version</th>
                        <th>OS</th>
                        <th>Ping</th>
                        <th>SSH</th>
                    </tr>
                </thead>
                <tbody id="handshakes-tbody">
                </tbody>
            </table>
        </div>

        <!-- Section 3: Observability -->
        <div class="section-header">
            <h2 class="section-title">Observability</h2>
            <div class="chart-controls">
                <button class="chart-btn metrics-range active" data-range="-5m">5m</button>
                <button class="chart-btn metrics-range" data-range="-15m">15m</button>
                <button class="chart-btn metrics-range" data-range="-1h">1h</button>
                <button class="chart-btn metrics-range" data-range="-6h">6h</button>
            </div>
        </div>

            <div class="metrics-grid">
                <div class="chart-container">
                    <div class="chart-header">
                        <span class="chart-title">Bandwidth (TX/RX)</span>
                    </div>
                    <div class="chart-wrapper small">
                        <canvas id="obs-bandwidth-chart"></canvas>
                    </div>
                </div>
                <div class="chart-container">
                    <div class="chart-header">
                        <span class="chart-title">Bytes Sent/Received</span>
                    </div>
                    <div class="chart-wrapper small">
                        <canvas id="bytes-chart"></canvas>
                    </div>
                </div>
                <div class="chart-container">
                    <div class="chart-header">
                        <span class="chart-title">Packets Sent/Received</span>
                    </div>
                    <div class="chart-wrapper small">
                        <canvas id="packets-chart"></canvas>
                    </div>
                </div>
                <div class="chart-container">
                    <div class="chart-header">
                        <span class="chart-title">Active Peers</span>
                    </div>
                    <div class="chart-wrapper small">
                        <canvas id="peers-chart"></canvas>
                    </div>
                </div>
            </div>

            <div class="logs-container">
                <div class="logs-toolbar">
                    <input type="text" class="search-input" id="log-search" placeholder="Search logs...">
                    <select class="filter-select" id="log-level-filter">
                        <option value="">All Levels</option>
                        <option value="DEBUG">DEBUG</option>
                        <option value="INFO">INFO</option>
                        <option value="WARN">WARN</option>
                        <option value="ERROR">ERROR</option>
                    </select>
                    <select class="filter-select" id="log-component-filter">
                        <option value="">All Components</option>
                        <option value="node">node</option>
                        <option value="conn">conn</option>
                        <option value="tun">tun</option>
                        <option value="store">store</option>
                        <option value="control">control</option>
                    </select>
                    <select class="filter-select" id="log-peer-filter">
                        <option value="">All Peers</option>
                        <!-- Populated dynamically from network peers -->
                    </select>
                    <div class="time-range">
                        <button class="time-btn active" data-range="-15m">15m</button>
                        <button class="time-btn" data-range="-1h">1h</button>
                        <button class="time-btn" data-range="-6h">6h</button>
                        <button class="time-btn" data-range="-24h">24h</button>
                    </div>
                </div>
                <div class="logs-list" id="logs-list">
                    <div class="loading"><div class="spinner"></div></div>
                </div>
            </div>
    </main>

    <!-- Sticky Footer with VPN controls -->
    <footer class="sticky-footer">
        <div class="footer-left">
            <div class="footer-vpn-control">
                <span class="footer-vpn-label">VPN Routing</span>
                <span class="footer-vpn-status" id="footer-vpn-status">-</span>
                <div class="toggle-switch" id="footer-vpn-toggle" onclick="toggleVPN()">
                    <div class="toggle-knob"></div>
                </div>
            </div>
        </div>
        <div class="footer-center">
            <span>Public IP:</span>
            <span class="footer-ip" id="footer-public-ip">Checking...</span>
            <span id="footer-verify-status"></span>
        </div>
        <div class="footer-right">
            <button class="footer-btn" onclick="loadVerify()">
                <span>&#9989;</span> Verify
            </button>
            <button class="theme-toggle" onclick="toggleTheme()" title="Toggle dark/light mode">
                <svg class="sun-icon" viewBox="0 0 24 24"><path d="M12 7a5 5 0 100 10 5 5 0 000-10zm0-5a1 1 0 011 1v2a1 1 0 11-2 0V3a1 1 0 011-1zm0 18a1 1 0 011 1v2a1 1 0 11-2 0v-2a1 1 0 011-1zm9-9a1 1 0 110 2h-2a1 1 0 110-2h2zM5 12a1 1 0 110 2H3a1 1 0 110-2h2zm14.07-6.07a1 1 0 010 1.41l-1.41 1.42a1 1 0 11-1.42-1.42l1.42-1.41a1 1 0 011.41 0zM7.76 16.24a1 1 0 010 1.41l-1.42 1.42a1 1 0 11-1.41-1.42l1.41-1.41a1 1 0 011.42 0zm10.48 0a1 1 0 011.42 0l1.41 1.41a1 1 0 11-1.41 1.42l-1.42-1.42a1 1 0 010-1.41zM7.76 7.76a1 1 0 01-1.42 0L4.93 6.34a1 1 0 111.41-1.41l1.42 1.41a1 1 0 010 1.42z"/></svg>
                <svg class="moon-icon" viewBox="0 0 24 24"><path d="M21 12.79A9 9 0 1111.21 3a7 7 0 109.79 9.79z"/></svg>
            </button>
            <span class="footer-version" id="footer-version">v-</span>
        </div>
    </footer>

    <!-- Terminal Modal for SSH -->
    <div id="terminal-modal" class="terminal-modal" onclick="closeTerminalOnBackdrop(event)">
        <div class="terminal-container">
            <div class="terminal-header">
                <div class="terminal-title">
                    <span class="terminal-title-dot" id="terminal-status-dot"></span>
                    <span id="terminal-title-text">SSH Terminal</span>
                </div>
                <button class="terminal-close-btn" onclick="closeTerminal()">&times;</button>
            </div>
            <div class="terminal-body" id="terminal-body">
                <!-- xterm.js terminal will be mounted here -->
            </div>
        </div>
    </div>

    <script>
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
                    const osBadge = peer.os ? ` + "`" + `<span class="os-badge">${peer.os}</span>` + "`" + ` : '';
                    const youBadge = isUs ? '<span class="you-badge">YOU</span>' : '';

                    // SSH command - uses VPN internal IP
                    // For server (linux), use root@. For clients (darwin), use miguel_lemos (family default)
                    const sshUser = peer.os === 'linux' ? 'root' : 'miguel_lemos';
                    const sshTarget = peer.vpn_address;
                    const sshCmd = ` + "`" + `ssh ${sshUser}@${sshTarget}` + "`" + `;

                    // Disable SSH button for ourselves
                    const sshBtnClass = isUs ? 'ssh-btn disabled' : 'ssh-btn';
                    const sshBtnOnClick = isUs ? '' : ` + "`" + `onclick="openSSHTerminal('${peer.name || 'Unknown'}', '${sshCmd}')"` + "`" + `;

                    return ` + "`" + `
                        <div class="peer-card ${isUs ? 'is-self' : ''}">
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
                    ` + "`" + `;
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
                        ? ` + "`" + `<span style="color: var(--success);">${entry.ping_test_ms}ms</span>` + "`" + `
                        : '<span style="color: var(--error);">FAIL</span>';
                    const sshBadge = entry.ssh_test_ok
                        ? '<span style="color: var(--success);">OK</span>'
                        : '<span style="color: var(--error);">FAIL</span>';
                    const osDisplay = entry.os + '/' + entry.arch;

                    return ` + "`" + `
                        <tr>
                            <td>${ts}</td>
                            <td>${entry.node_name || '-'}</td>
                            <td>${entry.vpn_address || '-'}</td>
                            <td>${entry.public_ip || '-'}</td>
                            <td><code>${entry.version || '-'}</code></td>
                            <td>${osDisplay}</td>
                            <td>${pingBadge}</td>
                            <td>${sshBadge}</td>
                        </tr>
                    ` + "`" + `;
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
                notification.style.cssText = ` + "`" + `
                    position: fixed;
                    bottom: 20px;
                    right: 20px;
                    background: var(--success);
                    color: white;
                    padding: 12px 20px;
                    border-radius: 8px;
                    font-size: 14px;
                    z-index: 1000;
                    animation: fadeIn 0.3s ease;
                ` + "`" + `;
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
                    tbody.innerHTML = networkNodes.map(p => ` + "`" + `
                        <tr>
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
                    ` + "`" + `).join('');
                } else {
                    const message = isVPNActive
                        ? 'No other peers in network'
                        : 'VPN routing disabled - enable to see peers';
                    tbody.innerHTML = ` + "`" + `<tr><td colspan="4" style="text-align:center;color:var(--text-secondary)">${message}</td></tr>` + "`" + `;
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
                const res = await fetch(` + "`" + `/api/stats?earliest=${currentBandwidthRange}&granularity=raw` + "`" + `);
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
                    option.textContent = ` + "`" + `${name} (${vpnAddr})` + "`" + `;
                    select.appendChild(option);
                });
            } catch (err) {
                console.error('Failed to load peer filter options:', err);
            }
        }

        // Load metrics charts
        async function loadMetricsCharts() {
            try {
                const res = await fetch(` + "`" + `/api/stats?earliest=${currentMetricsRange}&granularity=raw` + "`" + `);
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
            let url = ` + "`" + `/api/logs?earliest=${currentLogRange}` + "`" + `;
            if (peerFilter) url += ` + "`" + `&peer=${encodeURIComponent(peerFilter)}` + "`" + `;
            if (search) url += ` + "`" + `&search=${encodeURIComponent(search)}` + "`" + `;
            if (level) url += ` + "`" + `&level=${level}` + "`" + `;
            if (component) url += ` + "`" + `&component=${component}` + "`" + `;

            try {
                const res = await fetch(url);
                const data = await res.json();

                const container = document.getElementById('logs-list');
                const entries = data.entries || [];

                if (entries.length > 0) {
                    container.innerHTML = entries.map(e => ` + "`" + `
                        <div class="log-entry">
                            <span class="log-time">${e.timestamp?.substring(0, 19) || ''}</span>
                            <span class="log-level ${e.level}">${e.level}</span>
                            <span class="log-component">[${e.component}]</span>
                            <span class="log-message">${escapeHtml(e.message)}</span>
                        </div>
                    ` + "`" + `).join('');
                } else {
                    const peerMsg = peerFilter ? ' from remote peer (may be unreachable)' : '';
                    container.innerHTML = ` + "`" + `<div class="empty-state"><p>No logs found${peerMsg}</p></div>` + "`" + `;
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
                    ` + "`" + `${(data.nodes || []).length} nodes, ${(data.edges || []).length} connections${statusText}` + "`" + `;
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
            return ` + "`" + `${lat.toFixed(2)},${lon.toFixed(2)}` + "`" + `;
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
                    ? ` + "`" + `${groupNodes[0].geo.city || ''}, ${groupNodes[0].geo.country || ''}` + "`" + `.replace(/^, |, $/g, '')
                    : 'Unknown';

                let popupContent = ` + "`" + `<div class="node-popup">` + "`" + `;

                if (nodeCount > 1) {
                    popupContent += ` + "`" + `<div style="font-size: 12px; color: var(--text-secondary); margin-bottom: 8px;">
                        ${nodeCount} nodes at ${location}
                    </div>` + "`" + `;
                }

                // List all nodes at this location
                groupNodes.forEach((n, idx) => {
                    const isUs = isOurNode(n);
                    const isHelsinkiNode = n.vpn_address === HELSINKI_VPN_IP;
                    const icon = isUs ? '⭐' : (isHelsinkiNode ? '🌐' : '💻');
                    const youBadge = isUs ? '<span class="you-badge">YOU</span>' : '';

                    if (idx > 0) {
                        popupContent += ` + "`" + `<div style="border-top: 1px solid var(--border); margin: 8px 0;"></div>` + "`" + `;
                    }

                    popupContent += ` + "`" + `
                        <div class="node-popup-name">
                            ${icon} ${n.name || 'Unknown'} ${youBadge}
                        </div>
                        <div class="node-popup-info">
                            <strong>VPN IP:</strong> ${n.vpn_address || '-'}<br>
                            ${nodeCount === 1 ? '<strong>Location:</strong> ' + location + '<br>' : ''}
                            <strong>Distance:</strong> ${n.distance === 0 ? 'Local' : n.distance + ' hop(s)'}<br>
                            ${n.latency_ms ? '<strong>Latency:</strong> ' + n.latency_ms.toFixed(1) + ' ms<br>' : ''}
                            ${n.geo?.isp ? '<strong>ISP:</strong> ' + n.geo.isp + '<br>' : ''}
                        </div>
                    ` + "`" + `;
                });

                popupContent += ` + "`" + `</div>` + "`" + `;

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
                    const distanceClass = ` + "`" + `distance-${Math.min(n.distance || 0, 3)}` + "`" + `;
                    const distanceLabel = isUs ? 'You' :
                                          n.distance === 1 ? '1 hop' :
                                          ` + "`" + `${n.distance} hops` + "`" + `;
                    const selfBadge = isUs ? '<span class="self-indicator">YOU</span>' : '';
                    const statusColor = isUs || n.is_direct ? 'var(--success)' : 'var(--text-secondary)';
                    const statusText = isUs ? 'Local' : (n.is_direct ? 'Direct' : 'Via Relay');

                    // SSH command - uses VPN internal IP, root for linux, miguel_lemos for darwin
                    const sshUser = n.os === 'linux' ? 'root' : 'miguel_lemos';
                    const sshCmd = ` + "`" + `ssh ${sshUser}@${n.vpn_address}` + "`" + `;
                    const sshDisabled = isUs;

                    return ` + "`" + `
                        <tr>
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
                                ${sshDisabled ? '<span style="color: var(--text-secondary)">-</span>' : ` + "`" + `
                                    <button class="ssh-table-btn" onclick="openSSHTerminal('${n.name || 'Unknown'}', '${sshCmd}')" title="Open SSH terminal">
                                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                            <rect x="2" y="3" width="20" height="18" rx="2"/>
                                            <path d="M7 8l4 4-4 4"/>
                                            <line x1="13" y1="16" x2="17" y2="16"/>
                                        </svg>
                                    </button>
                                    <code class="ssh-cmd" onclick="copySSHCommand('${sshCmd}')" title="Click to copy">${sshCmd}</code>
                                ` + "`" + `}
                            </td>
                            <td class="screen-cell">
                                ${(sshDisabled || n.os === 'linux') ? '<span style="color: var(--text-secondary)">-</span>' : ` + "`" + `
                                    <button class="screen-table-btn" onclick="openScreenShare('${n.vpn_address}', '${sshUser}')" title="Open Screen Sharing (VNC)">
                                        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                            <rect x="2" y="3" width="20" height="14" rx="2"/>
                                            <line x1="8" y1="21" x2="16" y2="21"/>
                                            <line x1="12" y1="17" x2="12" y2="21"/>
                                        </svg>
                                    </button>
                                ` + "`" + `}
                            </td>
                        </tr>
                    ` + "`" + `;
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
                const res = await fetch(` + "`" + `/api/connection?action=${action}` + "`" + `, {
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
            notification.innerHTML = ` + "`" + `
                <div style="display: flex; align-items: center; gap: 12px;">
                    <span style="font-size: 24px;">🚀</span>
                    <div>
                        <div style="font-weight: 600;">New Version Deployed!</div>
                        <div style="font-size: 12px; opacity: 0.8;">v${oldVersion} → v${newVersion}</div>
                    </div>
                </div>
            ` + "`" + `;
            notification.style.cssText = ` + "`" + `
                position: fixed;
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
            ` + "`" + `;

            // Add animation keyframes
            const style = document.createElement('style');
            style.textContent = ` + "`" + `
                @keyframes slideIn {
                    from { transform: translateX(100%); opacity: 0; }
                    to { transform: translateX(0); opacity: 1; }
                }
            ` + "`" + `;
            document.head.appendChild(style);
            document.body.appendChild(notification);
        }

        updateFooterVersion();
        // Check for version changes every 10 seconds
        setInterval(updateFooterVersion, 10000);
    </script>

    <!-- Footer -->
    <footer class="footer">
        <div class="footer-version">
            VPN Mesh Network <span id="footer-version">v0.0.0</span>
        </div>
        <div class="footer-status">
            <span id="footer-status-dot" class="footer-status-dot"></span>
            <span>Node: <span id="footer-node">...</span></span>
        </div>
    </footer>
</body>
</html>`
}
