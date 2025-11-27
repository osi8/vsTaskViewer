# Application Security Review - vsTaskViewer Go Application

This document provides a comprehensive security review of the vsTaskViewer Go application code.

## Overall Security Posture: GOOD

The application demonstrates good security practices with proper input validation, authentication, and privilege management.

## Security Strengths

### 1. Authentication & Authorization
- **JWT-based authentication**: All endpoints require valid JWT tokens
- **Audience separation**: API tokens and viewer tokens use different audiences
- **Token expiration**: Tokens have expiration times
- **Secret validation**: Config requires auth.secret to be set

### 2. Input Validation
- **Task name validation**: Regex pattern `^[a-zA-Z0-9_-]+$` with length limits
- **Task ID validation**: Must be valid UUID format
- **Parameter validation**: 
  - Type checking (int/string)
  - Regex validation for parameter values
  - Required vs optional parameter handling
- **JSON size limits**: 1MB max for JSON requests
- **Request size limits**: Configurable max request size (default 10MB)

### 3. Command Injection Prevention
- **Bash command escaping**: `escapeBashCommand()` properly escapes single quotes
- **Parameter substitution**: Uses safe placeholder replacement
- **No direct shell execution**: Commands are wrapped in scripts with proper escaping

### 4. Privilege Management
- **Privilege dropping**: Drops from root to www-data after loading sensitive files
- **Verification**: Verifies privilege drop succeeded
- **Early file loading**: TLS and HTML files loaded before dropping privileges

### 5. Rate Limiting
- **Per-IP rate limiting**: Token bucket algorithm
- **Configurable**: Can be disabled (0) or set per minute
- **Memory cleanup**: Old buckets are cleaned up periodically

### 6. Resource Management
- **Process isolation**: Tasks run in separate directories
- **File permissions**: Task directories created with 0700 permissions
- **Timeout handling**: Max execution time with SIGTERM → SIGKILL escalation
- **Cleanup**: Task directories cleaned up after completion

### 7. WebSocket Security
- **Origin checking**: Configurable CORS with allowed origins
- **Authentication**: JWT required for WebSocket connections
- **Connection management**: Proper connection lifecycle management

## Security Issues Identified

### 1. IP Address Spoofing (Rate Limiting) MEDIUM
**Location**: `ratelimit.go:57-88`
**Issue**: Rate limiter trusts `X-Forwarded-For` and `X-Real-IP` headers without validation
**Risk**: Attackers behind proxies can spoof IP addresses to bypass rate limits
**Recommendation**: 
- Only trust these headers if behind a trusted reverse proxy
- Add configuration option to enable/disable header trust
- Consider using the rightmost IP in X-Forwarded-For (last proxy)

**Current Code**:
```go
if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
    return xff  // Takes first IP, could be spoofed
}
```

**Suggested Fix**:
```go
// Only trust if configured and behind trusted proxy
if config.TrustProxyHeaders {
    // Take rightmost IP (last proxy)
    ips := strings.Split(xff, ",")
    return strings.TrimSpace(ips[len(ips)-1])
}
```

### 2. HTML Template Injection LOW
**Location**: `viewer.go:67-69`
**Issue**: Direct string replacement in HTML template without escaping
**Risk**: If taskID contains HTML/JavaScript, it could be injected
**Mitigation**: 
- TaskID is validated as UUID, so risk is minimal
- WebSocket URL is also constructed with taskID, but URL encoding helps

**Current Code**:
```go
html = strings.ReplaceAll(html, "{{.TaskID}}", taskID)
html = strings.ReplaceAll(html, "{{.WebSocketURL}}", wsURL)
```

**Recommendation**: Use proper HTML escaping (though UUID validation mitigates this)

### 3. Temporary File Security GOOD
**Location**: `main.go:290-312`
**Status**: Properly handled
- Files created with restrictive permissions (0600 for key, 0644 for cert)
- Files are cleaned up with `defer os.Remove()`
- Created in system temp directory (isolated per process due to PrivateTmp)

