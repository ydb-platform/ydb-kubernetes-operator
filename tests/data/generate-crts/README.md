## Certificates for testing

`ca.crt` and `ca.key` are just a self-signed CA created like this:


```
openssl genrsa -out ca.key 2048

openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 -out ca.crt -subj "/CN=test-root-ca"
```

Then use `generate-test-certs.sh` to generate storage and database certs, which then are used in tests.
