# Installing engram-deployer on Unraid (FastRaid)

## One-time setup

1. **Install plugin via Unraid UI**

   Unraid web UI → **Plugins** → **Install Plugin** → paste the raw URL of `engram-deployer.plg` from the GitHub release.

   The plugin is a Slackware `.txz`. Unraid runs `upgradepkg --install-new`, which:
   - Extracts the package to `/` with root:root ownership
   - Installs `/usr/local/sbin/engram-deployer` + `/etc/rc.d/rc.engram-deployer`
   - Drops `engram-deployer.env.sample` into `/boot/config/plugins/engram-deployer/`
   - Runs `install/doinst.sh`: generates a self-signed ed25519 TLS cert (`cert.pem` + `key.pem`, **valid 10 years**) and starts the daemon if `engram-deployer.env` already exists (otherwise leaves it stopped for config)

2. **Capture the cert SHA-256** — printed by the install hook. Pin this in CI:

   ```bash
   ssh root@fastraid 'openssl x509 -in /boot/config/plugins/engram-deployer/cert.pem -noout -fingerprint -sha256'
   ```

3. **Create the env file**

   ```bash
   cd /boot/config/plugins/engram-deployer
   cp engram-deployer.env.sample engram-deployer.env
   nano engram-deployer.env
   ```

   Verify all `DEPLOYER_*` values match your repo / workflow / runner IP.

4. **Start the daemon**

   ```bash
   /etc/rc.d/rc.engram-deployer start
   /etc/rc.d/rc.engram-deployer status
   tail -f /var/log/engram-deployer.log
   ```

5. **Verify externally** (from any LAN host or the runner VM)

   ```bash
   curl --cacert <(ssh root@fastraid 'cat /boot/config/plugins/engram-deployer/cert.pem') \
        https://10.0.20.214:8443/healthz
   # → ok
   ```

## Adding the firewall rule (SlowRaid)

The runner VM lives behind `/boot/config/iptables.runner.sh` on SlowRaid. Add to the FORWARD allowlist for `:8443`:

```bash
# /boot/config/iptables.runner.sh, FORWARD section:
iptables -A FORWARD -s 10.20.99.10 -d 10.0.20.214 -p tcp --dport 8443 -j ACCEPT
```

Reload: `bash /boot/config/iptables.runner.sh`.

## Upgrades

Push a new `v*.*.*` tag. The release workflow builds the `.txz` and materializes the `.plg` with the version + MD5 baked in. Unraid UI shows "update available"; click → `upgradepkg --install-new` swaps in the new package, `doinst.sh` restarts the daemon. Cert + env preserved across upgrades.

## Uninstall

Plugins → engram-deployer → **Remove**. The remove hook calls `removepkg` (clean Slackware uninstall — every file from the install manifest, no more, no less). Cert and env stay in `/boot/config/plugins/engram-deployer/` so a re-install picks them up.

## Troubleshooting

| Symptom | Check |
|---|---|
| Daemon won't start | `tail /var/log/engram-deployer.log` |
| `config: missing required env: ...` | Edit `engram-deployer.env`, restart |
| TLS handshake fails from runner | Cert SHA in CI doesn't match installed cert. Recapture + redeploy CI workflow. |
| 401 on /deploy | Check `DEPLOYER_REPOSITORY`, `DEPLOYER_REF`, `DEPLOYER_WORKFLOW_REF` match the calling workflow's OIDC claims. |
| 403 on /deploy | `DEPLOYER_ALLOWED_IPS` doesn't include the runner's IP, OR firewall blocks. |
| Replay error after success | JTI cache holds 1000 entries × 30min — this is correct behavior on retry; mint a fresh token via `actions/core@v1.10+` `getIDToken`. |
