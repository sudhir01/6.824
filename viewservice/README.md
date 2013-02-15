## [6.824][0] - Spring 2013

# 6.824 Lab 2: Primary/Backup Key/Value Service

### Part A Due: Feb 15, 5:00pm

### Part B Due: Feb 22, 5:00pm

---

### Introduction

There are situations that your lock service from Lab 1 does not handle
correctly. For example, if a client thinks the primary is dead (due to
a network failure or lost packets) and switches to the backup, but the
primary is actually alive, the primary will miss some of the client's
operations. Another problem with Lab 1 is that it does not cope with
recovering servers: if the primary crashes, but is later repaired, it
can't be re-integrated into the system, which means the system cannot
tolerate any further failures.

In this lab you'll fix the above problems using a more sophisticated
form of primary/backup replication. This time you'll build a key/value
storage service. The service supports two RPCs: Put(key, value), and
Get(key). The service maintains a simple database of key/value pairs. Put()
updates the value for a particular key in the database. Get() fetches
the current value for a key.

In order to ensure that all parties (clients and servers) agree on
which server is the primary, and which is the backup, we'll introduce
a kind of master server, called the viewservice. The viewservice
monitors whether each available server is alive or dead. If the
current primary or backup becomes dead, the viewservice selects a
server to replace it. A client checks with the viewservice to find the
current primary. The servers cooperate with the viewservice to ensure
that at most one primary is active at a time.

Your key/value service will allow replacement of failed servers. If
the primary fails, the viewservice will promote the backup to be
primary. If the backup fails, or is promoted, and there is an idle
server available, the viewservice will cause it to be the backup.
The primary will send its complete database to the new backup,
and then send subsequent Puts to the backup to ensure that the
backup's key/value database remains identical to the primary's.

It turns out the primary must send Gets as well as Puts to the backup
(if there is one), and must wait for the backup to reply before
responding to the client. This helps prevent two servers from acting
as primary (a "split brain"). An example: S1 is the primary and S2 is
the backup. The view service decides (incorrectly) that S1 is dead,
and promotes S2 to be primary. If a client thinks S1 is still the
primary and sends it an operation, S1 will forward the operation to
S2, and S2 will reply with an error indicating that it is no longer
the backup. S1 can then return an error to the client indicating that
S1 is no longer the primary; the client can then ask the view service
for the correct primary (S2) and send it the operation.

A failed key/value server may restart, but it will do so without a
copy of the replicated data (i.e. the keys and values). That is, your
key/value server will keep the data in memory, not on disk. One
consequence of keeping data only in memory
is that if there's no backup, and the primary
fails, and then restarts, it cannot then act as primary.

Only RPC may be used for interaction between clients and servers,
between different servers, and between different clients. For example,
different instances of your server are not allowed to share Go
variables or files.

The design outlined in the lab
has some fault-tolerance and performance limitations. The
view service is vulnerable to failures, since it's not replicated. The
primary and backup must process operations one at a time, limiting
their performance. A recovering server must copy a complete database
of key/value pairs from the primary, which will be slow, even if the
recovering server has an almost-up-to-date copy of the data already
(e.g. only missed a few minutes of updates while its network
connection was temporarily broken). The servers don't store the
key/value database on disk, so they can't survive simultaneous
crashes. If a temporary problem prevents primary to backup
communication, the system has only two remedies: change the view to
eliminate the backup, or keep trying; neither performs well if such
problems are frequent.

The primary/backup scheme in this lab is not based on
any published protocol. 
It's similar to Flat Datacenter Storage
(the viewservice is like FDS's metadata server,
and the primary/backup servers are like FDS's tractservers),
though FDS pays far more attention to performance.
It's also a bit like a MongoDB replica set (though MongoDB
selects the leader with a Paxos-like election).
For a detailed description of a (different) primary-backup-like
protocol, see
[Chain
Replication][1].
Chain Replication has higher performance than this lab's design,
though it assumes "fail-stop" servers (i.e. that the view
service never makes mistakes about whether a server is dead).

