package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"

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
	status    Status
	responses int
	queue     col.Queue
	members   []string
}

func main() {
	clients := strings.Split(os.Getenv("CLIENTS"), ",")

	node := &Node{
		queue:   col.NewQueue(),
		members: clients,
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
	if n.status == Status_WANTED || n.status == Status_HELD {
		return nil
	}

	n.status = Status_WANTED
	n.responses = len(n.members)

	for _, member := range n.members {
		n.timestamp.Increment()

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
	log.Printf("Sending response\n")

	url := target + port
	log.Printf("Send res to: %s\n", url)
	conn, err := grpc.Dial(url, grpc.WithInsecure())
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

	log.Printf("Handling request from %s, current status: %d, local time: %d, in time: %d\n", callerIp, n.status, n.timestamp.GetTime(), in.GetTime())

	if n.status == Status_HELD || (n.status == Status_WANTED && n.timestamp.GetTime() < in.GetTime()) {
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
	log.Printf("Handling response from %s\n", getClientIpAddress(ctx))
	n.timestamp.Sync(in.GetTime())
	n.timestamp.Increment()

	// Decrease response count
	n.responses -= 1
	log.Printf("Reponses: %d\n", n.responses)

	// If all nodes have responded, we have achieved lock
	if n.responses == 0 {
		log.Printf("Achieved lock\n")

		n.status = Status_HELD
		go n.WriteToFile()
	}

	return &pb.EmptyWithTime{Time: n.timestamp.GetTime()}, nil
}

func (n *Node) WriteToFile() {
	log.Printf("Writing value to file\n")
	file, err := os.OpenFile("/tmp/exercise2/data/file.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}

	file.Write([]byte(n.timestamp.GetDisplayableContent() + "\n"))

	log.Printf("Closing file and exiting\n")
	file.Close()

	n.Exit()
}

/*
* Exits the 'HELD' mode and releases the distributed lock, by telling other nodes in the network
 */
func (n *Node) Exit() {
	log.Printf("Exiting\n")
	if n.status != Status_HELD {
		return // only when we are in status 'HELD' will this function be executed...
	}

	log.Printf("Sending exit responses\n")
	for !n.queue.IsEmpty() {
		addr := fmt.Sprintf("%v", n.queue.Dequeue())
		n.SendRes(addr)
	}

	n.status = Status_RELEASED
}