### 4. Path Traversal Prevention GOOD
**Location**: Multiple files
**Status**: Properly handled
- Uses `filepath.Join()` for path construction
- Task directories use UUIDs (no user input in paths)
- All paths validated before use

### 5. Memory Exhaustion GOOD
**Location**: Multiple files
**Status**: Properly handled
- JSON size limits (1MB)
- Request body size limits (configurable, default 10MB)
- File reading uses scanners (buffered)

### 6. Process Management GOOD
**Location**: `task.go`, `timeout.go`
**Status**: Properly handled
- Processes run as www-data (dropped privileges)
- Proper signal handling (SIGTERM → SIGKILL)
- Process isolation (separate directories)
- Zombie process prevention (Wait() in goroutine)

## Recommendations

### High Priority

1. **Fix IP Address Spoofing in Rate Limiting**
   - Add configuration for trusted proxy headers
   - Validate or ignore X-Forwarded-For in untrusted environments
   - Document proxy setup requirements

2. **Add Request ID Logging**
   - Add unique request IDs to all log messages
   - Helps with security incident investigation
   - Makes log correlation easier

### Medium Priority

3. **Add Security Headers**
   - Implement security headers in HTTP responses:
     - `X-Content-Type-Options: nosniff`
     - `X-Frame-Options: DENY`
     - `Content-Security-Policy` (if serving HTML)
   - Consider adding to viewer endpoint

4. **Enhance Error Messages**
   - Some error messages may leak information
   - Consider generic error messages for authentication failures
   - Log detailed errors server-side only

5. **Add Request Timeout**
   - HTTP server has timeouts, but consider per-request timeouts
   - WebSocket connections have timeouts (good)

### Low Priority

6. **HTML Template Escaping**
   - While UUID validation mitigates risk, consider HTML escaping
   - Use `html/template` package for safer template rendering

7. **Add Metrics/Monitoring**
   - Track authentication failures
   - Monitor rate limit hits
   - Track task execution times
   - Helps identify attacks

8. **Audit Logging**
   - Log all task starts with user context
   - Log authentication attempts (success/failure)
   - Consider structured logging (JSON)

## Code Quality Security Practices

### Good Practices Found
- Proper error handling (errors wrapped with context)
- Input validation at multiple layers
- Resource cleanup (defer statements)
- Thread-safe operations (mutexes where needed)
- Context cancellation for goroutines
- File permission checks
- Directory ownership validation

### Areas for Improvement
- Some error messages could be more generic
- Consider adding request ID tracking
- Add more comprehensive logging for security events

## Dependencies Security

### Current Dependencies
- `github.com/BurntSushi/toml` - TOML parser (stable, widely used)
- `github.com/golang-jwt/jwt/v5` - JWT library (maintained, secure)
- `github.com/google/uuid` - UUID generation (standard library quality)
- `github.com/gorilla/websocket` - WebSocket library (well-maintained)

### Recommendations
- Regularly update dependencies
- Monitor for security advisories
- Consider using `go mod verify` in CI/CD
- Use `go list -m -u all` to check for updates

## Testing Recommendations

1. **Security Testing**
   - Test command injection attempts
   - Test path traversal attempts
   - Test JWT token manipulation
   - Test rate limit bypass attempts
   - Test privilege escalation attempts

2. **Fuzzing**
   - Fuzz task names, parameters
   - Fuzz JWT tokens
   - Fuzz file paths

3. **Penetration Testing**
   - Test with authenticated and unauthenticated users
   - Test WebSocket connections
   - Test concurrent requests
   - Test resource exhaustion

## Conclusion

The application demonstrates **good security practices** overall. The main concern is the IP address spoofing vulnerability in rate limiting when behind untrusted proxies. All other identified issues are low risk or already mitigated.

**Security Rating: 8/10**

The application is production-ready with the recommended fixes, particularly addressing the rate limiting IP spoofing issue.

