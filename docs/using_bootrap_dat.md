### Table of Contents
1. [What is bootstrap.dat?](#What)<br />
2. [What are the pros and cons of using bootstrap.dat?](#ProsCons)
3. [Where do I get bootstrap.dat?](#Obtaining)
4. [How do I know I can trust the bootstrap.dat I downloaded?](#Trust)
5. [How do I use bootstrap.dat with btcd?](#Importing)

<a name="What" />
### 1. What is bootstrap.dat?

It is a flat, binary file containing bitcoin blockchain data starting from the
genesis block and continuing through a relatively recent block height depending
on the last time it was updated.

See [this](https://bitcointalk.org/index.php?topic=145386.0) thread on
bitcointalk for more details.

**NOTE:** Using bootstrap.dat is entirely optional.  Btcd will download the
block chain from other peers through the Bitcoin protocol with no extra
configuration needed.

<a name="ProsCons" />
### 2. What are the pros and cons of using bootstrap.dat?

Pros:
- Typically accelerates the initial process of bringing up a new node as it
  downloads from public P2P nodes and generally is able to achieve faster
  download speeds
- It is particularly beneficial when bringing up multiple nodes as you only need
  to download the data once

Cons:
- Requires you to setup and configure a torrent client if you don't already have
  one available
- Requires roughly twice as much disk space since you'll need the flat file as
  well as the imported database

<a name="Obtaining" />
### 3. Where do I get bootstrap.dat?

The bootstrap.dat file is made available via a torrent.  See
[this](https://bitcointalk.org/index.php?topic=145386.0) thread on bitcointalk
for the torrent download details.

<a name="Trust" />
### 4. How do I know I can trust the bootstrap.dat I downloaded?

You don't need to trust the file as the `addblock` utility verifies every block
using the same rules that are used when downloading the block chain normally
through the Bitcoin protocol.  Additionally, the chain rules contain hard-coded
checkpoints for the known-good block chain at periodic intervals.  This ensures
that not only is it a valid chain, but it is the same chain that everyone else
is using.

<a name="Importing" />
### 5. How do I use bootstrap.dat with btcd?

btcd comes with a separate utility named `addblock` which can be used to import
`bootstrap.dat`.  This approach is used since the import is a one-time operation
and we prefer to keep the daemon itself as lightweight as possible.

1. Stop btcd if it is already running.  This is required since addblock needs to
   access the database used by btcd and it will be locked if btcd is using it.
2. Note the path to the downloaded bootstrap.dat file.
3. Run the addblock utility with the `-i` argument pointing to the location of
   boostrap.dat:<br /><br />
**Windows:**
```bat
C:\> "%PROGRAMFILES%\Btcd Suite\Btcd\addblock" -i C:\Path\To\bootstrap.dat
```
**Linux/Unix/BSD/POSIX:**
```bash
$ $GOPATH/bin/addblock -i /path/to/bootstrap.dat
```
