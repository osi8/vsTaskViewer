# Security Audit Report - vsTaskViewer

## Identified Security Issues

### Critical Issues - FIXED ✅

1. **Command Injection Risk** (task.go:74) ✅ FIXED
   - Task commands are inserted directly into bash script without proper escaping
   - Even though only predefined tasks can be executed, malicious config could inject commands
   - **Fix**: Commands are now properly escaped using `escapeBashCommand()`
   - **Status**: Fixed

2. **WebSocket CORS** (websocket.go:24) ✅ FIXED
   - `CheckOrigin` allows all origins (`return true`)
   - Allows cross-origin WebSocket connections from any domain
   - **Fix**: Added configurable `allowed_origins` in config (empty = allow all for internal networks)
   - **Status**: Fixed (configurable)

### High Priority Issues

3. **JWT Token in URL** ⚠️ ACCEPTABLE FOR INTERNAL USE
   - Tokens passed in query parameters are visible in:
     - Browser history
     - Server logs
     - Referrer headers
   - **Status**: Acceptable for internal networks, consider Authorization header for production

4. **No Rate Limiting** ✅ FIXED
   - No protection against brute force attacks
   - No protection against DoS
   - **Fix**: Added IP-based rate limiting with configurable requests per minute
   - **Status**: Fixed

5. **Unbounded JSON Decoding** (api.go:44) ✅ FIXED
   - JSON decoder has no size limit
   - Vulnerable to memory exhaustion attacks
   - **Fix**: Added `decodeJSONRequest()` with 1MB limit
   - **Status**: Fixed

### Medium Priority Issues

6. **Information Disclosure** ⚠️ ACCEPTABLE FOR INTERNAL USE
   - Error messages may leak internal information
   - Stack traces in error responses
   - **Status**: Acceptable for internal networks, sanitize for production

7. **File Permissions** (task.go:81) ✅ FIXED
   - Scripts created with 0755 (executable by all)
   - Should be more restrictive (0700)
   - **Fix**: Changed to 0700 (owner only)
   - **Status**: Fixed

8. **No Input Validation** ✅ FIXED
   - Task names not validated (length, characters)
   - Task IDs should be validated as UUIDs
   - **Fix**: Added `validateTaskName()` and `validateTaskID()`
   - **Status**: Fixed

### Low Priority Issues

9. **No HTTPS Enforcement** ✅ FIXED
   - Application doesn't enforce HTTPS
   - Tokens transmitted in plain text over HTTP
   - **Fix**: Added optional TLS support via config (tls_key_file, tls_cert_file)
   - **Status**: Fixed (optional, configurable)

10. **No Request Size Limits** ✅ FIXED
    - HTTP request body size not limited
    - **Fix**: Added configurable max_request_size (default 10MB)
    - **Status**: Fixed

## Security Fixes Applied

### ✅ Fixed Issues

1. **Command Injection Protection**
   - Commands are now escaped using `escapeBashCommand()`
   - Prevents injection even if config is compromised

2. **Input Validation**
   - Task names validated: alphanumeric, underscore, hyphen only
   - Task names limited to 100 characters
   - Task IDs validated as UUIDs

3. **JSON Request Size Limit**
   - Maximum 1MB per JSON request
   - Prevents memory exhaustion attacks

4. **File Permissions**
   - Scripts: 0700 (owner only)
   - Directories: 0700 (owner only)

5. **WebSocket CORS**
   - Configurable allowed origins
   - Empty list = allow all (for internal networks)

6. **Rate Limiting**
   - IP-based token bucket rate limiter
   - Configurable requests per minute per IP
   - Automatic cleanup of old buckets

7. **TLS/HTTPS Support**
   - Optional TLS support via config
   - Requires tls_key_file and tls_cert_file
   - Automatically uses HTTPS if configured

8. **Request Size Limits**
   - Configurable max request body size
   - Default: 10MB
   - Max header size: 1MB
   - Timeouts: Read/Write 15s, Idle 60s

## Recommendations

1. **For Internal Networks**: Current security level is acceptable
2. **For Production**: 
   - Enable HTTPS
   - Configure `allowed_origins` in config
   - Consider rate limiting
   - Consider moving JWT tokens to Authorization header
   - Sanitize error messages
3. **Best Practices**:
   - Use strong JWT secrets
   - Regularly rotate secrets
   - Monitor logs for suspicious activity
   - Keep Go dependencies updated

