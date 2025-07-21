#!/bin/sh
set -eu pipefail

NATS_MAIN_URL="nats://0.0.0.0:4222"
NATS_LEAF_URL="nats://0.0.0.0:4223"

# nsc add operator --generate-signing-key --sys --name local

nsc edit operator --require-signing-keys \
  --account-jwt-server-url "$NATS_MAIN_URL"

# nsc add account APP
nsc edit account APP --sk generate
# nsc add user --account APP user

nsc env

nats context save main-user \
  --server "$NATS_MAIN_URL" \
  --nsc nsc://local/APP/user 


nats context save main-sys \
  --nsc nsc://local/SYS/sys


nats context save leaf-user \
  --server "$NATS_LEAF_URL"

nsc generate config --nats-resolver --sys-account SYS > resolver.conf

echo 'Creating the main server conf...'
cat <<- EOF > main.conf
port: 4222
leafnodes: {
  port: 7422
}


include resolver.conf
EOF

echo 'Creating the leaf node conf...'
cat <<- EOF > leaf.conf
port: 4223
leafnodes: {
  remotes: [
    {
      url: "nats-leaf://0.0.0.0:7422",
      credentials: "/root/.local/share/nats/nsc/keys/creds/local/APP/user.creds"
    }
  ]
}
EOF

nats-server -c main.conf 2> /dev/null &
MAIN_PID=$!


sleep 1

echo 'Pushing the account JWT...'
nsc push -a APP

nats-server -c leaf.conf 2> /dev/null &
LEAF_PID=$!


sleep 1

nats --context main-user reply 'greet' 'hello' &
SERVICE_PID=$!

sleep 1

nats --context leaf-user request 'greet' ''

kill $SERVICE_PID
kill $LEAF_PID
kill $MAIN_PID

