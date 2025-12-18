# vsTaskViewer

**Language**: English | [Deutsch](README.md)

vsTaskViewer is a Go application that starts predefined commands as background tasks and displays their output (stdout/stderr) live via a web interface.

> **Note**: This code was created with the support of LLM/AI tools.

![Code Coverage](https://osi8.de/coverage-40.2_percent.svg)

## Features

- **Task Management**: Starts predefined tasks as background processes
- **Parameterized Tasks**: Tasks can be configured with typed parameters (int/string)
- **Web Interface**: Minimalist HTML interface for live display of task output
- **WebSocket Support**: Live streaming of stdout and stderr via WebSocket
- **JWT Authentication**: All requests must be authenticated with a valid JWT token
- **Max Execution Time**: Automatic termination of tasks after configurable time (SIGTERM → SIGKILL)
- **Rate Limiting**: Protection against brute-force and DoS attacks
- **Request Size Limits**: Protection against oversized requests
- **Optional TLS/HTTPS**: Support for encrypted connections
- **Health Check**: `/health` endpoint for monitoring
- **Single Binary**: Creates a single Linux amd64 binary

## Installation

### Build

```bash
make build
```

Or manually:

```bash
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o vsTaskViewer
```

### Configuration

The configuration file is searched in the following order:

1. Path specified with `-c` flag
2. `vsTaskViewer.toml` in the same directory as the binary
3. `/etc/vsTaskViewer/vsTaskViewer.toml`

The templates directory (HTML files) is searched in the following order:

1. Path specified with `-t` flag
2. `html/` in the same directory as the binary
3. `/etc/vsTaskViewer/html/`

The task output directory is searched in the following order:

1. Path specified with `-d` flag
2. `task_dir` from the configuration file
3. `/var/vsTaskViewer` (default)

The execution user (exec user) is searched in the following order:

1. User specified with `-u` flag
2. `exec_user` from the configuration file
3. `www-data` (default, UID 33)

**Example Installation:**

```bash
# System-wide installation
sudo mkdir -p /etc/vsTaskViewer/html
sudo cp example-config.toml /etc/vsTaskViewer/vsTaskViewer.toml
sudo cp -r html/* /etc/vsTaskViewer/html/
sudo nano /etc/vsTaskViewer/vsTaskViewer.toml

# Install binary
sudo cp vsTaskViewer /usr/local/bin/
sudo chmod +x /usr/local/bin/vsTaskViewer

# Create task directory
sudo mkdir -p /var/vsTaskViewer
sudo chown www-data:www-data /var/vsTaskViewer
sudo chmod 700 /var/vsTaskViewer
```

**Important**: Change the `auth.secret` in the configuration!

### Systemd Service Installation

A systemd service file is included in the repository (`vsTaskViewer.service`):

```bash
# Install service file
sudo cp vsTaskViewer.service /etc/systemd/system/

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable vsTaskViewer
sudo systemctl start vsTaskViewer

# Check status
sudo systemctl status vsTaskViewer

# View logs
sudo journalctl -u vsTaskViewer -f
```

**Security Notes for systemd Service:**

- The service starts as `root` and automatically reduces privileges to `www-data`
- Strict security settings are enabled (ProtectSystem, NoNewPrivileges, etc.)
- The service requires `CAP_NET_BIND_SERVICE` for ports < 1024 and `CAP_CHOWN` for directory creation
- Only `/var/vsTaskViewer` is writable, all other paths are read-only
- PrivateTmp prevents access to temporary files of other processes

## Configuration

The configuration file `/etc/vsTaskViewer.toml` has the following structure:

```toml
[server]
port = 8080
# Path to HTML directory (must exist)
html_dir = "./html"
# Path to task output directory (default: /var/vsTaskViewer)
# Must be owned by the execution user and have permissions 700
# task_dir = "/var/vsTaskViewer"
# User to execute as (default: www-data)
# Must exist and is set after loading TLS files
# exec_user = "www-data"
# Rate Limiting: Requests per minute per IP (0 = disabled)
rate_limit_rpm = 60
# Maximum request size in bytes (0 = default 10MB)
max_request_size = 10485760
# TLS configuration (optional, leave empty to disable HTTPS)
# tls_key_file = "/etc/ssl/private/key.pem"
# tls_cert_file = "/etc/ssl/certs/fullchain.pem"
# Allowed origins for WebSocket (empty = allow all)
# allowed_origins = ["http://localhost:8080"]

[auth]
secret = "your-secret-key"

[[tasks]]
name = "task-name"
description = "Task description"
command = "command to execute"
# Maximum execution time in seconds (0 = no limit)
# If exceeded, SIGTERM is sent, then SIGKILL after 30 seconds
max_execution_time = 300

# Tasks can be parameterized
# Parameters are substituted in the command with {{param_name}}
[[tasks]]
name = "parameterized-task"
description = "Task with parameters"
command = "echo 'Processing {{filename}} with timeout {{timeout}}'"
max_execution_time = 300
# Parameter definitions
[[tasks.parameters]]
name = "filename"
type = "string"  # "int" or "string"
optional = false  # true = optional, false = required

[[tasks.parameters]]
name = "timeout"
type = "int"
optional = true
```

### HTML Directory

The `html_dir` directory must contain the following files:

- `viewer.html` - Main viewer page (with template placeholders `{{.TaskID}}` and `{{.WebSocketURL}}`)
- `400.html` - Bad Request error page
- `401.html` - Unauthorized error page
- `404.html` - Not Found error page
- `405.html` - Method Not Allowed error page
- `500.html` - Internal Server Error error page

All HTML files contain inline CSS and JavaScript.

## Usage

### Start Server

```bash
./vsTaskViewer
```

**Available Options:**

```bash
# Show help
./vsTaskViewer -h

# With specific config file
./vsTaskViewer -c /path/to/config.toml

# With specific templates directory
./vsTaskViewer -t /path/to/html

# With specific task output directory
./vsTaskViewer -d /var/vsTaskViewer

# With specific execution user
./vsTaskViewer -u www-data

# With specific port
./vsTaskViewer -p 9090

# Combined
./vsTaskViewer -c /path/to/config.toml -t /path/to/html -d /var/vsTaskViewer -u www-data -p 9090
```

### Start Task

**1. Generate JWT Token**

Create a JWT token with HS256. **Important**: API tokens must include a `body_sha1` claim that matches the SHA1 hash of the normalized JSON request body.

```go
// Example in Go
import (
    "crypto/sha1"
    "encoding/hex"
    "encoding/json"
    "github.com/golang-jwt/jwt/v5"
    "time"
)

// Request body for task start
requestBody := map[string]interface{}{
    "task_name": "example-task",
    // optional: "parameters": map[string]interface{}{...}
}

// Normalize JSON (removes whitespace differences)
bodyJSON, _ := json.Marshal(requestBody)

// Calculate SHA1 hash of normalized body
hash := sha1.New()
hash.Write(bodyJSON)
bodySHA1 := hex.EncodeToString(hash.Sum(nil))

// Create JWT token with body_sha1 claim
claims := jwt.MapClaims{
    "body_sha1": bodySHA1,  // Required for API tokens
    "exp": time.Now().Add(24 * time.Hour).Unix(),
}
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
tokenString, _ := token.SignedString([]byte("your-secret-key"))
```

**Note**: The `body_sha1` claim binds the token to the specific request body and prevents tampering. The JSON body is normalized before hashing, so formatting differences (whitespace, line breaks) do not affect the hash.

**2. Start Task via API**

**Important**: The JWT token must include a `body_sha1` claim that matches the SHA1 hash of the normalized JSON request body. See above for an example of token generation.

Task without parameters:
```bash
# First: Generate token with body_sha1 for '{"task_name": "example-task"}'
# Then:
curl -X POST http://localhost:8080/api/start?token=YOUR_JWT_TOKEN \
  -H "Content-Type: application/json" \
  -d '{"task_name": "example-task"}'
```

Task with parameters:
```bash
# First: Generate token with body_sha1 for the request body
# Then:
curl -X POST http://localhost:8080/api/start?token=YOUR_JWT_TOKEN \
  -H "Content-Type: application/json" \
  -d '{
    "task_name": "parameterized-task",
    "parameters": {
      "filename": "data.txt",
      "timeout": 30
    }
  }'
```

Response:
```json
{
  "task_id": "uuid-here",
  "viewer_url": "http://localhost:8080/viewer?task_id=uuid-here&token=viewer-token"
}
```

**3. Open Viewer**

Open the `viewer_url` from the response in your browser. The page shows the stdout and stderr output of the task live.

## API Endpoints

### POST /api/start

Starts a task.

**Query Parameters:**

- `token`: JWT token (HS256) with `body_sha1` claim

**Request Body:**
```json
{
  "task_name": "task-name",
  "parameters": {
    "param1": "value1",
    "param2": 42
  }
}
```

**Request Body Fields:**

- `task_name` (required): Name of the task from the configuration
- `parameters` (optional): Map of parameter names to values
- String parameters: `"param": "value"`
- Integer parameters: `"param": 42` or `"param": "42"`

**Token Requirements:**

The JWT token must include a `body_sha1` claim that matches the SHA1 hash of the normalized JSON request body. The server validates that the hash in the token matches the actually sent request body.

**Response:**
```json
{
  "task_id": "uuid",
  "viewer_url": "http://..."
}
```

**Errors:**

- `400 Bad Request`: Invalid parameters, missing required parameters, invalid characters, invalid JSON format
- `401 Unauthorized`: Invalid or missing JWT token, token audience mismatch, request body hash does not match token
- `500 Internal Server Error`: Task could not be started

### GET /viewer

Displays the HTML viewer page.

**Query Parameters:**

- `task_id`: Task ID (UUID)
- `token`: JWT token for viewer access

### WebSocket /ws

WebSocket endpoint for live output.

**Query Parameters:**

- `task_id`: Task ID (UUID)
- `token`: JWT token

**Messages:**
```json
{
  "type": "stdout",
  "data": "output line\n"
}
```

or

```json
{
  "type": "stderr",
  "data": "error line\n"
}
```

### GET /health

Health check endpoint for monitoring (no authentication required).

**Response:**
```
OK
```

Status Code: `200 OK`

## JWT Token

All requests must include a JWT token in the URL query parameter `token`.

**Claims:**

- `task_id` (optional): Task identifier
- `body_sha1` (required for API tokens): SHA1 hash of the normalized JSON request body (hex-encoded)
- `exp`: Expiration time (Unix Timestamp)
- `aud` (Audience): Token type to prevent token reuse
  - **API Tokens**: No `aud` claim or empty `aud` claim
  - **Viewer Tokens**: `aud="viewer"` - can only be used for viewer/WebSocket endpoints

**Signature:**

- Algorithm: HS256
- Secret: From configuration (`auth.secret`)

**Body Hashing for API Tokens:**

API tokens must include a `body_sha1` claim that matches the SHA1 hash of the normalized JSON request body. This provides the following security benefits:

- **Integrity Protection**: Prevents manipulation of the request body after token generation
- **Token Binding**: Binds the token to a specific request
- **Normalization**: The JSON body is normalized before hashing (whitespace, line breaks, and key order are ignored), so formatting differences do not affect the hash

**Example for Body Hash Calculation:**

1. Create the JSON request body (e.g., `{"task_name": "example-task"}`)
2. Normalize the JSON (parse and re-encode in compact form)
3. Calculate the SHA1 hash of the normalized JSON
4. Encode the hash as a hex string
5. Add the hash as `body_sha1` claim to the JWT token

**Security:**

- Viewer tokens have `aud="viewer"` and **cannot** be used for API requests
- API tokens have no `aud` claim and **cannot** be used for viewer/WebSocket endpoints
- API tokens must include a `body_sha1` claim that matches the request body
- This prevents viewer tokens from being misused for new API requests and protects against request body manipulation

## Task Output

Tasks are executed so that their output is stored in a configurable directory (default: `/var/vsTaskViewer/[task-id]/`):

- `[task-dir]/[task-id]/stdout`: Standard output
- `[task-dir]/[task-id]/stderr`: Error output
- `[task-dir]/[task-id]/pid`: Process ID of the running task
- `[task-dir]/[task-id]/exitcode`: Exit code after termination
- `[task-dir]/[task-id]/run.sh`: Wrapper script (created automatically)

The WebSocket endpoint continuously reads these files and sends new lines to the client.

**Security:**
- Directories have permissions `0700` (owner-only access) for additional security
- On startup, the task output directory is validated:
  - The directory must exist or be creatable with the execution user's permissions
  - The directory must be owned by the execution user (UID/GID)
  - The directory must have permissions `700`
  - On errors, the application terminates with an error message

## Task Timeouts

Each task can define a maximum execution time (`max_execution_time`) in seconds:

- `0` = No timeout (task runs indefinitely)
- `> 0` = Maximum execution time in seconds

**Timeout Behavior:**

1. When the maximum execution time is exceeded:
   - `SIGTERM` is sent to the process (graceful shutdown)
   - A system message is sent via WebSocket
   
2. After 30 seconds:
   - If the process is still running, `SIGKILL` is sent (force kill)
   - Another system message is sent via WebSocket

**Example:**
```toml
[[tasks]]
name = "limited-task"
command = "long-running-script.sh"
max_execution_time = 300  # 5 minutes
```

System messages in WebSocket:
```json
{
  "type": "timeout",
  "data": "Process exceeded maximum execution time. Sending SIGTERM (graceful shutdown)...",
  "pid": 12345
}
```

## Task Parameterization

Tasks can be configured with typed parameters that are substituted in the command.

### Parameter Definition

Parameters are defined in the task configuration:

```toml
[[tasks.parameters]]
name = "param_name"
type = "int"      # or "string"
optional = false  # true = optional, false = required
```

### Parameter Types

- **int**: Only digits 0-9 allowed
- **string**: Only the following characters allowed: `-a-zA-Z0-9_:,.` (hyphen, letters, digits, underscore, colon, comma, period)

### Parameter Substitution

Parameters are substituted in the command with the syntax `{{param_name}}`:

```toml
command = "echo 'Processing {{filename}} with timeout {{timeout}}'"
```

### Validation

- **Required Parameters**: If required parameters are missing, the request is rejected with `400 Bad Request`
- **Type Validation**: Parameters must match the defined type
- **Character Validation**: Invalid characters result in `400 Bad Request` with corresponding error message
- **Unknown Parameters**: Undefined parameters are rejected
- **Security**: Strict validation prevents command injection through parameters

### Examples

**Task with required string parameter:**
```toml
[[tasks]]
name = "process-file"
command = "cat {{filepath}}"
[[tasks.parameters]]
name = "filepath"
type = "string"
optional = false
```

**Task with optional parameters:**
```toml
[[tasks]]
name = "custom-task"
command = "echo '{{message}}' && sleep {{duration}}"
[[tasks.parameters]]
name = "message"
type = "string"
optional = true
[[tasks.parameters]]
name = "duration"
type = "int"
optional = true
```

**API Call:**
```bash
curl -X POST http://localhost:8080/api/start?token=TOKEN \
  -H "Content-Type: application/json" \
  -d '{
    "task_name": "process-file",
    "parameters": {
      "filepath": "/path/to/file.txt"
    }
  }'
```

### Error Handling for Parameters

For invalid parameters, a `400 Bad Request` is returned with a descriptive error message:

**Missing required parameters:**
```json
{
  "error": "parameter validation failed: required parameter 'filename' (type string) is missing"
}
```

**Invalid characters in int parameter:**
```json
{
  "error": "parameter validation failed: parameter 'timeout' (type int) contains invalid characters. Only digits 0-9 are allowed, got: 30abc"
}
```

**Invalid characters in string parameter:**
```json
{
  "error": "parameter validation failed: parameter 'filename' (type string) contains invalid characters. Only [-a-zA-Z0-9_:,.] are allowed, got: /path/to/file"
}
```

**Unknown parameters:**
```json
{
  "error": "parameter validation failed: unknown parameter 'unknown_param' provided (not defined in task configuration)"
}
```

**Wrong type:**
```json
{
  "error": "parameter validation failed: parameter 'timeout' must be of type 'int', got string"
}
```

## Security

- **JWT Authentication**: All endpoints (except `/health`) require valid JWT tokens
- **Body Hashing**: API tokens must include a `body_sha1` claim that matches the request body - prevents request body manipulation
- **Predefined Tasks**: Only tasks defined in the configuration can be started
- **Token Validation**: Expiration, signature, and audience are checked
- **Parameter Validation**: Strict type and character validation prevents command injection
- **Rate Limiting**: Protection against brute-force and DoS attacks
- **Request Size Limits**: Protection against oversized requests (default: 10MB)
- **Command Escaping**: Commands are safely escaped to prevent injection
- **Privilege Dropping**: The application runs as `www-data` (UID 33) by default after startup
- **TLS Files**: TLS keys and certificates are loaded before dropping privileges

### Privilege Dropping and Startup Order

The application follows a specific startup order for maximum security:

1. **Load Configuration**: Configuration file is loaded and paths are resolved
2. **Load TLS Files**: If TLS is configured, key and certificate files are loaded into memory **before** dropping privileges (may require elevated rights)
3. **Reduce Privileges**: The application switches to the configured execution user (`exec_user`, default: `www-data`)
4. **Validation**: Directories are validated as the execution user
5. **Start Server**: HTTP/HTTPS server is started

**Important for Production:**

- Start the application as `root` if TLS is used and TLS files require elevated rights
- The application automatically reduces privileges after loading TLS files
- If the application is already running as the target user (not root), no privilege dropping is performed

**Important for Production:**

- Use a strong, random secret in the configuration
- Use HTTPS in production
- Restrict access to the API (firewall, reverse proxy)
- Review all task definitions and parameter validations
- Use optional parameters only when necessary
- Configure a firewall to restrict access to the port
- Use a reverse proxy (e.g., nginx) for additional security
- Regularly monitor logs for suspicious activity
- Limit the number of concurrent tasks in the configuration
- Use rate limiting (enabled in configuration)

## Dependencies

- `github.com/BurntSushi/toml` - TOML configuration
- `github.com/golang-jwt/jwt/v5` - JWT token
- `github.com/google/uuid` - UUID generation
- `github.com/gorilla/websocket` - WebSocket support

## License

See LICENSE file.

