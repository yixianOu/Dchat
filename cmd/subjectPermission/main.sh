

#!/bin/sh
NATS_URL="nats://localhost:4222"

cat <<- EOF > server.conf
accounts: {
  APP: {
    users: [
      {
        user: greeter,
        password: greeter,
        permissions: {
          sub: {
            allow: ["services.greet"]
          },
          allow_responses: true
        }
      },
      {
        user: joe,
        password: joe,
        permissions: {
          pub: {
            allow: ["joe.>", "services.*"]
          },
          sub: {
            allow: ["_INBOX.>"]
          },
        }
      },
      {
        user: pam,
        password: pam,
        permissions: {
          pub: {
            allow: ["pam.>", "services.*"]
          },
          sub: {
            allow: ["_INBOX.>"]
          },
        }
      },
    ]
  }
}
EOF

nats-server -c server.conf 2> /dev/null &
SERVER_PID=$!

nats context save greeter \
  --user greeter --password greeter


nats context save joe \
  --user joe --password joe


nats context save pam \
  --user pam --password pam

nats --context greeter \
  reply 'services.greet' \
  'Reply {{ID}}' &


GREETER_PID=$!

sleep 0.5

nats --context joe request 'services.greet' ''
nats --context pam request 'services.greet' ''

nats --context pam sub '_INBOX.>' &
INBOX_SUB_PID=$!

nats --context joe request 'services.greet' ''

nats --context joe --inbox-prefix _INBOX_joe request 'services.greet' ''

cat <<- EOF > server.conf
accounts: {
  APP: {
    users: [
      {
        user: greeter,
        password: greeter,
        permissions: {
          sub: {
            allow: ["services.greet"]
          },
          allow_responses: true
        }
      },
      {
        user: joe,
        password: joe,
        permissions: {
          pub: {
            allow: ["joe.>", "services.*"]
          },
          sub: {
            allow: ["_INBOX_joe.>"]
          },
        }
      },
      {
        user: pam,
        password: pam,
        permissions: {
          pub: {
            allow: ["pam.>", "services.*"]
          },
          sub: {
            allow: ["_INBOX_pam.>"]
          },
        }
      },
    ]
  }
}
EOF

echo 'Reloading the server with new config...'
nats-server --signal reload=$SERVER_PID

kill $INBOX_SUB_PID
nats --context pam sub '_INBOX.>'
nats --context pam sub '_INBOX_joe.>'

nats --context joe --inbox-prefix _INBOX_joe request 'services.greet' ''
nats --context pam --inbox-prefix _INBOX_pam request 'services.greet' ''

kill $GREETER_PID
kill $SERVER_PID

