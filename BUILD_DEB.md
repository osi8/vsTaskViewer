# Building the .deb Package

This document describes how to build a Debian package for vsTaskViewer.

## Prerequisites

Install the required build tools:

```bash
sudo apt-get update
sudo apt-get install build-essential devscripts debhelper golang-go
```

## Building the Package

### Using Make

Simply run:

```bash
make deb
```

This will create a `.deb` file in the parent directory (e.g., `../vstaskviewer_1.0.0-1_amd64.deb`).

### Manual Build

Alternatively, you can build manually:

```bash
dpkg-buildpackage -b -us -uc
```

The `-b` flag builds a binary package only (no source package), `-us` and `-uc` skip signing.

## Package Contents

The `.deb` package includes:

- **Binary**: `/usr/bin/vsTaskViewer` (statically linked, no runtime dependencies)
- **Systemd Service**: `/etc/systemd/system/vsTaskViewer.service`
- **Sample Config**: `/etc/vsTaskViewer/vsTaskViewer.toml.example`
- **HTML Templates**: `/etc/vsTaskViewer/html/*.html`
- **Documentation**: `/usr/share/doc/vstaskviewer/README.md`

## Installation

After building, install the package:

```bash
sudo dpkg -i ../vstaskviewer_1.0.0-1_amd64.deb
```

If there are missing dependencies (unlikely for a statically linked binary), fix them with:

```bash
sudo apt-get install -f
```

## Post-Installation

After installation:

1. Edit the configuration file:
   ```bash
   sudo nano /etc/vsTaskViewer/vsTaskViewer.toml
   ```
   **Important**: Change the `auth.secret` value!

2. Start the service:
   ```bash
   sudo systemctl start vsTaskViewer
   sudo systemctl status vsTaskViewer
   ```

3. Enable auto-start on boot:
   ```bash
   sudo systemctl enable vsTaskViewer
   ```

## Package Structure

The Debian package structure:

```
debian/
├── changelog          # Package version history
├── compat             # debhelper compatibility level
├── control            # Package metadata and dependencies
├── copyright          # Copyright information
├── postinst           # Post-installation script
├── postrm             # Post-removal script
├── prerm              # Pre-removal script
├── rules              # Build rules (Makefile)
└── vsTaskViewer.service  # Systemd service file (uses /usr/bin)
```

## Debugging Service Issues

If the service fails to start, use the debug helper script:

```bash
sudo vsTaskViewer-debug
```

This script will:
- Check if all required files exist
- Verify permissions and ownership
- Show recent journal logs
- Test the binary directly

### Manual Debugging

1. **View service status:**
   ```bash
   sudo systemctl status vsTaskViewer.service -l
   ```

2. **View recent logs:**
   ```bash
   sudo journalctl -u vsTaskViewer.service -n 50 --no-pager
   ```

3. **Follow logs in real-time:**
   ```bash
   sudo journalctl -u vsTaskViewer.service -f
   ```

4. **Test binary directly (as root):**
   ```bash
   sudo /usr/bin/vsTaskViewer \
       -c /etc/vsTaskViewer/vsTaskViewer.toml \
       -t /etc/vsTaskViewer/html \
       -d /var/vsTaskViewer \
       -u www-data
   ```

5. **Check systemd service file:**
   ```bash
   sudo systemctl cat vsTaskViewer.service
   ```

## Notes

- The binary is statically linked and has **no runtime library dependencies**
- The package automatically creates `/var/vsTaskViewer` with proper permissions (www-data:www-data, 700)
- The systemd service is automatically enabled but not started (user must configure first)
- The sample config is installed as `.example` - the postinst script copies it to the actual config if it doesn't exist
- Logs are sent to both journal and console for easier debugging

