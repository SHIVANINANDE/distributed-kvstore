# Email Notifications Disabled

This file documents the removal of email notifications from the KVStore monitoring stack.

## Changes Made

### 1. Alertmanager Configuration
- **File**: `monitoring/alertmanager/alertmanager.yml`
- **Changes**: 
  - Commented out all SMTP configuration in global section
  - Disabled all `email_configs` in receivers
  - Only Slack notifications remain active (if configured)

### 2. Removed Email Configurations
- All `smtp_*` settings commented out
- All `email_configs` sections disabled
- Critical, high-priority, and security alert emails disabled

## If You Need to Re-enable Emails

To re-enable email notifications in the future:

1. **Uncomment SMTP configuration in `monitoring/alertmanager/alertmanager.yml`**:
   ```yaml
   global:
     smtp_smarthost: 'smtp.gmail.com:587'
     smtp_from: 'your-alerts@example.com'
     smtp_auth_username: 'your-alerts@example.com'
     smtp_auth_password: '${SMTP_PASSWORD}'
     smtp_require_tls: true
   ```

2. **Uncomment email_configs in receivers**:
   ```yaml
   email_configs:
     - to: 'your-team@example.com'
       subject: 'Alert Subject'
       body: 'Alert body content'
   ```

3. **Set environment variables**:
   ```bash
   export SMTP_PASSWORD="your-app-password"
   ```

## Current Status
✅ **All email notifications are DISABLED**
✅ **Only Slack notifications remain (if webhooks configured)**
✅ **No emails will be sent for any alerts or errors**

---
*Last updated: August 18, 2025*
