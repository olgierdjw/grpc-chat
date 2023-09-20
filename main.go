package main

import (
	"chat/client"
	"chat/protos"
	"chat/server"
	"flag"
	"fmt"
	"github.com/rivo/tview"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"time"
)

func main() {

	var serverMode bool
	flag.BoolVar(&serverMode, "server", false, "start a server")
	flag.Parse()

	// logger
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	fileName := fmt.Sprintf("logfile-%s.txt", timestamp[len(timestamp)-5:])
	logFile, err := os.Create(fileName)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(logFile)
	defer logFile.Close()

	if serverMode {
		serverStart()
	} else {
		clientStart()

	}
}

func serverStart() {
	log.SetOutput(os.Stdout)

	implementedGrpc := server.NewGrpcImplementation()

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(implementedGrpc.UnaryServerInterceptor),
		grpc.StreamInterceptor(implementedGrpc.StreamServerInterceptor),
	)

	protos.RegisterRegisterUserServer(grpcServer, implementedGrpc)

	net, err := net.Listen("tcp", ":8898")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Grpc server started!")
	err = grpcServer.Serve(net)
	if err != nil {
		log.Fatal(err)
	}

}

func clientStart() {
	database := client.NewInMemoryChatDatabase()
	appExit := make(chan bool, 1)
	service := client.NewChatServiceImplementation(appExit, database)

	conn, err := grpc.Dial(":8898", grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(service.UnaryClientInterceptor),
		grpc.WithStreamInterceptor(service.StreamClientInterceptor),
	)
	if err != nil {
		log.Fatalf("did not connect: %s", err)
	}
	defer conn.Close()

	service.InitGrpcClient(conn)

	var terminalApplication *tview.Application

	activeBoxManager := client.ActiveBoxManager{}
	terminalApplication = client.NewTerminalApplication(service, activeBoxManager, appExit)
	err = terminalApplication.Run()
	if err != nil {
		return
	}
	service.Unregister()
	fmt.Println("Bye!")
}

func serverExists() bool {
	conn, err := net.Dial("tcp", "localhost:8898")
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}
