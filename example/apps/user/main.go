package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/kieran091/pilot"
	"github.com/kieran091/pilot/discovery/etcd"
	"github.com/kieran091/pilot/example/apps/user/pb/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type UserServer struct {
	user.UnimplementedUserServer
}

func (s *UserServer) CreateUser(ctx context.Context, req *user.CreateUserReq) (*user.CreateUserResp, error) {
	fmt.Printf("email=%s, name=%s, password=%s", req.Email, req.Name, req.Password)
	return &user.CreateUserResp{
		Id: uuid.New().String(),
	}, nil
}

func (s *UserServer) GetUser(_ context.Context, req *user.GetUserReq) (*user.GetUserResp, error) {
	fmt.Println("GetUser called with ID:", req.Id)
	return &user.GetUserResp{
		Id:   req.Id,
		Name: fmt.Sprintf("User-%s", req.Id),
	}, nil
}

func main() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	etcdRegistry, err := etcd.NewRegistry(
		[]string{"127.0.0.1:2379"},
		10*time.Second,
		"test/server",
	)
	if err != nil {
		log.Fatalln(err)
	}

	serviceRegistrar := pilot.NewServiceRegistrar(
		"User",
		":9001",
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

	listen, err := net.Listen("tcp", ":9001")
	if err != nil {
		log.Fatalln(err)
	}

	s := grpc.NewServer()

	user.RegisterUserServer(s, &UserServer{})

	reflection.Register(s)

	log.Println("gRPC server is starting on port 9001...")

	go func() {
		if err := s.Serve(listen); err != nil {
			cancel()
			log.Fatalln("failed to serve:", err)
		}
	}()

	<-sigChan
	cancel()
	log.Println("shutting down gRPC server...")
	s.GracefulStop()
	_ = serviceRegistrar.Deregister(context.Background())
	log.Println("gRPC server stopped.")
}
