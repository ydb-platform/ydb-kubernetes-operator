#!/bin/bash

CA_KEY="ca.key"
CA_CERT="ca.crt"

# Output paths for the database and storage certificates and keys
DATABASE_KEY="../database.key"
DATABASE_CSR="database.csr"
DATABASE_CERT="../database.crt"

STORAGE_KEY="../storage.key"
STORAGE_CSR="storage.csr"
STORAGE_CERT="../storage.crt"

generate_certificate() {
  local KEY_PATH=$1
  local CSR_PATH=$2
  local CERT_PATH=$3
  local CONFIG_FILE=$4

  openssl req -new -newkey rsa:2048 -nodes -keyout "$KEY_PATH" -out "$CSR_PATH" -config "$CONFIG_FILE"
  openssl x509 -req -in "$CSR_PATH" -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial -out "$CERT_PATH" -days 5475 -sha256 -extensions req_ext -extfile "$CONFIG_FILE"
}

# Paths to .cnf files, where we will write certificate settings 
DATABASE_CONFIG="database-csr.cnf"
STORAGE_CONFIG="storage-csr.cnf"

cat > $DATABASE_CONFIG <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = req_ext
prompt = no

[req_distinguished_name]
O = test

[req_ext]
subjectAltName = @alt_names
extendedKeyUsage = serverAuth, clientAuth
basicConstraints = critical,CA:FALSE

[alt_names]
DNS.1 = database-grpc.ydb.svc.cluster.local
DNS.2 = *.database-interconnect.ydb.svc.cluster.local
EOF

cat > $STORAGE_CONFIG <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = req_ext
prompt = no

[req_distinguished_name]
O = test

[req_ext]
subjectAltName = @alt_names
extendedKeyUsage = serverAuth, clientAuth
basicConstraints = critical,CA:FALSE

[alt_names]
DNS.1 = storage-grpc.ydb.svc.cluster.local
DNS.2 = *.storage-interconnect.ydb.svc.cluster.local
EOF

generate_certificate "$DATABASE_KEY" "$DATABASE_CSR" "$DATABASE_CERT" "$DATABASE_CONFIG"

generate_certificate "$STORAGE_KEY" "$STORAGE_CSR" "$STORAGE_CERT" "$STORAGE_CONFIG"

# Clean up
rm $DATABASE_CSR $STORAGE_CSR $DATABASE_CONFIG $STORAGE_CONFIG

echo "Certificates generated:"
echo " - $DATABASE_CERT"
echo " - $STORAGE_CERT"
