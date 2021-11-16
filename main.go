package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
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
	port = ":5001"
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
	lockTs    int32
	status    Status
	responses int
	queue     col.Queue
	members   []string
	lock      sync.Mutex
}

func main() {
	clients := strings.Split(os.Getenv("CLIENTS"), ",")

	node := &Node{
		queue:   col.NewQueue(),
		members: clients,
		lock:    sync.Mutex{},
	}

	go node.Random()
	node.StartServer()
}

func getClientIpAddress(c context.Context) string {
	p, _ := peer.FromContext(c)

	return p.Addr.String()
}

func (n *Node) StartServer() {
	lis, err := net.Listen("tcp", port)
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

func (n *Node) Random() {
	r := rand.New(rand.NewSource(goTime.Now().UnixNano()))
	for {
		goTime.Sleep(goTime.Duration(r.Intn(10)) * goTime.Second)
		if n.status == Status_RELEASED {
			n.GetLock()
		}
	}
}

// Send Req message
func (n *Node) GetLock() error {
	log.Printf("Getting lock\n")
	// We cannot ask others if we already have the lock / have asked
	status := n.GetStatus()
	if status == Status_WANTED || status == Status_HELD {
		return nil
	}

	n.SetStatus(Status_WANTED)
	n.responses = len(n.members)
	n.timestamp.Increment()
	n.lockTs = n.timestamp.GetTime()

	for _, member := range n.members {
		url := member + port
		log.Printf("Send req to: %s\n", url)
		// Set up a connection to the server.
		conn, err := grpc.Dial(url, grpc.WithInsecure())
		if err != nil {
			return err
		}

		defer conn.Close()
		c := pb.NewDmeApiServiceClient(conn)

		ctx, cancel := context.WithTimeout(context.Background(), goTime.Second)
		defer cancel()

		_, err = c.Req(ctx, &pb.RequestMessage{
			Time: n.GetTs(),
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// Send Res message
func (n *Node) SendRes(target string) {
	// Work around the fact that a connection uses a random part
	url := strings.Split(target, ":")[0] + port
	log.Printf("Send res to: %s\n", url)

	conn, err := grpc.Dial(url, grpc.WithInsecure())
	if err != nil {
		return
	}

	defer conn.Close()
	c := pb.NewDmeApiServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), goTime.Second)
	defer cancel()

	_, err = c.Res(ctx, &pb.Empty{})
	if err != nil {
		log.Printf("Res errored: %v\n", err)
		return
	}
}

// Handle incoming Req message
func (n *Node) Req(ctx context.Context, in *pb.RequestMessage) (*pb.Empty, error) {
	callerIp := getClientIpAddress(ctx)

	log.Printf("Handling request from %s, current status: %d, local time: %d, in time: %d\n", callerIp, n.status, n.GetTs(), in.GetTime())

	status := n.GetStatus()
	if status == Status_HELD || (status == Status_WANTED && n.GetTs() < in.GetTime()) {
		n.queue.Enqueue(callerIp)
	} else {
		n.SendRes(callerIp)
	}

	n.timestamp.Sync(in.GetTime())

	return &pb.Empty{}, nil
}

// Handle incoming Res
func (n *Node) Res(ctx context.Context, in *pb.Empty) (*pb.Empty, error) {
	log.Printf("Handling response from %s\n", getClientIpAddress(ctx))

	// Decrease response count
	n.responses -= 1

	// If all nodes have responded, we have achieved lock
	if n.responses == 0 {
		log.Printf("Achieved lock\n")

		n.SetStatus(Status_HELD)
		go n.WriteToFile()
	}

	return &pb.Empty{}, nil
}

func (n *Node) WriteToFile() {
	log.Printf("Writing value to file\n")
	file, err := os.OpenFile("/tmp/exercise2/data/file.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}

	name, err := os.Hostname()
	if err != nil {
		name = "NoHostname"
	}

	file.Write([]byte(fmt.Sprintf("%s: %s\n", name, n.timestamp.GetDisplayableContent())))

	log.Printf("Closing file and exiting\n")
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
	log.Printf("Exiting\n")

	if n.GetStatus() != Status_HELD {
		return // only when we are in status 'HELD' will this function be executed...
	}

	log.Printf("Sending exit responses\n")
	for !n.queue.IsEmpty() {
		addr := n.queue.Dequeue().(string)
		n.SendRes(addr)
	}

	n.SetStatus(Status_RELEASED)
}

func (n *Node) GetTs() int32 {
	status := n.GetStatus()

	if status == Status_WANTED || status == Status_HELD {
		return n.lockTs
	} else {
		return n.timestamp.GetTime()
	}
}
