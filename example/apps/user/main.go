package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/kieran091/pilot"
	"github.com/kieran091/pilot/example/apps/user/pb/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type UserServer struct {
	user.UnimplementedUserServer
}

func (s *UserServer) GetUser(ctx context.Context, req *user.GetUserReq) (*user.GetUserResp, error) {
	return &user.GetUserResp{
		Id:   req.Id,
		Name: fmt.Sprintf("User-%s", req.Id),
	}, nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	etcdRegistry, err := pilot.NewEtcdRegistry(
		[]string{"127.0.0.1:2379"},
		10*time.Second,
		"test/server",
	)
	if err != nil {
		log.Fatalln(err)
	}

	serviceRegistrar := pilot.NewServiceRegistrar(
		"User",
		":9000",
		etcdRegistry,
	)
	err = serviceRegistrar.Register(
		ctx,
		pilot.WithFile("user.proto"),
		pilot.WithProtoPath("apps/user"),
		pilot.WithProtoPath("third_party"),
		pilot.WithProtoPath("."),
	)
	if err != nil {
		log.Fatalln(err)
	}

	listen, err := net.Listen("tcp", ":9000")
	if err != nil {
		log.Fatalln(err)
	}

	s := grpc.NewServer()

	user.RegisterUserServer(s, &UserServer{})

	reflection.Register(s)

	log.Println("gRPC server is starting on port 9000...")

	if err := s.Serve(listen); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
