package paxos

//
// Paxos library, to be included in an application.
// Multiple applications will run, each including
// a Paxos peer.
//
// Manages a sequence of agreed-on values.
// The set of peers is fixed.
// Copes with network failures (partition, msg loss, &c).
// Does not store anything persistently, so cannot handle crash+restart.
//
// The application interface:
//
// px = paxos.Make(peers []string, me string)
// px.Start(seq int, v interface{}) -- start agreement on new instance
// px.Status(seq int) (decided bool, v interface{}) -- get info about an instance
// px.Done(seq int) -- ok to forget all instances <= seq
// px.Max() int -- highest instance seq known, or -1
// px.Min() int -- instances before this seq have been forgotten
//

import "net"
import "net/rpc"
import "log"
import "os"
import "syscall"
import "sync"
import "fmt"
import "math/rand"
import "math"
import "time"
import "container/list"

type Paxos struct {
  mu sync.Mutex
  l net.Listener
  dead bool
  unreliable bool
  rpcCount int
  peers []string
  me int // index into peers[]


  // Your data here.
  instances map[int]Instance
  maxPeerDones map[string]int


}

// proposer(v):
//   choose n, unique and higher than any n seen so far
//   send prepare(n) to all servers including self
//   if prepare_ok(n_a, v_a) from majority:
//     v' = v_a with highest n_a; choose own v otherwise
//     send accept(n, v') to all
//     if accept_ok(n) from majority:
//       send decided(v') to all

// Phase 1. 
// (a) A proposer selects a proposal number n and sends a prepare
//     request with number n to a majority of acceptors.
// (b) If an acceptor receives a prepare request with number n greater
//     than that of any prepare request to which it has already responded,
//     then it responds to the request with a promise not to accept any more
//     proposals numbered less than n and with the highest-numbered proposal (if any) that it has accepted.
func (px *Paxos) Propose(instance int, value interface{}) {
//   choose n, unique and higher than any n seen so far
//   send prepare(n) to all servers including self
  proposal := 0
  next := -1
  proposalDone := false
  for !proposalDone {
    next += 1
    proposal = next
    
    replies := list.New()
    quorum := len(px.peers) / 2
    
    for _, peer := range px.peers {
      prepareArgs := &PrepareArgs{instance, proposal, px.maxPeerDones[px.peers[px.me]], px.peers[px.me]}
      var reply PrepareReply
      if peer != px.peers[px.me]{
        ok := call(peer, "Paxos.Prepare", prepareArgs, &reply)
        if ok {
          replies.PushBack(reply)
        }
      } else {
        px.Prepare(prepareArgs, &reply)
        replies.PushBack(reply)
      }
    }

//   if prepare_ok(n_a, v_a) from majority:
    prepareReplyCount := 0
    maxProposal := -1
    maxProposalValue := value
    for e := replies.Front(); e != nil; e = e.Next(){
      reply := e.Value.(PrepareReply)
      if reply.OK {
        prepareReplyCount++
        if reply.MaxProposalAcceptedSoFar != -1 && reply.MaxProposalAcceptedSoFar > maxProposal{
          maxProposal = reply.MaxProposalAcceptedSoFar
          maxProposalValue = reply.Value
        }
      } else {
        next = int(math.Max(float64(next), float64(reply.NextProposalNumber)))
      }
    }
    
    replies = list.New()
    
    //did we reach quorum?
    if (prepareReplyCount > quorum) {
//     v' = v_a with highest n_a; choose own v otherwise
//     send accept(n, v') to all
      for _, peer := range px.peers {
        acceptArgs := &AcceptArgs{instance, proposal, maxProposalValue}
        var reply AcceptReply
        if peer != px.peers[px.me] {
          ok := call(peer, "Paxos.Accept", acceptArgs, &reply)
          if ok {
            replies.PushBack(reply)
          }
        } else {
          px.Accept(acceptArgs, &reply)
          replies.PushBack(reply)
        }
      }

      var accepted = 0
      for e := replies.Front(); e != nil; e = e.Next() {
        reply := e.Value.(AcceptReply)
        if reply.OK {
          accepted++
        } else {
          next = int(math.Max(float64(next), float64(reply.NextProposalNumber)))
        }
      }
//     if accept_ok(n) from majority:
//       send decided(v') to all
      if accepted > quorum {
        decidedArgs := &DecidedArgs{instance, maxProposalValue}
        var reply DecidedReply
        for _, peer := range px.peers {
          if peer != px.peers[px.me]{
            call(peer, "Paxos.Decided", decidedArgs, &reply)
          } else {
            px.Decided(decidedArgs, &reply)
          }
        }
        proposalDone = true
      }
    } 
    time.Sleep(5*time.Millisecond)   
  }
}

