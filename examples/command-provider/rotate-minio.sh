#!/bin/sh
# Example command provider: rotate a MinIO service-account secret key.
#
# Wired up with:
#
#   secrets:
#     - name: minio_secret_key
#       type: command
#       env_key: MINIO_SECRET_KEY
#       env_file: .env
#       command:
#         apply:  ["./rotate-minio.sh", "apply"]
#         verify: ["./rotate-minio.sh", "verify"]
#         pass_env: [MINIO_ALIAS]
#       max_age: 90d
#
# Everything arrives in the environment, never in the arguments: the process
# table is world-readable, so a credential passed as an argument would be
# visible to every user on the host.
#
#   ROTATOR_OPERATION         apply | verify | restore
#   ROTATOR_SECRET_NAME       the secret's configured name
#   ROTATOR_NEW_SECRET        the value to set (apply) or check (verify)
#   ROTATOR_PREVIOUS_SECRET   the value to put back (restore only)
#   ROTATOR_CURRENT_SECRET    what the service accepts now (restore only)
#   ROTATOR_HOST, ROTATOR_PORT, ROTATOR_USERNAME, ROTATOR_TARGET_USER
#
# rotator's own environment is NOT inherited, so the master passphrase cannot
# leak into this script. Anything else it needs must be listed in pass_env.
#
# Exit 0 for success, non-zero for failure. Output is captured and quoted back
# in the error, so do not echo the secret: it would end up in the audit trail.

set -eu

ALIAS="${MINIO_ALIAS:-local}"
ACCOUNT="${ROTATOR_TARGET_USER:?no target user}"

case "${ROTATOR_OPERATION}" in
  apply)
    # Set the new secret key on the service account.
    mc admin user svcacct edit \
      --secret-key "${ROTATOR_NEW_SECRET}" \
      "${ALIAS}" "${ACCOUNT}"
    ;;

  verify)
    # Prove the new key works by using it. A temporary alias keeps this
    # independent of the one configured above.
    mc alias set rotator-verify \
      "http://${ROTATOR_HOST}:${ROTATOR_PORT:-9000}" \
      "${ACCOUNT}" "${ROTATOR_NEW_SECRET}" >/dev/null
    trap 'mc alias remove rotator-verify >/dev/null 2>&1 || true' EXIT
    mc ls rotator-verify >/dev/null
    ;;

  restore)
    # No `restore:` is configured above, so rotator re-runs `apply` with the
    # previous value and this branch is never reached. It is here to show the
    # shape for a service where applying is not idempotent.
    mc admin user svcacct edit \
      --secret-key "${ROTATOR_PREVIOUS_SECRET}" \
      "${ALIAS}" "${ACCOUNT}"
    ;;

  *)
    echo "unknown operation: ${ROTATOR_OPERATION}" >&2
    exit 64
    ;;
esac