### Collaboration Policy
You must write all the code you hand in for 6.824, except for code
that we give you as part of the assignment. You are not allowed to
look at anyone else's solution, and you are not allowed to look at
code from previous years. You may discuss the assignments with other
students, but you may not look at or copy each others' code.

### Software

Do a git pull to get the latest lab software. We supply you
with new skeleton code and new tests in src/viewservice and
src/pbservice. You won't be using your lock server code in
this or subsequent labs.

    $ add 6.824
    $ export GOROOT=/mit/6.824/src/go
    $
    $ cd ~/6.824
    $ git pull
    ...
    $ cd src/viewservice
    $ go test
    2012/12/28 14:51:47 method Kill has wrong number of ins: 1
    First primary: --- FAIL: Test1 (1.02 seconds)
            test_test.go:13: wanted primary /var/tmp/viewserver-35913-1, got 
    FAIL
    exit status 1
    FAIL    _/afs/athena.mit.edu/user/r/t/rtm/6.824/src/viewservice 1.041s
    $
    

Ignore the method Kill error message now and in the future.
Our test code fails because viewservice/server.go has empty
RPC handlers.

You can run your code as stand-alone programs using the source in
main/viewd.go,
main/pbd.go, and
main/pbc.go.
See the comments in pbc.go.

### Part A: The Viewservice

First you'll implement a viewservice and make sure it passes
our tests; in Part B you'll build the k/value service.
Your viewservice won't itself be replicated, so it will
be relatively straightforward.

The view service goes through a sequence of numbered
_views_, each with a primary and (if possible) a backup.
A view consists of a view number and the identity (network port name) of
the view's primary and backup servers.

The primary in a view must always be either the primary
or the backup of the previous view. This helps ensure
that the key/value service's state is preserved.
An exception: when the viewservice first starts, it should
accept any server at all as the first primary.
The backup in a view can be any server (other than the primary),
or can be altogether missing if no server is available.

Each key/value server should send a Ping RPC once per 
PingInterval
(see viewservice/common.go).
The view service replies to the Ping with a description of the current
view. A Ping lets the view service know that the key/value
server is alive; informs the key/value server of the current
view; and informs the view service of the most recent view
that the key/value server knows about.
If the viewservice doesn't receive a Ping from a server
for DeadPings PingIntervals, the
viewservice should consider the server to be dead.

The view service proceeds to a new view when either it hasn't
received a Ping from the primary or backup for DeadPings
PingIntervals, or
if there is no backup and there's an idle server
(a server that's been Pinging but is
neither the primary nor the backup).
But the view service must **not** change views until 
the primary from the current view acknowledges
that it is operating in the current view (by sending
a Ping with the current view number). If the view service has not yet
received an acknowledgment for the current view from the primary of
the current view, the view service should not change views even if it
thinks that the primary or backup has died.

The acknowledgment rule prevents the view service from getting more
than one view ahead of the key/value servers. If the view service
could get arbitrarily far ahead, then it would need a more complex
design in which it kept a history of views, allowed key/value servers
to ask about old views, and garbage-collected information about old
views when appropriate.

An idle server (neither primary nor backup) should send Pings with
an argument of zero.

