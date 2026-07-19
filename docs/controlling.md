# Controlling and querying praxisd via praxctl

praxctl is a command line utility that can be used to both control and query
praxisd via [RPC](http://www.wikipedia.org/wiki/Remote_procedure_call).
praxisd does **not** enable its RPC server by default; You must configure at
minimum both an RPC username and password or both an RPC limited username and
password:

* praxisd.conf configuration file

```bash
[Application Options]
rpcuser=myuser
rpcpass=SomeDecentp4ssw0rd
rpclimituser=mylimituser
rpclimitpass=Limitedp4ssw0rd
```

* praxctl.conf configuration file

```bash
[Application Options]
rpcuser=myuser
rpcpass=SomeDecentp4ssw0rd
```

OR

```bash
[Application Options]
rpclimituser=mylimituser
rpclimitpass=Limitedp4ssw0rd
```

For a list of available options, run: `$ praxctl --help`
