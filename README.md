# vsTaskViewer

vsTaskViewer ist eine Go-Anwendung, die vordefinierte Commands als Hintergrund-Tasks startet und deren Ausgabe (stdout/stderr) live über einen Web-Interface anzeigt.

> **Hinweis**: Dieser Code wurde mit Unterstützung von LLM/AI-Tools erstellt.

![Code Coverage](https://osi8.de/coverage-40.2_percent.svg)

## Features

- **Task-Management**: Startet vordefinierte Tasks über den `at`-Befehl
- **Parametrisierte Tasks**: Tasks können mit typisierten Parametern (int/string) konfiguriert werden
- **Web-Interface**: Minimalistisches HTML-Interface zur Live-Anzeige der Task-Ausgabe
- **WebSocket-Support**: Live-Streaming von stdout und stderr über WebSocket
- **JWT-Authentifizierung**: Alle Requests müssen mit einem gültigen JWT-Token authentifiziert werden
- **Max Execution Time**: Automatische Beendigung von Tasks nach konfigurierbarer Zeit (SIGTERM → SIGKILL)
- **Rate Limiting**: Schutz vor Brute-Force und DoS-Angriffen
- **Request Size Limits**: Schutz vor zu großen Requests
- **Optional TLS/HTTPS**: Unterstützung für verschlüsselte Verbindungen
- **Health Check**: `/health` Endpunkt für Monitoring
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

Die Konfigurationsdatei wird in folgender Reihenfolge gesucht:

1. Pfad angegeben mit `-c` Flag
2. `vsTaskViewer.toml` im gleichen Verzeichnis wie die Binary
3. `/etc/vsTaskViewer/vsTaskViewer.toml`

Das Templates-Verzeichnis (HTML-Dateien) wird in folgender Reihenfolge gesucht:

1. Pfad angegeben mit `-t` Flag
2. `html/` im gleichen Verzeichnis wie die Binary
3. `/etc/vsTaskViewer/html/`

Das Task-Ausgabe-Verzeichnis wird in folgender Reihenfolge gesucht:

1. Pfad angegeben mit `-d` Flag
2. `task_dir` aus der Konfigurationsdatei
3. `/var/vsTaskViewer` (Standard)

Der Ausführungsbenutzer (exec user) wird in folgender Reihenfolge gesucht:

1. Benutzer angegeben mit `-u` Flag
2. `exec_user` aus der Konfigurationsdatei
3. `www-data` (Standard, UID 33)

**Beispiel-Installation:**

```bash
# Systemweite Installation
sudo mkdir -p /etc/vsTaskViewer/html
sudo cp example-config.toml /etc/vsTaskViewer/vsTaskViewer.toml
sudo cp -r html/* /etc/vsTaskViewer/html/
sudo nano /etc/vsTaskViewer/vsTaskViewer.toml

# Binary installieren
sudo cp vsTaskViewer /usr/local/bin/
sudo chmod +x /usr/local/bin/vsTaskViewer

# Task-Verzeichnis erstellen
sudo mkdir -p /var/vsTaskViewer
sudo chown www-data:www-data /var/vsTaskViewer
sudo chmod 700 /var/vsTaskViewer
```

**Wichtig**: Ändern Sie das `auth.secret` in der Konfiguration!

### Systemd Service Installation

Ein systemd Service-File ist im Repository enthalten (`vsTaskViewer.service`):

```bash
# Service-File installieren
sudo cp vsTaskViewer.service /etc/systemd/system/

# Service aktivieren und starten
sudo systemctl daemon-reload
sudo systemctl enable vsTaskViewer
sudo systemctl start vsTaskViewer

# Status prüfen
sudo systemctl status vsTaskViewer

# Logs anzeigen
sudo journalctl -u vsTaskViewer -f
```

**Sicherheitshinweise für den systemd Service:**

- Der Service startet als `root` und reduziert automatisch die Rechte auf `www-data`
- Strikte Sicherheitseinstellungen sind aktiviert (ProtectSystem, NoNewPrivileges, etc.)
- Der Service benötigt `CAP_NET_BIND_SERVICE` für Ports < 1024 und `CAP_CHOWN` für Verzeichnis-Erstellung
- Nur `/var/vsTaskViewer` ist schreibbar, alle anderen Pfade sind read-only
- PrivateTmp verhindert Zugriff auf temporäre Dateien anderer Prozesse

## Konfiguration

Die Konfigurationsdatei `/etc/vsTaskViewer.toml` hat folgende Struktur:

```toml
[server]
port = 8080
# Pfad zum HTML-Verzeichnis (muss existieren)
html_dir = "./html"
# Pfad zum Task-Ausgabe-Verzeichnis (Standard: /var/vsTaskViewer)
# Muss im Besitz des ausführenden Benutzers sein und Berechtigungen 700 haben
# task_dir = "/var/vsTaskViewer"
# Benutzer zum Ausführen (Standard: www-data)
# Muss existieren und wird nach dem Laden der TLS-Dateien gesetzt
# exec_user = "www-data"
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
# Maximum execution time in seconds (0 = no limit)
# If exceeded, SIGTERM is sent, then SIGKILL after 30 seconds
max_execution_time = 300

# Tasks können parametrisiert werden
# Parameter werden im Command mit {{param_name}} substituiert
[[tasks]]
name = "parameterized-task"
description = "Task mit Parametern"
command = "echo 'Processing {{filename}} with timeout {{timeout}}'"
max_execution_time = 300
# Parameter-Definitionen
[[tasks.parameters]]
name = "filename"
type = "string"  # "int" oder "string"
optional = false  # true = optional, false = erforderlich

[[tasks.parameters]]
name = "timeout"
type = "int"
optional = true
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

**Verfügbare Optionen:**

```bash
# Hilfe anzeigen
./vsTaskViewer -h

# Mit spezifischer Config-Datei
./vsTaskViewer -c /path/to/config.toml

# Mit spezifischem Templates-Verzeichnis
./vsTaskViewer -t /path/to/html

# Mit spezifischem Task-Ausgabe-Verzeichnis
./vsTaskViewer -d /var/vsTaskViewer

# Mit spezifischem Ausführungsbenutzer
./vsTaskViewer -u www-data

# Mit spezifischem Port
./vsTaskViewer -p 9090

# Kombiniert
./vsTaskViewer -c /path/to/config.toml -t /path/to/html -d /var/vsTaskViewer -u www-data -p 9090
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

Task ohne Parameter:
```bash
curl -X POST http://localhost:8080/api/start?token=YOUR_JWT_TOKEN \
  -H "Content-Type: application/json" \
  -d '{"task_name": "example-task"}'
```

Task mit Parametern:
```bash
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
  "task_name": "task-name",
  "parameters": {
    "param1": "value1",
    "param2": 42
  }
}
```


- `task_name` (erforderlich): Name des Tasks aus der Konfiguration
- `parameters` (optional): Map von Parameternamen zu Werten
- String-Parameter: `"param": "value"`
- Integer-Parameter: `"param": 42` oder `"param": "42"`

**Response:**
```json
{
  "task_id": "uuid",
  "viewer_url": "http://..."
}
```

**Fehler:**

- `400 Bad Request`: Ungültige Parameter, fehlende erforderliche Parameter, ungültige Zeichen
- `401 Unauthorized`: Ungültiges oder fehlendes JWT-Token
- `500 Internal Server Error`: Task konnte nicht gestartet werden

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

### GET /health

Health-Check-Endpunkt für Monitoring (keine Authentifizierung erforderlich).

**Response:**
```
OK
```

Status Code: `200 OK`

## JWT-Token

Alle Requests müssen ein JWT-Token im URL-Query-Parameter `token` enthalten.

**Claims:**

- `task_id` (optional): Task-Kennung
- `exp`: Ablaufzeit (Unix Timestamp)
- `aud` (Audience): Token-Typ zur Verhinderung von Token-Reuse
  - **API-Tokens**: Kein `aud` Claim oder leerer `aud` Claim
  - **Viewer-Tokens**: `aud="viewer"` - können nur für Viewer/WebSocket-Endpunkte verwendet werden

**Signatur:**

- Algorithmus: HS256
- Secret: Aus der Konfiguration (`auth.secret`)

**Sicherheit:**

- Viewer-Tokens haben `aud="viewer"` und können **nicht** für API-Requests verwendet werden
- API-Tokens haben kein `aud` Claim und können **nicht** für Viewer/WebSocket-Endpunkte verwendet werden
- Dies verhindert, dass Viewer-Tokens für neue API-Requests missbraucht werden

## Task-Ausgabe

Tasks werden so ausgeführt, dass ihre Ausgabe in einem konfigurierbaren Verzeichnis gespeichert wird (Standard: `/var/vsTaskViewer/[task-id]/`):

- `[task-dir]/[task-id]/stdout`: Standard-Ausgabe
- `[task-dir]/[task-id]/stderr`: Fehler-Ausgabe
- `[task-dir]/[task-id]/pid`: Prozess-ID des laufenden Tasks
- `[task-dir]/[task-id]/exitcode`: Exit-Code nach Beendigung
- `[task-dir]/[task-id]/run.sh`: Wrapper-Script (wird automatisch erstellt)

Der WebSocket-Endpunkt liest diese Dateien kontinuierlich und sendet neue Zeilen an den Client.

**Sicherheit:**
- Die Verzeichnisse haben Berechtigungen `0700` (nur Owner-Zugriff) für zusätzliche Sicherheit
- Beim Start wird das Task-Ausgabe-Verzeichnis validiert:
  - Das Verzeichnis muss existieren oder mit den Rechten des ausführenden Benutzers erstellt werden können
  - Das Verzeichnis muss im Besitz des ausführenden Benutzers sein (UID/GID)
  - Das Verzeichnis muss Berechtigungen `700` haben
  - Bei Fehlern wird die Anwendung mit einer Fehlermeldung beendet

## Task-Timeouts

Jeder Task kann eine maximale Ausführungszeit (`max_execution_time`) in Sekunden definieren:

- `0` = Kein Timeout (Task läuft unbegrenzt)
- `> 0` = Maximale Ausführungszeit in Sekunden

**Timeout-Verhalten:**

1. Wenn die maximale Ausführungszeit überschritten wird:
   - Es wird `SIGTERM` an den Prozess gesendet (graceful shutdown)
   - Eine Systemnachricht wird über WebSocket gesendet
   
2. Nach 30 Sekunden:
   - Wenn der Prozess noch läuft, wird `SIGKILL` gesendet (force kill)
   - Eine weitere Systemnachricht wird über WebSocket gesendet

**Beispiel:**
```toml
[[tasks]]
name = "limited-task"
command = "long-running-script.sh"
max_execution_time = 300  # 5 Minuten
```

Systemnachrichten im WebSocket:
```json
{
  "type": "timeout",
  "data": "Process exceeded maximum execution time. Sending SIGTERM (graceful shutdown)...",
  "pid": 12345
}
```

## Task-Parametrisierung

Tasks können mit typisierten Parametern konfiguriert werden, die im Command substituiert werden.

### Parameter-Definition

Parameter werden in der Task-Konfiguration definiert:

```toml
[[tasks.parameters]]
name = "param_name"
type = "int"      # oder "string"
optional = false  # true = optional, false = erforderlich
```

### Parameter-Typen

- **int**: Nur Ziffern 0-9 erlaubt
- **string**: Nur folgende Zeichen erlaubt: `-a-zA-Z0-9_:,.` (Bindestrich, Buchstaben, Ziffern, Unterstrich, Doppelpunkt, Komma, Punkt)

### Parameter-Substitution

Parameter werden im Command mit der Syntax `{{param_name}}` substituiert:

```toml
command = "echo 'Processing {{filename}} with timeout {{timeout}}'"
```

### Validierung

- **Erforderliche Parameter**: Fehlen erforderliche Parameter, wird der Request mit `400 Bad Request` abgelehnt
- **Typ-Validierung**: Parameter müssen dem definierten Typ entsprechen
- **Zeichen-Validierung**: Ungültige Zeichen führen zu `400 Bad Request` mit entsprechender Fehlermeldung
- **Unbekannte Parameter**: Nicht definierte Parameter werden abgelehnt
- **Sicherheit**: Die strikte Validierung verhindert Command-Injection durch Parameter

### Beispiele

**Task mit erforderlichem String-Parameter:**
```toml
[[tasks]]
name = "process-file"
command = "cat {{filepath}}"
[[tasks.parameters]]
name = "filepath"
type = "string"
optional = false
```

**Task mit optionalen Parametern:**
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

**API-Aufruf:**
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

### Fehlerbehandlung bei Parametern

Bei fehlerhaften Parametern wird ein `400 Bad Request` mit einer beschreibenden Fehlermeldung zurückgegeben:

**Fehlende erforderliche Parameter:**
```json
{
  "error": "parameter validation failed: required parameter 'filename' (type string) is missing"
}
```

**Ungültige Zeichen in int-Parameter:**
```json
{
  "error": "parameter validation failed: parameter 'timeout' (type int) contains invalid characters. Only digits 0-9 are allowed, got: 30abc"
}
```

**Ungültige Zeichen in string-Parameter:**
```json
{
  "error": "parameter validation failed: parameter 'filename' (type string) contains invalid characters. Only [-a-zA-Z0-9_:,.] are allowed, got: /path/to/file"
}
```

**Unbekannte Parameter:**
```json
{
  "error": "parameter validation failed: unknown parameter 'unknown_param' provided (not defined in task configuration)"
}
```

**Falscher Typ:**
```json
{
  "error": "parameter validation failed: parameter 'timeout' must be of type 'int', got string"
}
```

## Sicherheit

- **JWT-Authentifizierung**: Alle Endpunkte (außer `/health`) erfordern gültige JWT-Tokens
- **Vordefinierte Tasks**: Nur in der Konfiguration definierte Tasks können gestartet werden
- **Token-Validierung**: Expiration und Signatur werden geprüft
- **Parameter-Validierung**: Strikte Typ- und Zeichen-Validierung verhindert Command-Injection
- **Rate Limiting**: Schutz vor Brute-Force und DoS-Angriffen
- **Request Size Limits**: Schutz vor zu großen Requests (Standard: 10MB)
- **Command Escaping**: Commands werden sicher escaped, um Injection zu verhindern
- **Privilege Dropping**: Die Anwendung läuft standardmäßig als `www-data` (UID 33) nach dem Start
- **TLS-Dateien**: TLS-Schlüssel und Zertifikate werden vor dem Dropping der Rechte geladen

### Privilege Dropping und Startup-Reihenfolge

Die Anwendung folgt einer spezifischen Startup-Reihenfolge für maximale Sicherheit:

1. **Konfiguration laden**: Konfigurationsdatei wird geladen und Pfade werden aufgelöst
2. **TLS-Dateien laden**: Wenn TLS konfiguriert ist, werden die Schlüssel- und Zertifikatsdateien **vor** dem Dropping der Rechte in den Speicher geladen (benötigt möglicherweise erhöhte Rechte)
3. **Rechte reduzieren**: Die Anwendung wechselt zum konfigurierten Ausführungsbenutzer (`exec_user`, Standard: `www-data`)
4. **Validierung**: Verzeichnisse werden als Ausführungsbenutzer validiert
5. **Server starten**: HTTP/HTTPS-Server wird gestartet

**Wichtig für Produktion:**

- Starten Sie die Anwendung als `root`, wenn TLS verwendet wird und die TLS-Dateien erhöhte Rechte benötigen
- Die Anwendung reduziert automatisch die Rechte nach dem Laden der TLS-Dateien
- Wenn die Anwendung bereits als Zielbenutzer läuft (nicht root), wird kein Privilege Dropping durchgeführt

**Wichtig für Produktion:**

- Verwenden Sie ein starkes, zufälliges Secret in der Konfiguration
- Verwenden Sie HTTPS in Produktion
- Beschränken Sie den Zugriff auf die API (Firewall, Reverse Proxy)
- Überprüfen Sie alle Task-Definitionen und Parameter-Validierungen
- Verwenden Sie optionale Parameter nur wenn nötig
- Konfigurieren Sie eine Firewall, um den Zugriff auf den Port zu beschränken
- Verwenden Sie einen Reverse Proxy (z.B. nginx) für zusätzliche Sicherheit
- Überwachen Sie die Logs regelmäßig auf verdächtige Aktivitäten
- Begrenzen Sie die Anzahl der gleichzeitigen Tasks in der Konfiguration
- Verwenden Sie Rate Limiting (in der Konfiguration aktiviert)

## Abhängigkeiten

- `github.com/BurntSushi/toml` - TOML-Konfiguration
- `github.com/golang-jwt/jwt/v5` - JWT-Token
- `github.com/google/uuid` - UUID-Generierung
- `github.com/gorilla/websocket` - WebSocket-Support

## Lizenz

Siehe LICENSE-Datei.