An example sequence of view changes:

    Viewnum Primary Backup     Event
    --------------------------------
    0       none    none
                               server S1 sends Ping(0), which returns view 0
    1       S1      none
                               server S2 sends Ping(0), which returns view 1
                                 (no view change yet since S1 hasn't acked)
                               server S1 sends Ping(1), which returns view 1 or 2
    2       S1      S2
                               server S1 sends Ping(2), which returns view 2
                               server S1 stops sending Pings
    3       S2      none
                               server S2 sends Ping(3), which returns view 3
    

We provide you with a complete client.go and
appropriate RPC definitions in common.go.
Your job is to supply the needed code in server.go.
When you're done, you should pass all the viewservice
tests:

    $ cd ~/6.824/src/viewservice
    $ go test
    2012/12/28 15:23:26 method Kill has wrong number of ins: 1
    First primary: OK
    First backup: OK
    Backup takes over if primary fails: OK
    Restarted server becomes backup: OK
    Idle third server becomes backup if primary fails: OK
    Restarted primary treated as dead: OK
    Viewserver waits for primary to ack view: OK
    Uninitialized server can't become primary: OK
    PASS
    $
    

Hint: you'll want to add field(s) to ViewServer in
server.go in order to keep track of the most recent
time at which the viewservice has heard a Ping from each
server. Perhaps a map from server names to
time.Time. You can find the current time with time.Now().

Hint: add field(s) to ViewServer to keep track of the
current view.

Hint: you'll need to keep track of whether the primary for the
current view has acknowledged it (in PingArgs.Viewnum).

Hint: your viewservice needs to make periodic decisions, for
example to promote the backup if the viewservice has missed DeadPings
pings from the primary. Add this code to the tick()
function, which is called once per PingInterval.

Hint: there may be more than two servers sending Pings. The
extra ones (beyond primary and backup) are volunteering
to be backup if needed.

Hint: the viewservice needs a way to detect that a primary
or backup has failed and re-started. For example, the primary
may crash and quickly restart without missing sending a
single Ping.

### Part B: The primary/backup key/value service

Your key/value service should continue operating correctly as long as
there has never been a time at which no server was alive. It should
also operate correctly with partitions: a server that suffers
temporary network failure without crashing, or can talk to some
computers but not others. If your service is operating with just one
server, it should be able to incorporate a recovered or idle server
(as backup), so that it can then tolerate another server failure.

Correct operation means that calls to Clerk.Get(k)
return the latest value set by a successful call to Clerk.Put(k,v),
or an empty string if the key has never been Put().

You should assume that the viewservice never halts or crashes. 

Your clients and servers may only communicate using RPC, and both
clients and servers must
send RPCs with the call() function in client.go.

It's crucial that only one primary be active at any given time. You
should have a clear story worked out for why that's the case for your
design. A danger: suppose in some view S1 is the primary; the viewservice changes
views so that S2 is the primary; but S1 hasn't yet heard about the new
view and thinks it is still primary. Then some clients might talk to
S1, and others talk to S2, and not see each others' Put()s.

A server that isn't the active primary should either
not respond to clients, or respond with an error:
it should set GetReply.Err or
PutReply.Err to something other than OK.

Clerk.Get() and Clerk.Put() should only return when they have
completed the operation. That is, Clerk.Put() should keep trying
until it has updated the key/value database, and Clerk.Get() should
keep trying until if has retrieved the current value for the
key (if any). Your server does **not** need to filter out
the duplicate RPCs that these client re-tries will generate
(though you will need to do this for Lab 3).

A server should not talk to the viewservice for every Put/Get
it receives, since that would put the viewservice on the critical path
for performance and fault-tolerance. Instead servers should
Ping the viewservice periodically
(in pbservice/server.go's tick())
to learn about new views.

Part of your one-primary-at-a-time strategy should rely on the
viewservice only promoting the backup from view _i_
to be primary in view _i+1_. If the old primary from
view _i_ tries to handle a client request, it will
forward it to its backup. If that backup hasn't heard about
view _i+1_, then it's not acting as primary yet, so
no harm done. If the backup has heard about view _i+1_
and is acting as primary, it knows enough to reject the old
primary's forwarded client requests.

You'll need to ensure that the backup sees every update to the
key/value database, by a combination of the primary initializing it with
the complete key/value database and forwarding subsequent
client Puts.

The skeleton code for the key/value servers is in src/pbservice.
It uses your viewservice, so you'll have to set up
your GOPATH as follows:

    $ export GOPATH=$HOME/6.824
    $ cd ~/6.824/src/pbservice
    $ go test -i
    $ go test
    Single primary, no backup: --- FAIL: TestBasicFail (2.00 seconds)
            test_test.go:50: first primary never formed view
    --- FAIL: TestFailPut (5.55 seconds)
            test_test.go:165: wrong primary or backup
    Concurrent Put()s to the same key: --- FAIL: TestConcurrentSame (8.51 seconds)
    ...
    Partition an old primary: --- FAIL: TestPartition (3.52 seconds)
            test_test.go:354: wrong primary or backup
    ...
    $
    

Here's a recommended plan of attack:

1.  You should start by modifying pbservice/server.go
    to Ping the viewservice to find the current view. Do this
    in the tick() function. Once a server knows the
    current view, it knows if it is the primary, the backup,
    or neither.
1.  Implement Put and Get handlers in pbservice/server.go; store
    keys and values in a map\[string\]string.
1.  Modify your Put handler so that the primary forwards updates
    to the backup.
1.  When a server becomes the backup in a new view, the primary
    should send it the primary's complete key/value database.
1.  Modify client.go so that clients keep re-trying until
    they get an answer. If the current primary doesn't respond,
    or doesn't think it's the primary, have the client consult
    the viewservice (in case the primary has changed) and try again.
    Sleep for viewservice.PingInterval
    between re-tries to avoid burning up too much CPU time.
    

You're done if you can pass all the pbservice tests:

    $ cd ~/6.824/src/pbservice
    $ go test
    Single primary, no backup: OK
    Add a backup: OK
    Primary failure: OK
    Kill last server, new one should not be active: OK
    Put() immediately after backup failure: OK
    Put() immediately after primary failure: OK
    Concurrent Put()s to the same key: OK
    Concurrent Put()s to the same key; unreliable: OK
    Partition an old primary: OK
    Repeated failures/restarts: OK
    Repeated failures/restarts; unreliable: OK
    PASS
    $
    

You'll see some "method Kill has wrong number of ins" complaints
and lots of "rpc: client protocol error" and "rpc: writing response"
complaints; ignore them.

Hint: you'll probably need to create new RPCs to forward client
requests from primary to backup, since the backup should reject
a direct client request but should accept a forwarded request.

Hint: you'll probably need to create new RPCs to handle the transfer
of the complete key/value database from the primary to a new backup.
You can send the whole database in one RPC (for example,
include a map\[string\]string in the RPC arguments).

Hint: the tester arranges for RPC replies to be lost in tests whose
description includes "unreliable". This will cause RPCs to be executed
by the receiver, but since the sender sees no reply, it cannot
tell whether the server executed the RPC.

Hint: even if your viewserver passed all the tests in Part A, it
may still have bugs that cause failures in Part B.

### Handin procedure

Submit your code via the class's submission website, located here:

[https://narula.scripts.mit.edu:444/6.824/handin.py][2]

You may use your MIT Certificate or request an API key via email to
log in for the first time.

Upload your code for each part of the lab as a gzipped tar file by the deadline at the top of
the page. To do this, execute these commands:

    $ cd ~/6.824/src
    $ tar czvf lab2a-handin.tar.gz .
    

That should produce a file called lab2a-handin.tar.gz.
Upload that file to the submission website via the webpage or use curl and your API key:

    $ curl -F file=@lab2a-handin.tar.gz \
            -F key=XXXXXXXX \
            http://narula.scripts.mit.edu/6.824/handin.py/upload

And for Part B:

    $ cd ~/6.824/src
    $ tar czvf lab2b-handin.tar.gz .
    $ curl -F file=@lab2b-handin.tar.gz \
            -F key=XXXXXXXX \
            http://narula.scripts.mit.edu/6.824/handin.py/upload

You will receive full credit if your software passes
the test\_test.go tests when we run your software on our
machines. We will use the timestamp of your **last**
submission for the purpose of calculating late days.

---

Please post questions on [Piazza][3].



[0]: ../index.html
[1]: http://www.cs.cornell.edu/home/rvr/papers/osdi04.pdf
[2]: https://narula.scripts.mit.edu:444/6.824/handin.py
[3]: http://piazza.com