# Security Review and Hardening

This document describes the security measures implemented in the vsTaskViewer Debian package.

## Systemd Service Security Hardening

### Privilege Management
- **Starts as root**: Required to read TLS certificates and HTML files from `/etc`
- **Drops privileges**: Application drops to `www-data` user after loading sensitive files
- **NoNewPrivileges=false**: Required to allow privilege dropping via `setuid()`/`setgid()`
- **RestrictSUIDSGID=false**: Required to allow privilege dropping syscalls

### Filesystem Protection
- **ProtectSystem=full**: Makes `/usr`, `/boot`, `/etc` read-only (prevents writes)
- **ProtectHome=true**: Makes home directories inaccessible
- **ReadWritePaths**: Only `/var/vsTaskViewer` is writable
- **ReadOnlyPaths**: `/etc/vsTaskViewer`, `/etc/ssl`, `/etc/letsencrypt` are readable
- **PrivateTmp=true**: Uses private `/tmp` directory

### Capability Management
- **CapabilityBoundingSet**: Limits capabilities to:
  - `CAP_NET_BIND_SERVICE`: Bind to ports < 1024
  - `CAP_CHOWN`: Create task directories with proper ownership
  - `CAP_SETUID`/`CAP_SETGID`: Drop privileges (only during startup)
- **Note**: After privilege drop, capabilities become ineffective as process is no longer root

### Process Isolation
- **RestrictNamespaces=true**: Prevents creating new namespaces
- **RestrictRealtime=true**: Prevents realtime scheduling
- **MemoryDenyWriteExecute=true**: Prevents W^X memory pages
- **LockPersonality=true**: Prevents changing execution domain
- **ProtectKernelTunables=true**: Prevents modifying kernel parameters
- **ProtectKernelModules=true**: Prevents loading kernel modules
- **ProtectControlGroups=true**: Prevents modifying cgroups
- **RestrictAddressFamilies**: Only IPv4 and IPv6 allowed

### Resource Limits
- **LimitNOFILE=65536**: Maximum open file descriptors
- **LimitNPROC=4096**: Maximum processes

## File Permissions

### Installed Files
- **Binary** (`/usr/bin/vsTaskViewer`): `755` (executable by all, standard for system binaries)
- **Config** (`/etc/vsTaskViewer/vsTaskViewer.toml`): `600` (readable/writable by root only)
- **HTML files**: `644` (readable by all, writable by root)
- **HTML directory**: `755` (readable/executable by all)
- **Task directory** (`/var/vsTaskViewer`): `700` (accessible only by www-data)

## Post-Installation Security

### User Creation
- Creates `www-data` user if it doesn't exist (system user, no login shell)
- Verifies user exists before setting directory ownership

### Directory Setup
- Task directory created with `700` permissions (www-data only)
- Config directory created with proper permissions
- HTML directory created with `755` permissions

## Known Security Considerations

### 1. Privilege Dropping Window
- **Risk**: Small window where process runs as root before dropping privileges
- **Mitigation**: 
  - Minimal code execution as root
  - Only loads necessary files (TLS, HTML)
  - Immediately drops privileges after loading

### 2. Capabilities After Privilege Drop
- **Risk**: Capabilities remain in bounding set even after dropping to www-data
- **Mitigation**: Capabilities are ineffective once process is no longer root (UID != 0)
- **Note**: This is a systemd limitation - capabilities are set at service start

### 3. Config File Security
- **Risk**: Config file contains JWT secret
- **Mitigation**: 
  - Permissions set to `600` (root only)
  - User must change default secret after installation
  - Warning displayed during installation

## Recommendations

### Production Deployment
1. Ensure config file has strong JWT secret
2. Use TLS/HTTPS in production
3. Regularly review and rotate JWT secrets
4. Monitor logs for suspicious activity
5. Review application logs regularly via journalctl

## Application Security

For detailed security review of the Go application code, see:
- `/usr/share/doc/vstaskviewer/APPLICATION_SECURITY_REVIEW.md` (after installation)
- `debian/APPLICATION_SECURITY_REVIEW.md` (in source package)

## Security Updates

When security issues are discovered:
1. Update the package version in `debian/changelog`
2. Document the issue and fix in this file
3. Release updated package promptly

