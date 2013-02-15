package viewservice

import "net"
import "net/rpc"
import "log"
import "time"
import "sync"
import "fmt"
import "os"

type ViewServer struct {
  mu sync.Mutex
  l net.Listener
  dead bool
  me string


  // Your declarations here.
  pings map[string]ServerStatus
  primary string
  backup string
  currentView View

  primaryIsUpToDate bool
  servers map[string]int

}

//
// server Ping RPC handler.
//
func (vs *ViewServer) Ping(args *PingArgs, reply *PingReply) error {
  //who pinged the viewserver?
  pingFrom := args.Me

  //what view do they think it is?
  pingViewNum := args.Viewnum
  // fmt.Println("Ping from: ", pingFrom, pingViewNum, vs.primary)//, vs.backup)
  now := time.Now()

  if vs.currentView.Viewnum == 0 && vs.currentView.Primary == "" {
    //hurray first time!
    vs.primary = pingFrom
    vs.currentView.Primary = vs.primary
    vs.currentView.Viewnum = 1
  } else if vs.currentView.Backup == "" && vs.primary != pingFrom {
    vs.backup = pingFrom
    vs.currentView.Backup = vs.backup
    vs.currentView.Viewnum++
    // fmt.Println("added backup in ping ", vs.currentView.Viewnum)
  } else {

  }

  //is there a primary?
  if vs.primary != "" {
    //is the primary up to date on the current view?
    if vs.primary == pingFrom {
      if pingViewNum == vs.currentView.Viewnum {
        vs.primaryIsUpToDate = true
      } else {
        vs.primaryIsUpToDate = false
      }
    }
  }
  //update ping table
  serverStatus := new(ServerStatus)
  serverStatus.LastPingTime = now
  serverStatus.LastViewNum = pingViewNum
  vs.pings[pingFrom] = *serverStatus

  //reply to the ping
  reply.View = vs.currentView
  return nil
}

// 
// server Get() RPC handler.
//
func (vs *ViewServer) Get(args *GetArgs, reply *GetReply) error {
  reply.View = vs.currentView
  return nil
}


//
// tick() is called once per PingInterval; it should notice
// if servers have died or recovered, and change the view
// accordingly.
//
// The view service proceeds to a new view when either it hasn't received a Ping from the primary or backup for DeadPings PingIntervals, 
// or if there is no backup and there's an idle server (a server that's been Pinging but is neither the primary nor the backup). 
// But the view service must not change views until the primary from the current view acknowledges that it is operating in the current view
// (by sending a Ping with the current view number). If the view service has not yet received an acknowledgment for the current view 
// from the primary of the current view, the view service should not change views even if it thinks that the primary or backup has died.
//
// The acknowledgment rule prevents the view service from getting more than one view ahead of the key/value servers. 
// If the view service could get arbitrarily far ahead, then it would need a more complex design in which it kept a history of views, 
// allowed key/value servers to ask about old views, and garbage-collected information about old views when appropriate.
func (vs *ViewServer) tick() {
  //if there is a primary
  if vs.primary != "" {
    now := time.Now()
    isPrimaryAlive := true
    isBackup := true
    idleServer := ""
    for server, serverStatus := range vs.pings {
      timePinged := serverStatus.LastPingTime
      lastViewNum := serverStatus.LastViewNum
      //check on the server's status
      if now.Sub(timePinged) > PingInterval {
        life, ok := vs.servers[server]
        if ok {
          if life > 0 {
            //server still has x many lives left
            vs.servers[server] = life - 1
          } else {
            //methinks server is dead
            if server == vs.primary {
              //oh noes its the primary!
              isPrimaryAlive = false
            } else if server == vs.backup {
              //oh noes its the backup!
              // fmt.Println("Backup is dead")
              isBackup = false
            }
          }
        } else {
          //first time I've heard from this server
          vs.servers[server] = DeadPings
        }
      } else {
        vs.servers[server] = DeadPings
      }
      //did the primary restart?
      if server == vs.primary {
        if lastViewNum == 0 && vs.currentView.Viewnum != 0 {
          isPrimaryAlive = false
          vs.primaryIsUpToDate = true
          // fmt.Println("Detected server restart")
        }
      } else if server != vs.primary && server != vs.backup && vs.servers[server] > 0 {
        idleServer = server
      }
    }
    if !isPrimaryAlive {
      //primary is dead -> promote backup
      if isBackup && vs.primaryIsUpToDate {
        vs.primary = vs.backup
        vs.backup = ""
        vs.currentView.Primary = vs.primary
        vs.currentView.Backup = vs.backup
        if idleServer != "" {
          vs.backup = idleServer
          vs.currentView.Backup = vs.backup
          // fmt.Println("added backup in tick ", vs.currentView.Viewnum)
        }
        vs.currentView.Viewnum++
      } else {
        //the end is here!
      }
    }
  if !isBackup {
    //no backup - promote idle server
    if idleServer != "" {
      vs.backup = idleServer
      vs.currentView.Backup = vs.backup
      // fmt.Println("added backup in tick ", vs.currentView.Viewnum)
      vs.currentView.Viewnum++
    } else{
      vs.backup = ""
      vs.currentView.Backup = ""
    }
  }
  } else{
    //primary is ""

  }
  // fmt.Println("The currentView: ", vs.currentView.Viewnum, vs.currentView.Primary, vs.currentView.Backup)
}

//
// tell the server to shut itself down.
// for testing.
// please don't change this function.
//
func (vs *ViewServer) Kill() {
  vs.dead = true
  vs.l.Close()
}

func StartServer(me string) *ViewServer {
  vs := new(ViewServer)
  vs.me = me
  // Your vs.* initializations here.
  vs.pings = map[string]ServerStatus{}
  vs.primary = ""
  vs.backup = ""
  currentView := new(View)
  currentView.Viewnum = 0
  currentView.Primary = vs.primary
  currentView.Backup = vs.backup
  vs.currentView = *currentView
  vs.primaryIsUpToDate = false

  vs.servers = map[string]int{}


  // tell net/rpc about our RPC server and handlers.
  rpcs := rpc.NewServer()
  rpcs.Register(vs)

  // prepare to receive connections from clients.
  // change "unix" to "tcp" to use over a network.
  os.Remove(vs.me) // only needed for "unix"
  l, e := net.Listen("unix", vs.me);
  if e != nil {
    log.Fatal("listen error: ", e);
  }
  vs.l = l

  // please don't change any of the following code,
  // or do anything to subvert it.

  // create a thread to accept RPC connections from clients.
  go func() {
    for vs.dead == false {
      conn, err := vs.l.Accept()
      if err == nil && vs.dead == false {
        go rpcs.ServeConn(conn)
      } else if err == nil {
        conn.Close()
      }
      if err != nil && vs.dead == false {
        fmt.Printf("ViewServer(%v) accept: %v\n", me, err.Error())
        vs.Kill()
      }
    }
  }()

  // create a thread to call tick() periodically.
  go func() {
    for vs.dead == false {
      vs.tick()
      time.Sleep(PingInterval)
    }
  }()

  return vs
}
