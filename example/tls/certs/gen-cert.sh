#!/usr/bin/env bash

echo "generating CA certs ==="
cfssl gencert -initca ca-csr.json | cfssljson -bare ca -

echo "generating etcd peer certs ==="
cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=peer peer.json | cfssljson -bare peer

echo "generating etcd server certs ==="
cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=server server.json | cfssljson -bare server

echo "generating etcd client certs ==="
cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=client etcd-client.json | cfssljson -bare etcd-client

mv etcd-client.pem etcd-client.crt
mv etcd-client-key.pem etcd-client.key
cp ca.pem etcd-client-ca.crt

mv server.pem server.crt
mv server-key.pem server.key
cp ca.pem server-ca.crt

mv peer.pem peer.crt
mv peer-key.pem peer.key
mv ca.pem peer-ca.crt

rm *.csr ca-key.pem
