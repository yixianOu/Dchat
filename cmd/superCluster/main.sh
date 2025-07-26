
#!/bin/sh

cat <<- EOF > sys.conf
accounts: {
  \$SYS: {
    users: [{user: sys, password: sys}]
  }
}
EOF

cat <<- EOF > east.conf
port: 4222
http_port: 8222
server_name: n1
include sys.conf
gateway: {
  name: east,
  port: 7222,
  gateways: [
    {name: "east", urls: ["nats://0.0.0.0:7222"]},
    {name: "west", urls: ["nats://0.0.0.0:7223"]},
  ]
}
EOF

cat <<- EOF > west.conf
port: 4223
http_port: 8223
server_name: n2
include sys.conf
gateway: {
  name: west,
  port: 7223,
  gateways: [
    {name: "east", urls: ["nats://0.0.0.0:7222"]},
    {name: "west", urls: ["nats://0.0.0.0:7223"]},
  ]
}
EOF

nats-server -c east.conf > /dev/null 2>&1 &
nats-server -c west.conf > /dev/null 2>&1 &
sleep 3

curl --fail --silent \
  --retry 5 \
  --retry-delay 1 \
  http://localhost:8222/healthz > /dev/null
curl --fail --silent \
  --retry 5 \
  --retry-delay 1 \
  http://localhost:8223/healthz > /dev/null

nats context save east \
  --server "nats://localhost:4222"
nats context save east-sys \
  --server "nats://localhost:4222" \
  --user sys \
  --password sys
nats context save west \
  --server "nats://localhost:4223"

nats --context east-sys server list
nats --context east reply 'greet' 'hello from east' &
sleep 1
nats --context west request 'greet' ''

