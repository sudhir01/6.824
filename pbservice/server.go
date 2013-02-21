package pbservice

import "net"
import "fmt"
import "net/rpc"
import "log"
import "time"
import "viewservice"
import "sync"
import "os"
import "syscall"
import "math/rand"


type PBServer struct {
  mu sync.Mutex
  l net.Listener
  dead bool // for testing
  unreliable bool // for testing
  me string
  vs *viewservice.Clerk
  
  // Your declarations here.
  db map[string]string //key/value storage
  state string //primary, backup etc
  currentView viewservice.View

}

func (pb *PBServer) Get(args *GetArgs, reply *GetReply) error {
  pb.mu.Lock()
  defer pb.mu.Unlock()
  if pb.state == "primary" {
    if value, ok := pb.db[args.Key]; ok {
      reply.Value = value
      reply.Err = "OK"
    } else {
      reply.Value = ""
      reply.Err = ErrNoKey
    }
  } else {
    reply.Err = ErrWrongServer
  }

  return nil
}

func (pb *PBServer) Put(args *PutArgs, reply *PutReply) error {
  pb.mu.Lock()
  defer pb.mu.Unlock()
  reply.Err = OK

  // Your code here.
  if pb.state == "primary" {
    pb.db[args.Key] = args.Value
    ok := false
    var backupReply PutReply
    tries := 5
    for !ok && pb.currentView.Backup != "" && tries > 0{
      fmt.Println("fwding to backup...")
      ok = call(pb.currentView.Backup, "PBServer.PutBackup", args, &backupReply)
      if backupReply.Err == ErrWrongServer {
        fmt.Println("what I thought was the backup isn't")
      }
      tries--
      time.Sleep(viewservice.PingInterval)
    }
  } else {
    reply.Err = ErrWrongServer
  }

  return nil
}

func (pb *PBServer) PutBackup(args *PutArgs, reply *PutReply) error {
  pb.mu.Lock()
  defer pb.mu.Unlock()
  reply.Err = OK
  if pb.state == "backup" {
    fmt.Println("backup recieved", args.Key, args.Value, pb.me)
    pb.db[args.Key] = args.Value
  } else {
    reply.Err = ErrWrongServer
  }

  return nil
}

func (pb *PBServer) RestoreBackup(args *RestoreArgs, reply *PutReply) error {
  pb.mu.Lock()
  defer pb.mu.Unlock()
  reply.Err = OK
  // if pb.state == "backup" {
    fmt.Println("backup restored")
    pb.db = args.Db
  // } else {
  //   reply.Err = ErrWrongServer
  // }

  return nil
}

func (pb *PBServer) SendRestoreBackup(){
  ok := false
  var backupReply PutReply
  args := &RestoreArgs{}
  args.Db = pb.db
  for !ok && pb.currentView.Backup != ""{
    fmt.Println("restoring backup...")
    ok = call(pb.currentView.Backup, "PBServer.RestoreBackup", args, &backupReply)
    if backupReply.Err == ErrWrongServer {
      //what I thought was the backup isn't
      fmt.Println("backup rejected restore")
    }
  }
}


//
// ping the viewserver periodically.
// if view changed:
//   transition to new view.
//   manage transfer of state from primary to new backup.
//
func (pb *PBServer) tick() {
  pb.mu.Lock()
  defer pb.mu.Unlock()
  //ping viewserver
  view, _ := pb.vs.Ping(pb.currentView.Viewnum)
  // fmt.Println("tick ping: ", pb.me, view.Primary, view.Backup, view.Viewnum)
  oldView := pb.currentView
  pb.currentView = view
  if pb.currentView.Primary == pb.me {
    pb.state = "primary"
  } else if pb.currentView.Backup == pb.me {
    pb.state = "backup"
  } else {
    pb.state = "neither"
  }
  if oldView.Viewnum != view.Viewnum {
    //view has changed - find out why
    if view.Backup != oldView.Backup && pb.state == "primary" {
      //backup has changed, send new backup the db
      pb.SendRestoreBackup()
    }
    fmt.Println("view has changed between ticks!", pb.me, pb.currentView.Primary)
  }
}

// tell the server to shut itself down.
// please do not change this function.
func (pb *PBServer) kill() {
  pb.dead = true
  pb.l.Close()
}


func StartServer(vshost string, me string) *PBServer {
  pb := new(PBServer)
  pb.me = me
  pb.vs = viewservice.MakeClerk(me, vshost)
  // Your pb.* initializations here.
  pb.db = map[string]string{}
  pb.state = "unknown"
  currentView := new(viewservice.View)
  currentView.Viewnum = 0
  currentView.Primary = ""
  currentView.Backup = ""
  pb.currentView = *currentView


  rpcs := rpc.NewServer()
  rpcs.Register(pb)

  os.Remove(pb.me)
  l, e := net.Listen("unix", pb.me);
  if e != nil {
    log.Fatal("listen error: ", e);
  }
  pb.l = l

  // please do not change any of the following code,
  // or do anything to subvert it.

  go func() {
    for pb.dead == false {
      conn, err := pb.l.Accept()
      if err == nil && pb.dead == false {
        if pb.unreliable && (rand.Int63() % 1000) < 100 {
          // discard the request.
          conn.Close()
        } else if pb.unreliable && (rand.Int63() % 1000) < 200 {
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
      if err != nil && pb.dead == false {
        fmt.Printf("PBServer(%v) accept: %v\n", me, err.Error())
        pb.kill()
      }
    }
  }()

  go func() {
    for pb.dead == false {
      pb.tick()
      time.Sleep(viewservice.PingInterval)
    }
  }()

  return pb
}
