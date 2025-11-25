# vsTaskViewer

vsTaskViewer ist eine Go-Anwendung, die vordefinierte Tasks über den Linux `at`-Befehl als Hintergrund-Tasks startet und deren Ausgabe (stdout/stderr) live über einen Web-Interface anzeigt.

## Features

- **Task-Management**: Startet vordefinierte Tasks über den `at`-Befehl
- **Web-Interface**: Minimalistisches HTML-Interface zur Live-Anzeige der Task-Ausgabe
- **WebSocket-Support**: Live-Streaming von stdout und stderr über WebSocket
- **JWT-Authentifizierung**: Alle Requests müssen mit einem gültigen JWT-Token authentifiziert werden
- **Single Binary**: Erstellt ein einzelnes Linux amd64 Binary

## Installation

### Build

```bash
make build
```

Oder manuell:

```bash
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o vsTaskViewer
```

### Konfiguration

Kopieren Sie die Beispiel-Konfiguration nach `/etc/vsTaskViewer.toml`:

```bash
sudo cp example-config.toml /etc/vsTaskViewer.toml
sudo nano /etc/vsTaskViewer.toml
```

**Wichtig**: Ändern Sie das `auth.secret` in der Konfiguration!

## Konfiguration

Die Konfigurationsdatei `/etc/vsTaskViewer.toml` hat folgende Struktur:

```toml
[server]
port = 8080
# Pfad zum HTML-Verzeichnis (muss existieren)
html_dir = "./html"
# Rate Limiting: Requests pro Minute pro IP (0 = deaktiviert)
rate_limit_rpm = 60
# Maximale Request-Größe in Bytes (0 = Standard 10MB)
max_request_size = 10485760
# TLS-Konfiguration (optional, leer lassen um HTTPS zu deaktivieren)
# tls_key_file = "/etc/ssl/private/key.pem"
# tls_cert_file = "/etc/ssl/certs/fullchain.pem"
# Erlaubte Origins für WebSocket (leer = alle erlauben)
# allowed_origins = ["http://localhost:8080"]

[auth]
secret = "your-secret-key"

[[tasks]]
name = "task-name"
description = "Task description"
command = "command to execute"
```

### HTML-Verzeichnis

Das `html_dir` Verzeichnis muss folgende Dateien enthalten:
- `viewer.html` - Haupt-Viewer-Seite (mit Template-Platzhaltern `{{.TaskID}}` und `{{.WebSocketURL}}`)
- `400.html` - Bad Request Fehlerseite
- `401.html` - Unauthorized Fehlerseite
- `404.html` - Not Found Fehlerseite
- `405.html` - Method Not Allowed Fehlerseite
- `500.html` - Internal Server Error Fehlerseite

Alle HTML-Dateien enthalten inline CSS und JavaScript.

## Verwendung

### Server starten

```bash
./vsTaskViewer
```

Oder mit angegebenem Port:

```bash
./vsTaskViewer -port 8080
```

### Task starten

**1. JWT-Token generieren**

Erstellen Sie ein JWT-Token mit HS256:

```go
// Beispiel in Go
import "github.com/golang-jwt/jwt/v5"

claims := jwt.MapClaims{
    "task_id": "optional-task-id",
    "exp": time.Now().Add(24 * time.Hour).Unix(),
}
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
tokenString, _ := token.SignedString([]byte("your-secret-key"))
```

**2. Task über API starten**

```bash
curl -X POST http://localhost:8080/api/start?token=YOUR_JWT_TOKEN \
  -H "Content-Type: application/json" \
  -d '{"task_name": "example-task"}'
```

Antwort:
```json
{
  "task_id": "uuid-here",
  "viewer_url": "http://localhost:8080/viewer?task_id=uuid-here&token=viewer-token"
}
```

**3. Viewer öffnen**

Öffnen Sie die `viewer_url` aus der Antwort im Browser. Die Seite zeigt live die stdout und stderr Ausgabe des Tasks.

## API-Endpunkte

### POST /api/start

Startet einen Task.

**Query Parameter:**
- `token`: JWT-Token (HS256)

**Request Body:**
```json
{
  "task_name": "task-name"
}
```

**Response:**
```json
{
  "task_id": "uuid",
  "viewer_url": "http://..."
}
```

### GET /viewer

Zeigt die HTML-Viewer-Seite.

**Query Parameter:**
- `task_id`: Task-ID (UUID)
- `token`: JWT-Token für Viewer-Zugriff

### WebSocket /ws

WebSocket-Endpunkt für Live-Output.

**Query Parameter:**
- `task_id`: Task-ID (UUID)
- `token`: JWT-Token

**Nachrichten:**
```json
{
  "type": "stdout",
  "data": "output line\n"
}
```

oder

```json
{
  "type": "stderr",
  "data": "error line\n"
}
```

## JWT-Token

Alle Requests müssen ein JWT-Token im URL-Query-Parameter `token` enthalten.

**Claims:**
- `task_id` (optional): Task-Kennung
- `exp`: Ablaufzeit (Unix Timestamp)

**Signatur:**
- Algorithmus: HS256
- Secret: Aus der Konfiguration (`auth.secret`)

## Task-Ausgabe

Tasks werden so ausgeführt, dass ihre Ausgabe in `/tmp/[task-id]/` gespeichert wird:
- `/tmp/[task-id]/stdout`: Standard-Ausgabe
- `/tmp/[task-id]/stderr`: Fehler-Ausgabe

Der WebSocket-Endpunkt liest diese Dateien kontinuierlich und sendet neue Zeilen an den Client.

## Sicherheit

- **JWT-Authentifizierung**: Alle Endpunkte erfordern gültige JWT-Tokens
- **Vordefinierte Tasks**: Nur in der Konfiguration definierte Tasks können gestartet werden
- **Token-Validierung**: Expiration und Signatur werden geprüft

**Wichtig für Produktion:**
- Verwenden Sie ein starkes, zufälliges Secret in der Konfiguration
- Verwenden Sie HTTPS in Produktion
- Beschränken Sie den Zugriff auf die API

## Abhängigkeiten

- `github.com/BurntSushi/toml` - TOML-Konfiguration
- `github.com/golang-jwt/jwt/v5` - JWT-Token
- `github.com/google/uuid` - UUID-Generierung
- `github.com/gorilla/websocket` - WebSocket-Support

## Lizenz

Siehe LICENSE-Datei.
