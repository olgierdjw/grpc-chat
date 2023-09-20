package server

import (
	"chat/protos"
	"context"
	"errors"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
)

type User struct {
	proto                *protos.User
	sendMessage          chan *protos.DirectMessage
	sendUserStatusUpdate chan *protos.UserStatusChange
}

type GrpcBackend struct {
	onlineUsers map[string]*User
}

func (s *GrpcBackend) Deregister(ctx context.Context, _ *protos.Empty) (*protos.Empty, error) {
	clientId, success := getClientIdFromContext(ctx)
	if success != true {
		return &protos.Empty{}, errors.New("user not found")
	}

	log.Printf("user <%s> deleted\n", s.onlineUsers[clientId].proto.Username)
	userToDelete := s.onlineUsers[clientId]

	for _, user := range s.onlineUsers {
		user.sendUserStatusUpdate <- &protos.UserStatusChange{
			Changed: &protos.User{
				Id:       userToDelete.proto.Id,
				Username: userToDelete.proto.Username,
			},
			Add: false,
		}
	}

	delete(s.onlineUsers, clientId)

	return &protos.Empty{}, nil
}

func NewGrpcImplementation() *GrpcBackend {
	gb := &GrpcBackend{onlineUsers: make(map[string]*User, 20)}
	//gb.Register(context.Background(), &protos.RegisterRequest{
	//	Username: "bot-always available",
	//})

	return gb
}

func (s *GrpcBackend) Register(ctx context.Context, request *protos.RegisterRequest) (*protos.User, error) {
	for _, user := range s.onlineUsers {
		if user.proto.Username == request.Username {
			return nil, errors.New("username is already taken")
		}
	}

	user := &User{
		proto: &protos.User{
			Id:       uuid.NewString(),
			Username: request.Username,
		},
		sendMessage:          make(chan *protos.DirectMessage, 100),
		sendUserStatusUpdate: make(chan *protos.UserStatusChange, 100),
	}

	s.onlineUsers[user.proto.Id] = user

	// create notification
	update := &protos.UserStatusChange{
		Changed: &protos.User{
			Id:       user.proto.Id,
			Username: user.proto.Username,
		},
		Add: true,
	}

	// update users' lists
	for _, otherUsers := range s.onlineUsers {
		if otherUsers.proto.Id == update.Changed.Id {
			continue
		}
		otherUsers.sendUserStatusUpdate <- update
	}

	return user.proto, nil
}

func (s *GrpcBackend) List(context.Context, *protos.Empty) (*protos.UserList, error) {
	allUsers := make([]*protos.User, 0, 20)
	for _, v := range s.onlineUsers {
		allUsers = append(allUsers, v.proto)
	}

	return &protos.UserList{
		Users: allUsers,
	}, nil
}

func getClientIdFromContext(ctx context.Context) (string, bool) {
	clientId, ok := ctx.Value("client-id").(string)
	return clientId, ok
}

func (s *GrpcBackend) SendDirectMessage(ctx context.Context, request *protos.NewMessage) (*protos.DirectMessage, error) {
	// parse all request data
	sender, _ := getClientIdFromContext(ctx)
	receiver := request.ReceiverId
	if _, r := s.onlineUsers[receiver]; !r {
		return nil, errors.New("receiver not found")
	}
	message := request.Message

	if _, ok := s.onlineUsers[receiver]; !ok {
		log.Println("message receiver does not exist")
	}

	log.Printf("[%s]->[%s], message: '%s'\n", s.onlineUsers[sender].proto.Username, s.onlineUsers[receiver].proto.Username, message)
	// forward message
	messageReceiver := s.onlineUsers[receiver]
	newMessage := &protos.DirectMessage{
		SenderId: sender,
		Message:  message,
		Time:     timestamppb.Now(),
	}

	messageReceiver.sendMessage <- newMessage
	return newMessage, nil
}

func (s *GrpcBackend) GetUpdates(_ *protos.SubscriptionRequest, server protos.RegisterUser_GetUpdatesServer) error {
	clientId, success := getClientIdFromContext(server.Context())
	if success != true {
		return errors.New("client id not provided")
	}

	user := s.onlineUsers[clientId]

	// stream all notifications from the buffered channels
	for {
		serverUpdate := &protos.ServerUpdate{}
		select {

		case message := <-user.sendMessage:
			serverUpdate.Content = &protos.ServerUpdate_IncomingMessage{IncomingMessage: message}

		case changeUserList := <-user.sendUserStatusUpdate:
			serverUpdate.Content = &protos.ServerUpdate_UserOnlineStatus{UserOnlineStatus: changeUserList}

			if changeUserList.Changed.Id == clientId {
				log.Printf("user: <%s> will not receive new messages\n", changeUserList.Changed.Username)
				return nil
			}
		}

		server.Send(serverUpdate)
	}
}
