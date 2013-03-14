package kvpaxos

import "net"
import "fmt"
import "net/rpc"
import "log"
import "paxos"
import "sync"
import "os"
import "syscall"
import "encoding/gob"
import "math/rand"
import "time"


type Op struct {
  // Your definitions here.
  // Field names must start with capital letters,
  // otherwise RPC will break.
  Type string
  Key string
  Value string
  From int
  RequestID int
}

func MakeGetOp(optype string, key string, from int, requestID int) Op {
  op := Op{}
  op.Type = optype
  op.Key = key
  op.From = from
  op.RequestID = requestID
  op.Value = ""
  return op
}

func MakePutOp(optype string, key string, value string, from int, requestID int) Op {
  op := Op{}
  op.Type = optype
  op.Key = key
  op.Value = value
  op.From = from
  op.RequestID = requestID
  return op
}

type KVPaxos struct {
  mu sync.Mutex
  l net.Listener
  me int
  dead bool // for testing
  unreliable bool // for testing
  px *paxos.Paxos

  // Your definitions here.
  currentSeq int
  requestsSeen map[int]GetReply //from requestID -> requests seen
  db map[string]string //key/value storage
}

// 
// Your kvpaxos servers will use Paxos to agree on the order in which client Put()s and Get()s execute.
// Each time a kvpaxos server receives a Put() or Get() RPC, it will use Paxos to cause some Paxos 
// instance's value to be a description of that Put() or Get(). That instance's sequence number determines
// when the Put() or Get() executes relative to other Put()s and Get()s. In order to find the value to 
// be returned by a Get(), kvpaxos should first apply all Put()s that are ordered before the Get() to its key/value database.
// 
// You should think of kvpaxos as using Paxos to implement a "log" of Put/Get operations. That is, 
// each Paxos instance is a log element, and the order of operations in the log is the order in which 
// all kvpaxos servers will apply the operations to their key/value databases. Paxos will ensure that 
// the kvpaxos servers agree on this order.
// 

func (kv *KVPaxos) Paxos(op Op) int {
  seq := kv.currentSeq
  for {
    kv.px.Start(seq, op)
    sleepTime := 10 * time.Millisecond //20 * time.Millisecond
    var actualOp Op
    for {
      done, top := kv.px.Status(seq)
      //has paxos decided? 
      if done {
        actualOp = top.(Op)
        break
      }
      time.Sleep(sleepTime)
      //readjust sleepTime
      if sleepTime < 10 * time.Second {
        sleepTime *= 2
      }
    }
    if actualOp == op{
      break;
    } else {
      seq++
    }
  }
  //return the log seq number for the op
  return seq
}

func (kv *KVPaxos) UpdateLocalLog(currentSeq int, seq int){
  for i := currentSeq; i <= seq; i++ {
    if done, value := kv.px.Status(i); done {
      iOp := value.(Op)
      if iOp.Type == PUT {
        if _, ok := kv.requestsSeen[iOp.RequestID]; !ok {
          var reply GetReply
          reply.Err = OK
          kv.requestsSeen[iOp.RequestID] = reply
          kv.db[iOp.Key] = iOp.Value
        }
      } else if iOp.Type == GET {
        if _, ok := kv.requestsSeen[iOp.RequestID]; !ok {
          var reply GetReply
          if value, _ok := kv.db[iOp.Key]; _ok{
            reply.Err = OK
            reply.Value = value
          } else {
              reply.Err = ErrNoKey
          }
          kv.requestsSeen[iOp.RequestID] = reply
        }
      }
    }
  }
} 


func (kv *KVPaxos) Get(args *GetArgs, reply *GetReply) error {
  // Your code here.
  //don't forget to lock :)
  kv.mu.Lock()
  defer kv.mu.Unlock()

  // optype, value, from, requestID
  op := MakeGetOp(GET, args.Key, args.ClientID, args.RequestID)

  
  //check to see if we've already handled this 
  if existingReply, ok := kv.requestsSeen[op.RequestID]; ok{
    reply.Value = existingReply.Value
    reply.Err = existingReply.Err
    return nil
  }

  //send the op to paxos
  seq := kv.Paxos(op)

  //update the log from where I am to where Paxos currently is
  cS, s := kv.currentSeq, seq
  kv.UpdateLocalLog(cS, s) 

  //fill in reply
  if existingReply, ok := kv.requestsSeen[op.RequestID]; ok{
    reply.Value = existingReply.Value
    reply.Err = existingReply.Err
  }

  //call done
  kv.px.Done(s)

  //update currentSeq
  kv.currentSeq = s + 1
  return nil
}


func (kv *KVPaxos) Put(args *PutArgs, reply *PutReply) error {
  // Your code here.
  //don't forget to lock :)
  kv.mu.Lock()
  defer kv.mu.Unlock()


  //optype, key, value, from, requestID
  op := MakePutOp(PUT, args.Key, args.Value, args.ClientID, args.RequestID)
  
  //check to see if we've already handled this 
  if existingReply, found := kv.requestsSeen[op.RequestID]; found{
    reply.Err = existingReply.Err
    return nil
  }

  //send the op to paxos
  seq := kv.Paxos(op)

  //update the log from where I am to where Paxos currently is
  cS, s := kv.currentSeq, seq
  //go is pass by what?
  kv.UpdateLocalLog(cS, s) 

  //update reply 
  if existingReply, found := kv.requestsSeen[op.RequestID]; found{
    reply.Err = existingReply.Err
  }

  //call done
  kv.px.Done(s)

  //update currentSeq
  kv.currentSeq = s + 1
  return nil
}

// tell the server to shut itself down.
// please do not change this function.
func (kv *KVPaxos) kill() {
  kv.dead = true
  kv.l.Close()
  kv.px.Kill()
}

//
// servers[] contains the ports of the set of
// servers that will cooperate via Paxos to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// 
func StartServer(servers []string, me int) *KVPaxos {
  // this call is all that's needed to persuade
  // Go's RPC library to marshall/unmarshall
  // struct Op.
  gob.Register(Op{})

  kv := new(KVPaxos)
  kv.me = me

  // Your initialization code here.
  kv.db = map[string]string{}
  kv.requestsSeen = map[int]GetReply{}


  rpcs := rpc.NewServer()
  rpcs.Register(kv)

  kv.px = paxos.Make(servers, me, rpcs)

  os.Remove(servers[me])
  l, e := net.Listen("unix", servers[me]);
  if e != nil {
    log.Fatal("listen error: ", e);
  }
  kv.l = l

  // please do not change any of the following code,
  // or do anything to subvert it.

  go func() {
    for kv.dead == false {
      conn, err := kv.l.Accept()
      if err == nil && kv.dead == false {
        if kv.unreliable && (rand.Int63() % 1000) < 100 {
          // discard the request.
          conn.Close()
        } else if kv.unreliable && (rand.Int63() % 1000) < 200 {
          // process the request but force discard of reply.
          c1 := conn.(*net.UnixConn)
          f, _ := c1.File()
          err := syscall.Shutdown(int(f.Fd()), syscall.SHUT_WR)
          if err != nil {
            fmt.Printf("shutdown: %v\n", err)
          }
          go rpcs.ServeConn(conn)
        } else {
          go rpcs.ServeConn(conn)
        }
      } else if err == nil {
        conn.Close()
      }
      if err != nil && kv.dead == false {
        fmt.Printf("KVPaxos(%v) accept: %v\n", me, err.Error())
        kv.kill()
      }
    }
  }()

  return kv
}