func (px *Paxos) GetPaxosState(seq int) *Instance {
  if instance, found := px.instances[seq]; found {
    return &instance
  }
  px.instances[seq] = *MakeInstance()
  instance := px.instances[seq]
  return &instance
}

// acceptor's prepare(n) handler:
//   if n > n_p
//     n_p = n
//     reply prepare_ok(n_a, v_a)
//   else
//     reply prepare_reject
func (px *Paxos) Prepare(args *PrepareArgs, reply *PrepareReply) error {
  //don't forget to lock :)
  px.mu.Lock()
  defer px.mu.Unlock()
  
  px.maxPeerDones[args.Me] = int(math.Max(float64(px.maxPeerDones[args.Me]), float64(args.Done)))
  
  //garbage collecting
  min := px.Min()
  for instance := range(px.instances) {
    if min > instance {
      delete(px.instances, instance)
    }
  }

  reply.OK = false
  instance := px.GetPaxosState(args.Instance)
  if instance.highestResponded < args.Proposal {
    instance.highestResponded = args.Proposal

    px.instances[args.Instance] = *instance
    reply.MaxProposalAcceptedSoFar = instance.highestAccepted
    reply.Value = instance.value
    reply.OK = true
  }
  
  return nil
}

// Phase 2.
// (a) If the proposer receives a response to its prepare requests
//     (numbered n) from a majority of acceptors, then it sends an accept
//     request to each of those acceptors for a proposal numbered n with a
//     value v, where v is the value of the highest-numbered proposal among
//     the responses, or is any value if the responses reported no proposals.
// (b) If an acceptor receives an accept request for a proposal numbered
//     n, it accepts the proposal unless it has already responded to a prepare
//     request having a number greater than n.

// acceptor's accept(n, v) handler:
//   if n >= n_p
//     n_p = n
//     n_a = n
//     v_a = v
//     reply accept_ok(n)
//   else
//     reply accept_reject

func (px *Paxos) Accept(args *AcceptArgs, reply *AcceptReply) error {
  //don't forget to lock :)
  px.mu.Lock()
  defer px.mu.Unlock()
  
  reply.OK = false
  instance := px.GetPaxosState(args.Instance)
  if instance.highestResponded <= args.Proposal {
    instance.highestResponded = args.Proposal
    instance.highestAccepted = args.Proposal
    instance.value = args.Value
    px.instances[args.Instance] = *instance
    
    reply.OK = true
  } 
  return nil
}

func (px *Paxos) Decided(args *DecidedArgs, reply *DecidedReply) error {
  //don't forget to lock :)
  px.mu.Lock()
  defer px.mu.Unlock()
  
  instance := px.GetPaxosState(args.Instance)
  instance.value = args.Value
  instance.agreed = true
  px.instances[args.Instance] = *instance

  reply.OK = true
  return nil
}


//
// call() sends an RPC to the rpcname handler on server srv
// with arguments args, waits for the reply, and leaves the
// reply in reply. the reply argument should be a pointer
// to a reply structure.
//
// the return value is true if the server responded, and false
// if call() was not able to contact the server. in particular,
// the replys contents are only valid if call() returned true.
//
// you should assume that call() will time out and return an
// error after a while if it does not get a reply from the server.
//
// please use call() to send all RPCs, in client.go and server.go.
// please do not change this function.
//
func call(srv string, name string, args interface{}, reply interface{}) bool {
  c, err := rpc.Dial("unix", srv)
  if err != nil {
    err1 := err.(*net.OpError)
    if err1.Err != syscall.ENOENT && err1.Err != syscall.ECONNREFUSED {
      fmt.Printf("paxos Dial() failed: %v\n", err1)
    }
    return false
  }
  defer c.Close()
    
  err = c.Call(name, args, reply)
  if err == nil {
    return true
  }
  return false
}


//
// the application wants paxos to start agreement on
// instance seq, with proposed value v.
// Start() returns right away; the application will
// call Status() to find out if/when agreement
// is reached.
//
func (px *Paxos) Start(seq int, v interface{}) {
    //don't forget to lock :)
  px.mu.Lock()
  defer px.mu.Unlock()

  if px.Min() <= seq {
    instance := MakeInstance()
    px.instances[seq] = *instance
    go px.Propose(seq, v)
  }
  return
}

//
// the application on this machine is done with
// all instances <= seq.
//
// see the comments for Min() for more explanation.
//
func (px *Paxos) Done(seq int) {
  //don't forget to lock :)
  px.mu.Lock()
  defer px.mu.Unlock()

  //update my done map
  me := px.peers[px.me]
  px.maxPeerDones[me] = int(math.Max(float64(seq), float64(px.maxPeerDones[me])))

  //garbage collecting
  // min := px.Min()
  for instance := range(px.instances) {
    if seq >= instance {
      delete(px.instances, instance)
    }
  }
}

