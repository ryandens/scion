# GitHub App Credential Rotation Runbook

## Private Key Rotation

GitHub Apps support multiple active private keys, enabling zero-downtime rotation.

### Procedure

1. **Generate a new key on GitHub**:
   - Go to GitHub App settings > General > Private keys
   - Click **Generate a private key**
   - Download and securely store the new `.pem` file

2. **Update the Hub configuration**:
   ```bash
   # Copy the new key to the Hub server
   scp new-key.pem hub-server:/etc/scion/github-app-key-new.pem
   chmod 600 /etc/scion/github-app-key-new.pem

   # Update settings.yaml to point to the new key
   # private_key_path: /etc/scion/github-app-key-new.pem
   ```

3. **Restart/reload the Hub server**:
   ```bash
   systemctl restart scion-hub
   ```

4. **Verify token minting works**:
   ```bash
   # Check the app config is valid
   curl https://<hub-url>/api/v1/github-app \
     -H "Authorization: Bearer <admin-token>"

   # Trigger a permission sync (uses the new key to call GitHub API)
   curl -X POST https://<hub-url>/api/v1/github-app/sync-permissions \
     -H "Authorization: Bearer <admin-token>"
   ```

5. **Delete the old key on GitHub**:
   - Go to GitHub App settings > General > Private keys
   - Click **Delete** next to the old key

6. **Clean up the old key file**:
   ```bash
   rm /etc/scion/github-app-key-old.pem
   ```

### Rollback

If the new key doesn't work, revert `private_key_path` in settings.yaml to the old key and restart. Both keys remain valid on GitHub until explicitly deleted.

---

## Webhook Secret Rotation

GitHub does **not** support multiple webhook secrets simultaneously. There will be a brief window where incoming webhooks may fail signature validation.

### Procedure

1. **Update the webhook secret on GitHub**:
   - Go to GitHub App settings > General > Webhook
   - Enter a new secret value
   - Click **Save changes**

2. **Immediately update the Hub configuration**:
   ```yaml
   # settings.yaml
   github_app:
     webhook_secret: "new-secret-value"
   ```

3. **Restart the Hub server**:
   ```bash
   systemctl restart scion-hub
   ```

### Impact Window

- **Duration**: Seconds (between GitHub update and Hub restart)
- **Effect**: Webhook events during this window will fail signature validation and be dropped
- **Safety net**: The periodic health check (every 6 or 24 hours) will detect any state changes missed during the window
- **Manual recovery**: Run discovery to catch any missed installation changes:
  ```bash
  curl -X POST https://<hub-url>/api/v1/github-app/installations/discover \
    -H "Authorization: Bearer <admin-token>"
  ```

### Minimizing Impact

1. Choose a low-activity time for rotation
2. Pre-stage the Hub config change (have the new secret ready in the file)
3. Update GitHub, then immediately restart the Hub
4. After restart, check GitHub's webhook delivery log for any failed deliveries and manually reconcile if needed
