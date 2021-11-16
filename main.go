package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"

	goTime "time"

	pb "github.com/ap/DME2/api"
	col "github.com/ap/DME2/internal/collection"
	"github.com/ap/DME2/internal/time"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

const (
	ip = "127.0.0.1:5001"
)

type Status int32

const (
	Status_RELEASED Status = 0
	Status_WANTED   Status = 1
	Status_HELD     Status = 2
)

type Node struct {
	pb.UnimplementedDmeApiServiceServer
	timestamp time.LamportTimestamp
	status    Status
	responses int
	queue     col.Queue
	members   []string
	lock 	  sync.Mutex
}

func main() {
	clients := strings.Split(os.Getenv("CLIENTS"), ",")

	node := &Node{
		queue:   col.NewQueue(),
		members: clients,
		lock: 	 sync.Mutex{},
	}

	node.StartServer()
}

func getClientIpAddress(c context.Context) string {
	p, _ := peer.FromContext(c)

	return p.Addr.String()
}

func (n *Node) StartServer() {
	lis, err := net.Listen("tcp", ip)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterDmeApiServiceServer(s, n)
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

// Send Req message
func (n *Node) GetLock(in *pb.EmptyWithTime) error {
	// We cannot ask others if we already have the lock / have asked
	status := n.GetStatus()
	if status == Status_WANTED || status == Status_HELD {
		return nil
	}

	n.SetStatus(Status_WANTED)
	n.responses = len(n.members)

	for _, member := range n.members {
		n.timestamp.Increment()

		// Set up a connection to the server.
		conn, err := grpc.Dial(member, grpc.WithInsecure())
		if err != nil {
			return err
		}

		defer conn.Close()
		c := pb.NewDmeApiServiceClient(conn)

		ctx, cancel := context.WithTimeout(context.Background(), goTime.Second)
		defer cancel()

		msg, err := c.Req(ctx, &pb.RequestMessage{
			Time: n.timestamp.GetTime(),
		})
		if err != nil {
			return err
		}

		n.timestamp.Sync(msg.GetTime())
	}

	return nil
}

// Send Res message
func (n *Node) SendRes(target string) {
	conn, err := grpc.Dial(target, grpc.WithInsecure())
	if err != nil {
		return
	}

	defer conn.Close()
	c := pb.NewDmeApiServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), goTime.Second)
	defer cancel()

	n.timestamp.Increment()
	msg, err := c.Res(ctx, &pb.EmptyWithTime{
		Time: n.timestamp.GetTime(),
	})
	if err != nil {
		return
	}

	n.timestamp.Sync(msg.GetTime())
}

// Handle incoming Req message
func (n *Node) Req(ctx context.Context, in *pb.RequestMessage) (*pb.EmptyWithTime, error) {

	callerIp := getClientIpAddress(ctx)
	status := n.GetStatus()
	if status == Status_HELD || (status == Status_WANTED && n.timestamp.GetTime() < in.GetTime()) {
		n.queue.Enqueue(callerIp)
	} else {
		n.SendRes(callerIp)
	}

	n.timestamp.Sync(in.GetTime())
	n.timestamp.Increment()

	return &pb.EmptyWithTime{Time: n.timestamp.GetTime()}, nil
}

// Handle incoming Res
// TODO: Implement handling of release here
func (n *Node) Res(ctx context.Context, in *pb.EmptyWithTime) (*pb.EmptyWithTime, error) {
	n.timestamp.Sync(in.GetTime())
	n.timestamp.Increment()

	// Decrease response count
	n.responses -= 1

	// If all nodes have responded, we have achieved lock
	if n.responses == 0 {
		n.SetStatus(Status_HELD)
		go n.WriteToFile()
	}

	return &pb.EmptyWithTime{Time: n.timestamp.GetTime()}, nil
}

func (n *Node) WriteToFile(){
	file, err := os.OpenFile("file.txt",os.O_APPEND|os.O_CREATE|os.O_WRONLY,0666)
	if err!=nil{
		log.Fatal(err)
	}

	file.Write([]byte(n.timestamp.GetDisplayableContent()+"\n"))

	file.Close()

	n.Exit()
}

func (n *Node) SetStatus(s Status) {
	n.lock.Lock()
	defer n.lock.Unlock()

	n.status = s
}

func (n *Node) GetStatus() Status {
	n.lock.Lock()
	defer n.lock.Unlock()

	return n.status
}

/*
* Exits the 'HELD' mode and releases the distributed lock, by telling other nodes in the network
*/
func (n *Node) Exit() {
	if n.GetStatus() != Status_HELD {
		return // only when we are in status 'HELD' will this function be executed...
	}

	for !n.queue.IsEmpty() {
		addr := fmt.Sprintf("%v", n.queue.Dequeue())
		n.SendRes(addr)
	}

	n.SetStatus(Status_RELEASED)
}
