package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"

	goTime "time"

	pb "github.com/ap/DME2/api"
	"github.com/ap/DME2/internal/time"
	col "github.com/ap/DME2/internal/collection"
	"github.com/hashicorp/serf/serf"
	"google.golang.org/grpc"
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
	client    pb.DmeApiServiceClient
	timestamp time.LamportTimestamp
	status    Status
	processId int
	cluster   *serf.Serf
	responses int
	queue     col.Queue
}

func main() {
	clusterAddr := flag.String("clusterAddress", "localhost", "")
	flag.Parse()
	node := &Node{
		processId: os.Getpid(),
		queue:     col.NewQueue(),
	}

	node.StartServer()
	node.StartCluster(clusterAddr)
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

func (n *Node) StartCluster(clusterAddr *string) error {
	conf := serf.DefaultConfig()
	conf.Init()
	conf.MemberlistConfig.AdvertiseAddr = ip

	cluster, err := serf.Create(conf)
	if err != nil {
		return err
	}

	_, err = cluster.Join([]string{*clusterAddr}, true)
	if err != nil {
		log.Printf("Couldn't join cluster, starting own: %v\n", err)
	}

	n.cluster = cluster
	return nil
}

// Send Req message
func (n *Node) GetLock(in *pb.EmptyWithTime) error {
	for _, member := range n.cluster.Members() {
		n.timestamp.Increment()

		// Set up a connection to the server.
		conn, err := grpc.Dial(member.Addr, grpc.WithInsecure())
		if err != nil {
			return err
		}

		defer conn.Close()
		c := pb.NewDmeApiServiceClient(conn)

		// Contact the server and print out its response.
		ctx, cancel := context.WithTimeout(context.Background(), goTime.Second)
		defer cancel()

		msg, err := c.Req(ctx, &pb.RequestMessage{
			Time:      n.timestamp.GetTime(),
			ProcessId: int32(n.processId),
		})
		if err != nil {
			return err
		}

		n.timestamp.Sync(msg.GetTime())
	}

	return nil
}

// Send Res message
// TODO: Implement me
func (n *Node) SendRes(target string) {

}

// Handle incoming Req message
// TODO: Implement handling of request here
func (n *Node) Req(ctx context.Context, in *pb.RequestMessage) (*pb.EmptyWithTime, error) {
	if n.status == Status_HELD || (n.status == Status_WANTED && n.timestamp.GetTime() < in.GetTime()) {
		// TODO: Send request to queue
	} else {
		// TODO: Send response
	}

	n.timestamp.Sync(in.GetTime())
	n.timestamp.Increment()

	return &pb.EmptyWithTime{Time: n.timestamp.GetTime()}, nil
}

// Handle incoming Res
// TODO: Implement handling of release here
func (n *Node) Res(ctx context.Context, in *pb.EmptyWithTime) (*pb.EmptyWithTime, error) {
	n.timestamp.Increment()

	return &pb.EmptyWithTime{Time: n.timestamp.GetTime()}, nil
}

/*
* Exits the 'HELD' mode and releases the distributed lock, by telling other nodes in the network
*/
func (n *Node) Exit() {
	if n.status !=  Status_HELD{
		return // only when we are in status 'HELD' will this function be executed...
	}

	for !n.queue.IsEmpty() {
		addr := string(n.queue.Pop())
	}
}
