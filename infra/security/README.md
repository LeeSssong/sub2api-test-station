# Chromium screenshot sandbox

Based on https://github.com/moby/profiles/blob/main/seccomp/default.json fetched 2026-10-13.
The final rule allows clone, unshare, and setns for Chromium sandbox namespaces.
All other default seccomp restrictions remain; no privileged mode, SYS_ADMIN capability, or --no-sandbox flag.
Apply only to sub2api-blue, sub2api-green and sub2api-worker with security_opt seccomp=/opt/sub2api/production/security/chromium-seccomp.json.
Install from pushed main; back up protected Compose and replace atomically without printing its contents. Existing containers remain until normal blue/green and drained worker replacement.
Native linux/amd64 non-root screenshot tested with network disabled: PNG dimensions and pixel assertion pass.
