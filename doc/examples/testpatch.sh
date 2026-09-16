kubectl exec -it cluster-example-1 -c postgres -- psql   "dbname=spiffedb host=cluster-example-rw user=spiffeuser sslmode=verify-full \
   sslrootcert=/controller/certificates/server-ca.crt \
   sslkey=/spiffe-certs/svid_key.pem sslcert=/spiffe-certs/svid.pem"