## [6.824][0] - Spring 2013

# 6.824 Lab 1: Lock Server

### Due: Feb 11, 5:00pm

---

### Introduction

The labs in 6.824 will give you experience designing, building, and
debugging distributed systems. The labs will focus on fault tolerance,
since that is perhaps the most difficult and important aspect of
distributed systems.

In this lab you'll build a lock service that can keep operating
despite a single "fail-stop" failure of a server.
Clients will talk to your lock service via RPC. Clients can use two
RPC requests: Lock(lockname) and Unlock(lockname). The lock service
maintains state about an open-ended set of named locks. If a
Lock(lockname) request arrives at the service and the named lock is
not held, or has never been used before, the lock service should remember
that the lock is held, and return a successful reply to the client. If
the lock is held, the service should return an unsuccessful reply to
the client. If a client sends Unlock(lockname), the service should
mark the named lock as not held. If the lock had been held, the service
should return a successful reply to the client; otherwise an unsuccessful
reply.

Your lock service will be replicated on two servers, a primary and a
backup. If there has been no failure, clients will send lock and
unlock requests to the primary, and the primary will forward them to the
backup. The point is for the backup to maintain state identical to
that of the primary (i.e., the primary and backup states should
agree about whether each lock is held or free).

The only failure your system is required to tolerate is a single
fail-stop server failure. A fail-stop failure means that the server
halts. There are no other kinds of failure in this lab (for example, clients don't
fail, the network doesn't fail, all network messages to non-crashed
servers are delivered, and a server fails only by halting, not by computing
incorrectly). Servers are never repaired in this lab -- once a primary or
backup has failed, it will stay failed.

If a client cannot get a response from the primary, it should contact
the backup instead. If the primary cannot get a response from the
backup while forwarding client requests, the primary should stop
forwarding client requests to the backup.

The failure model in this lab (fail-stop, no repair) is so restricted
that your lock service won't be much use in practice; subsequent labs
will be able tolerate a much wider range of failures.

### Collaboration Policy
You must write all the code you hand in for 6.824, except for code
that we give you as part of the assignment. You are not allowed to
look at anyone else's solution, and you are not allowed to look at
code from previous years. You may discuss the assignments with other
students, but you may not look at or copy each others' code.

### Software
You'll implement this lab (and all the labs) in 
[Go][1]. The Go web site contains lots
of tutorial information which you may want to look at. We supply you
with a partial lock server implementation (just the boring bits) and
some tests.

