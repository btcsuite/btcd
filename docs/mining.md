# Mining

lbcd supports the `getblocktemplate` RPC.
The limited user cannot access this RPC.

## Add the payment addresses with the `miningaddr` option

```bash
[Application Options]
rpcuser=myuser
rpcpass=SomeDecentp4ssw0rd
miningaddr=12c6DSiU4Rq3P4ZxziKxzrL5LmMBrzjrJX
miningaddr=1M83ju3EChKYyysmM2FXtLNftbacagd8FR
```

## Add lbcd's RPC TLS certificate to system Certificate Authority list

Various miners use [curl](http://curl.haxx.se/) to fetch data from the RPC server.
Since curl validates the certificate by default, we must install the `lbcd` RPC
certificate into the default system Certificate Authority list.

## Ubuntu

1. Copy rpc.cert to /usr/share/ca-certificates: `# cp /home/user/.lbcd/rpc.cert /usr/share/ca-certificates/lbcd.crt`
2. Add lbcd.crt to /etc/ca-certificates.conf: `# echo lbcd.crt >> /etc/ca-certificates.conf`
3. Update the CA certificate list: `# update-ca-certificates`

## Set your mining software url to use https

`cgminer -o https://127.0.0.1:9245 -u rpcuser -p rpcpassword`

Alternatively, you can disable TLS with the `--notls` option for the server.