//
// the application wants to know the
// highest instance sequence known to
// this peer.
//
func (px *Paxos) Max() int {
  maxSeq := -1
  for inst := range(px.instances){
    maxSeq = int(math.Max(float64(maxSeq), float64(inst)))
  }
  return maxSeq
}

//
// Min() should return one more than the minimum among z_i,
// where z_i is the highest number ever passed
// to Done() on peer i. A peers z_i is -1 if it has
// never called Done().
//
// Paxos is required to have forgotten all information
// about any instances it knows that are < Min().
// The point is to free up memory in long-running
// Paxos-based servers.
//
// It is illegal to call Done(i) on a peer and
// then call Start(j) on that peer for any j <= i.
//
// Paxos peers need to exchange their highest Done()
// arguments in order to implement Min(). These
// exchanges can be piggybacked on ordinary Paxos
// agreement protocol messages, so it is OK if one
// peers Min does not reflect another Peers Done()
// until after the next instance is agreed to.
//
// The fact that Min() is defined as a minimum over
// *all* Paxos peers means that Min() cannot increase until
// all peers have been heard from. So if a peer is dead
// or unreachable, other peers Min()s will not increase
// even if all reachable peers call Done. The reason for
// this is that when the unreachable peer comes back to
// life, it will need to catch up on instances that it
// missed -- the other peers therefor cannot forget these
// instances.
// 
func (px *Paxos) Min() int {
  me := px.peers[px.me]
  minSeq := px.maxPeerDones[me]
  for _, peerMin := range(px.maxPeerDones){
    minSeq = int(math.Min(float64(minSeq), float64(peerMin)))
  }
  for instance := range(px.instances) {
    if minSeq >= instance {
      delete(px.instances, instance)
    }
  }
  return minSeq + 1
}

//
// the application wants to know whether this
// peer thinks an instance has been decided,
// and if so what the agreed value is. Status()
// should just inspect the local peers state;
// it should not contact other Paxos peers.
//
func (px *Paxos) Status(seq int) (bool, interface{}) {
    //don't forget to lock :)
  px.mu.Lock()
  defer px.mu.Unlock()

  if px.Min() <= seq {
    instance := px.GetPaxosState(seq)
    return instance.agreed, instance.value
  }
  return false, nil
}


//
// tell the peer to shut itself down.
// for testing.
// please do not change this function.
//
func (px *Paxos) Kill() {
  px.dead = true
  if px.l != nil {
    px.l.Close()
  }
}

//
// the application wants to create a paxos peer.
// the ports of all the paxos peers (including this one)
// are in peers[]. this servers port is peers[me].
//
func Make(peers []string, me int, rpcs *rpc.Server) *Paxos {
  px := &Paxos{}
  px.peers = peers
  px.me = me


  // Your initialization code here.
  px.instances = map[int]Instance{}
  px.maxPeerDones = map[string]int{}
  for _, peer := range(px.peers) {
    px.maxPeerDones[peer] = -1
  }

  if rpcs != nil {
    // caller will create socket &c
    rpcs.Register(px)
  } else {
    rpcs = rpc.NewServer()
    rpcs.Register(px)

    // prepare to receive connections from clients.
    // change "unix" to "tcp" to use over a network.
    os.Remove(peers[me]) // only needed for "unix"
    l, e := net.Listen("unix", peers[me]);
    if e != nil {
      log.Fatal("listen error: ", e);
    }
    px.l = l
    
    // please do not change any of the following code,
    // or do anything to subvert it.
    
    // create a thread to accept RPC connections
    go func() {
      for px.dead == false {
        conn, err := px.l.Accept()
        if err == nil && px.dead == false {
          if px.unreliable && (rand.Int63() % 1000) < 100 {
            // discard the request.
            conn.Close()
          } else if px.unreliable && (rand.Int63() % 1000) < 200 {
            // process the request but force discard of reply.
            c1 := conn.(*net.UnixConn)
            f, _ := c1.File()
            err := syscall.Shutdown(int(f.Fd()), syscall.SHUT_WR)
            if err != nil {
              fmt.Printf("shutdown: %v\n", err)
            }
            px.rpcCount++
            go rpcs.ServeConn(conn)
          } else {
            px.rpcCount++
            go rpcs.ServeConn(conn)
          }
        } else if err == nil {
          conn.Close()
        }
        if err != nil && px.dead == false {
          fmt.Printf("Paxos(%v) accept: %v\n", me, err.Error())
        }
      }
    }()
  }


  return px
}
