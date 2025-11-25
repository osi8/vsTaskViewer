package main

import (
	"fmt"
	"net/http"
)

// handleViewer serves the HTML viewer page
func handleViewer(w http.ResponseWriter, r *http.Request, config *Config) {
	// Authenticate request
	claims, err := validateJWT(r, config.Auth.Secret)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unauthorized: %v", err), http.StatusUnauthorized)
		return
	}

	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		taskID = claims.TaskID
	}

	if taskID == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}

	// Get token from query
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}

	// Build WebSocket URL
	scheme := "ws"
	if r.TLS != nil {
		scheme = "wss"
	}
	wsURL := fmt.Sprintf("%s://%s/ws?task_id=%s&token=%s", scheme, r.Host, taskID, token)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="de">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Task Viewer - %s</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            background: #1e1e1e;
            color: #d4d4d4;
            padding: 20px;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        h1 {
            color: #4ec9b0;
            margin-bottom: 10px;
        }
        .info {
            color: #858585;
            margin-bottom: 20px;
            font-size: 14px;
        }
        .status {
            padding: 10px;
            margin-bottom: 20px;
            border-radius: 4px;
            background: #252526;
            border-left: 3px solid #007acc;
        }
        .status.connected {
            border-left-color: #4ec9b0;
        }
        .status.disconnected {
            border-left-color: #f48771;
        }
        .output-container {
            background: #252526;
            border-radius: 4px;
            padding: 15px;
            margin-bottom: 10px;
        }
        .output-label {
            color: #858585;
            font-size: 12px;
            margin-bottom: 8px;
            text-transform: uppercase;
        }
        .output {
            background: #1e1e1e;
            border: 1px solid #3e3e42;
            border-radius: 4px;
            padding: 15px;
            font-size: 13px;
            line-height: 1.6;
            max-height: 400px;
            overflow-y: auto;
            white-space: pre-wrap;
            word-wrap: break-word;
        }
        .output::-webkit-scrollbar {
            width: 10px;
        }
        .output::-webkit-scrollbar-track {
            background: #1e1e1e;
        }
        .output::-webkit-scrollbar-thumb {
            background: #424242;
            border-radius: 5px;
        }
        .output::-webkit-scrollbar-thumb:hover {
            background: #4e4e4e;
        }
        .stdout {
            color: #d4d4d4;
        }
        .stderr {
            color: #f48771;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Task Viewer</h1>
        <div class="info">Task ID: %s</div>
        <div id="status" class="status disconnected">Disconnected</div>
        
        <div class="output-container">
            <div class="output-label">STDOUT</div>
            <div id="stdout" class="output stdout"></div>
        </div>
        
        <div class="output-container">
            <div class="output-label">STDERR</div>
            <div id="stderr" class="output stderr"></div>
        </div>
    </div>

    <script>
        const taskId = '%s';
        const wsUrl = '%s';
        const stdoutEl = document.getElementById('stdout');
        const stderrEl = document.getElementById('stderr');
        const statusEl = document.getElementById('status');

        let ws = null;
        let reconnectAttempts = 0;
        const maxReconnectAttempts = 5;

        function connect() {
            try {
                ws = new WebSocket(wsUrl);

                ws.onopen = function() {
                    statusEl.textContent = 'Connected';
                    statusEl.className = 'status connected';
                    reconnectAttempts = 0;
                };

                ws.onmessage = function(event) {
                    try {
                        const data = JSON.parse(event.data);
                        if (data.type === 'stdout') {
                            stdoutEl.textContent += data.data;
                            stdoutEl.scrollTop = stdoutEl.scrollHeight;
                        } else if (data.type === 'stderr') {
                            stderrEl.textContent += data.data;
                            stderrEl.scrollTop = stderrEl.scrollHeight;
                        }
                    } catch (e) {
                        console.error('Failed to parse message:', e);
                    }
                };

                ws.onerror = function(error) {
                    console.error('WebSocket error:', error);
                    statusEl.textContent = 'Connection Error';
                    statusEl.className = 'status disconnected';
                };

                ws.onclose = function() {
                    statusEl.textContent = 'Disconnected';
                    statusEl.className = 'status disconnected';
                    
                    if (reconnectAttempts < maxReconnectAttempts) {
                        reconnectAttempts++;
                        setTimeout(connect, 2000 * reconnectAttempts);
                    } else {
                        statusEl.textContent = 'Disconnected - Max reconnect attempts reached';
                    }
                };
            } catch (e) {
                console.error('Failed to connect:', e);
                statusEl.textContent = 'Connection Failed';
                statusEl.className = 'status disconnected';
            }
        }

        connect();
    </script>
</body>
</html>`, taskID, taskID, taskID, wsURL)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

