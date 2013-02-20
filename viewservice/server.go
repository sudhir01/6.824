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

  primaryAckedCurrentView bool
  servers map[string]int

}

//
// server Ping RPC handler.
//
func (vs *ViewServer) Ping(args *PingArgs, reply *PingReply) error {
  vs.mu.Lock()
  defer vs.mu.Unlock()

  //who pinged the viewserver?
  pingFrom := args.Me

  //what view do they think it is?
  pingViewNum := args.Viewnum
  // fmt.Println("Ping from: ", pingFrom, pingViewNum, vs.primary)//, vs.backup)
  now := time.Now()
  //update ping table
  if serverStatus, ok := vs.pings[pingFrom]; ok {
    serverStatus.LastPingTime = now
    serverStatus.LastViewNum = serverStatus.CurrentViewNum
    serverStatus.CurrentViewNum = pingViewNum
    vs.pings[pingFrom] = serverStatus
    // fmt.Println("serverstatus", serverStatus)
  } else {
    serverStatus := new(ServerStatus)
    serverStatus.LastPingTime = now
    serverStatus.LastViewNum = serverStatus.CurrentViewNum
    serverStatus.CurrentViewNum = vs.currentView.Viewnum
    vs.pings[pingFrom] = *serverStatus
  }
  if pingFrom == vs.currentView.Primary && pingViewNum == vs.currentView.Viewnum {
    vs.primaryAckedCurrentView = true
  }
  //reply to the ping
  reply.View = vs.currentView
  return nil
}

// 
// server Get() RPC handler.
//
func (vs *ViewServer) Get(args *GetArgs, reply *GetReply) error {
  vs.mu.Lock()
  defer vs.mu.Unlock()
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
  vs.mu.Lock()
  defer vs.mu.Unlock()
  //setup
  if vs.currentView.Viewnum == 0 {
    for server, _ := range vs.pings { //serverStatus
      if vs.currentView.Primary == "" {
        vs.currentView.Primary = server
      } 
    }
    if vs.currentView.Primary != "" {
      vs.currentView.Viewnum = 1
      vs.primaryAckedCurrentView = false
    }
  //initial backup setup
  } else if vs.currentView.Viewnum == 1 {
    for server, _ := range vs.pings { //serverStatus
      if vs.currentView.Backup == "" && server != vs.currentView.Primary {
        vs.currentView.Backup = server
      } 
    }
    if vs.currentView.Primary != "" && vs.currentView.Backup != "" {
      vs.currentView.Viewnum = 2
      vs.primaryAckedCurrentView = false
    }
  //everything else
  } else {
    //figure out the state
    now := time.Now()
    oldPrimary := ""
    idleServer := ""
    for server, serverStatus := range vs.pings {
      timePinged := serverStatus.LastPingTime
      lastViewNum := serverStatus.LastViewNum
      currentViewNum := serverStatus.CurrentViewNum
      if now.Sub(timePinged) > PingInterval {
        if life, ok := vs.servers[server]; ok {
          if life > 0 {
            vs.servers[server] = life - 1
          } else {
            //methinks server is dead
            if server == vs.currentView.Primary {
              vs.currentView.Primary = "dead"
              oldPrimary = server
            } else if server == vs.currentView.Backup {
              vs.currentView.Backup = ""
            }
          }
        } else {
          //first time I've heard from this server
          vs.servers[server] = DeadPings
        }
      } else {
        //alive and pinging
        vs.servers[server] = DeadPings
      }
      //primary restart
      if server == vs.currentView.Primary {
        if lastViewNum != currentViewNum && currentViewNum == 0 {
          vs.currentView.Primary = "dead"
          oldPrimary = server
        }
      } else if server != vs.currentView.Primary && server != vs.currentView.Backup && vs.servers[server] > 0 {
        idleServer = server
      }
    }
    //primary is dead, promote backup and then try to replace it with an idleServer
    viewChange := false
    if vs.currentView.Primary == "dead" && vs.currentView.Backup != "" && vs.primaryAckedCurrentView {
      vs.currentView.Primary = vs.currentView.Backup
      if idleServer != "" {
        vs.currentView.Backup = idleServer
      } else {
        vs.currentView.Backup = ""
      }
      vs.currentView.Viewnum++
      viewChange = true
      vs.primaryAckedCurrentView = false
    } else if vs.currentView.Primary == "dead" && vs.currentView.Backup != "" && !vs.primaryAckedCurrentView{
      vs.currentView.Primary = oldPrimary
    }
    //backup is dead, promote available idle server
    if vs.currentView.Primary != "dead" && vs.currentView.Backup == "" {
      if idleServer != "" && idleServer != vs.currentView.Primary {
        vs.currentView.Backup = idleServer
        if !viewChange{
          vs.currentView.Viewnum++
          viewChange = true
          vs.primaryAckedCurrentView = false
        }
      } 
    }
  }

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
  vs.primaryAckedCurrentView = false

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