You'll fetch the initial lab software with
[git][2]
(a version control system).
To learn more about git, take a look at the
[git
user's manual][3], or, if you are already familiar with other version control
systems, you may find this
[CS-oriented
overview of git][4] useful.

The URL for the course git repository is
[http://am.lcs.mit.edu/6.824-2013/golabs-class.git][5].
To install the files in your Athena account, you need to _clone_
the course repository, by running the commands below. You must use an
x86 or x86\_64 Athena machine; that is, uname -a should
mention i386 GNU/Linux or i686 GNU/Linux or 
x86\_64 GNU/Linux. You can
log into a public i686 Athena host with
athena.dialup.mit.edu.

    
    $ add git
    $ git clone http://am.lcs.mit.edu/6.824-2013/golabs-class.git 6.824
    $ cd 6.824
    $ ls
    src
    $ 
    

Git allows you to keep track of the changes you make to the code.
For example, if you want
to checkpoint your progress, you can commit your changes
by running:

    
    $ git commit -am 'partial solution to lab 1'
    $ 
    

### Getting started

Compile the initial software we provide you and run our tests:

    
    $ add ggo_v1.0.1
    $ PATH=/mit/ggo_v1.0.1/bin:$PATH
    $ export GOROOT=/mit/ggo_v1.0.1/go
    $ cd ~/6.824/src/lockservice
    $
    $ go test
    Test: Basic lock/unlock ...
    --- FAIL: TestBasic (0.00 seconds)
    test_test.go:14:        Lock(a) returned false; expected true
    Test: Primary failure ...
    --- FAIL: TestPrimaryFail1 (0.00 seconds)
    test_test.go:14:        Lock(d) returned false; expected true
    Test: Primary failure just before reply #1 ...
    --- FAIL: TestPrimaryFail2 (2.00 seconds)
    ...
    

You'll see there are five files in the lockservice
directory.
common.go contains RPC request and response definitions
for the Lock and Unlock RPCs.
client.go contains Lock and Unlock stubs that are to
be linked into client programs; these stubs talk to the 
lock service. The client code we've supplied you only contacts
the primary; you'll have to modify client.go to switch to the backup
if the primary seems to have failed.
server.go contains the server code; it has a basic
implementation of a Lock handler, not no Unlock handler,
and no code to forward operations from the primary to the backup.
Finally, test\_test.go contains our test code; you
should not modify it, but you'll need to look at it
to see the details of what we're testing.

We've given you code that sends RPCs via "UNIX-domain sockets".
This means that RPCs only work between processes on the same machine.
It would be easy to convert the code to use TCP/IP-based
RPC instead, so that it would communicate between machines;
you'd have to change the first argument to calls to Listen() and Dial() to
"tcp" instead of "unix", and the second argument to a port number
like ":5100".

Your lock code is a library intended to be used in client and
server programs. You can see source for simple lock server and client programs
in ~/6.824/src/main/lockd.go and lockc.go.
You can practice starting a primary and a backup server process,
locking a few locks, then crashing the primary (with control-C)
and observing that the backup server still thinks the locks are held.
lockd.go and lockc.go are just for your amusement;
we will not use them for testing your code.

### Your Job
Your job is to modify client.go, server.go,
and common.go
so that they pass the tests in test\_test.go (which
go test runs). When you're done you should see:

    
    $ go test
    Test: Basic lock/unlock ...
      ... Passed
    Test: Primary failure ...
      ... Passed
    Test: Primary failure just before reply #1 ...
      ... Passed
    Test: Primary failure just before reply #2 ...
      ... Passed
    Test: Primary failure just before reply #3 ...
      ... Passed
    Test: Primary failure just before reply #4 ...
      ... Passed
    Test: Primary failure just before reply #5 ...
      ... Passed
    Test: Backup failure ...
      ... Passed
    Test: Multiple clients with primary failure ...
      ... Passed
    Test: Multiple clients, single lock, primary failure ...
      ... Passed
    PASS
    

The above output omits some benign Go rpc errors.

The only communication allowed between client and servers, and between
primary and backup, is via RPC. For example, you are not allowed to
communicate via Go variables or files.

### Hints

Start by modifying client.go so that Unlock()
is a copy of Lock(), but modified to send Unlock
RPCs. You can test your code by inserting fmt.Printf()s
into server.go.

Next, modify server.go so that the Unlock()
function unlocks the lock. This should require only a few lines of code,
and it will let you pass the first test.

Modify client.go so that it first sends an RPC to the
primary, and if the primary does not respond, it sends an RPC to
the backup.

Modify server.go so that the primary tells the backup
about Lock and Unlock operations. Use RPC for this communication.

Now do something about the case in which the client sends to
the primary, the primary forwards to the backup and then crashes,
and the client re-sends the RPC to the backup -- which has now
already seen the RPC.

It is OK to assume that each client application will only make one
call to Clerk.Lock() or Clerk.Unlock() at a time. Of course there may
be more than one client application, each with its own Clerk.

Remember that the Go RPC server framework starts a new thread for each
received RPC request. Thus if multiple RPCs arrive at the same time
(from multiple clients), there may be multiple threads running
concurrently in the server.

The easiest way to track down bugs is to insert fmt.Printf()
statements, collect the output in a file with go test \>
out, and then think about whether the output matches your
understanding of how your code should behave. The last step is the
most important.

### Handin procedure

Submit your code via the class's submission website, located here:

[https://narula.scripts.mit.edu:444/6.824/handin.py][6]

You may use your MIT Certificate or request an API key via email to
log in for the first time.

Upload your code as a gzipped tar file by the deadline at the top of
the page. To do this, execute these commands:

    
    $ cd ~/6.824/src
    $ tar czvf lab1-handin.tar.gz .
    

That should produce a file called lab1-handin.tar.gz.
Upload that file to the submission website via the webpage or use curl and your API key:

    
    $ curl -F file=@lab1-handin.tar.gz \
            -F key=XXXXXXXX \
            http://narula.scripts.mit.edu/6.824/upload

You will receive full credit if your software passes
the test\_test.go tests when we run your software on our
machines. We will use the timestamp of your **last**
submission for the purpose of calculating late days.

---

Please post questions on [Piazza][7].



[0]: ../index.html
[1]: http://www.golang.org/
[2]: http://git.or.cz/
[3]: http://www.kernel.org/pub/software/scm/git/docs/user-manual.html
[4]: http://eagain.net/articles/git-for-computer-scientists/
[5]: http://am.lcs.mit.edu/6.824-2013/golabs-class.git
[6]: https://narula.scripts.mit.edu:444/6.824/handin.py
[7]: http://piazza.com