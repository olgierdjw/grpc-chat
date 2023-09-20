package client

import (
	"chat/protos"
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"log"
)

type ChatService interface {
	GetUserId() (id string)
	GetUsername() (username string)
	GetUserDetails(clientId string) string
	AllUsers() []User
	Register(username string) error
	SendMessage(receiverId, message string) DbMessage
	ReadMessages(clientId string) []DbMessage
	SendNotification(clientId string)
	NewMessageNotification() <-chan protos.DirectMessage
	OnlineUserChangedNotification() <-chan bool
	CanChatWith(clientId string) bool
}

type ChatServiceImplementation struct {
	database LocalDatabase

	user               *protos.User
	registerUserClient protos.RegisterUserClient

	newMessages       chan protos.DirectMessage
	userStatusUpdated chan bool

	appStopRequest chan<- bool
}

func NewChatServiceImplementation(appEndRequest chan<- bool, database LocalDatabase) *ChatServiceImplementation {
	return &ChatServiceImplementation{appStopRequest: appEndRequest, database: database}
}

func (s *ChatServiceImplementation) InitGrpcClient(conn *grpc.ClientConn) {
	s.registerUserClient = protos.NewRegisterUserClient(conn)
}

func (s *ChatServiceImplementation) GetUserId() (id string) {
	if s.user == nil {
		log.Fatalln("user object not found")
	}
	return s.user.Id
}

func (s *ChatServiceImplementation) GetUserDetails(clientId string) string {
	user := s.database.GetUser(clientId)
	return user.username
}

func (s *ChatServiceImplementation) GetUsername() (username string) {
	if s.user == nil {
		log.Fatalln("user object not found")
	}
	return s.user.Username
}

func (s *ChatServiceImplementation) NewMessageNotification() <-chan protos.DirectMessage {
	return s.newMessages
}
func (s *ChatServiceImplementation) OnlineUserChangedNotification() <-chan bool {
	return s.userStatusUpdated
}

func (s *ChatServiceImplementation) CanChatWith(clientId string) bool {
	return s.database.UserOnline(clientId)
}

func (s *ChatServiceImplementation) UnaryClientInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	log.Println("UNARY INTERCEPTOR", "method:", method)
	userRegistered := s.user != nil
	if userRegistered {
		ctx = metadata.AppendToOutgoingContext(ctx, "client-id", s.user.Id)
	}
	err := invoker(ctx, method, req, reply, cc, opts...)
	return err
}

func (s *ChatServiceImplementation) StreamClientInterceptor(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	log.Println("STREAM INTERCEPTOR", "method:", method)
	stream, err := streamer(metadata.AppendToOutgoingContext(ctx, "client-id", s.user.Id), desc, cc, method, opts...)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (s *ChatServiceImplementation) AllUsers() []User {
	return s.database.ListAllUsers()
}

func (s *ChatServiceImplementation) Register(username string) error {
	newUser, err := s.registerUserClient.Register(context.Background(), &protos.RegisterRequest{
		Username: username,
	})

	if err != nil {
		log.Printf("registration failed: %s\n", err.Error())
		return err
	}

	s.user = newUser
	s.subscribe()
	log.Printf("user online, id: %s\n", newUser.Id)

	// register all users to the db
	list, err := s.registerUserClient.List(context.Background(), &protos.Empty{})
	for _, u := range list.Users {
		s.database.AddUser(&protos.User{
			Id:       u.Id,
			Username: u.Username,
		})
	}
	return nil
}

func (s *ChatServiceImplementation) subscribe() {
	if s.newMessages != nil && s.userStatusUpdated != nil {
		log.Fatal("grpc stream already active")
	}
	log.Println("Loading update channels...")

	s.newMessages = make(chan protos.DirectMessage, 100)
	s.userStatusUpdated = make(chan bool, 100)

	stream, err := s.registerUserClient.GetUpdates(context.Background(), &protos.SubscriptionRequest{})
	if err != nil {
		log.Fatal("subscription request failed:", err.Error())
	}

	go func() {
		for {
			update, err := stream.Recv()
			if err != nil {
				log.Println("connection lost")
				s.appStopRequest <- true
				return
			}

			switch updateContent := update.Content.(type) {
			case *protos.ServerUpdate_IncomingMessage:
				im := updateContent.IncomingMessage
				s.database.SaveIncomingMessage(*im)
				s.newMessages <- *im
				log.Println("new message!")
			case *protos.ServerUpdate_UserOnlineStatus:
				listUserChange := updateContent.UserOnlineStatus
				if listUserChange.Add {
					s.database.AddUser(&protos.User{
						Id:       listUserChange.Changed.Id,
						Username: listUserChange.Changed.Username,
					})
				} else {
					s.database.DeleteUser(listUserChange.Changed.Id)
				}
				s.userStatusUpdated <- true
			default:
				log.Printf("Received unknown update type")
			}
		}
	}()

}

func (s *ChatServiceImplementation) getAllUsers() *protos.UserList {
	allUsers, err := s.registerUserClient.List(context.Background(), &protos.Empty{})
	if err != nil {
		log.Println("users loading failed:", err.Error())
		return &protos.UserList{
			Users: make([]*protos.User, 0),
		}
	}
	return allUsers
}

func (s *ChatServiceImplementation) SendMessage(receiverId, message string) DbMessage {
	dm := &protos.NewMessage{
		ReceiverId: receiverId,
		Message:    message,
	}
	mess, err := s.registerUserClient.SendDirectMessage(context.Background(), dm)
	if err != nil {
		fmt.Println("message:", err.Error())
	}

	s.database.SaveOutgoingMessage(receiverId, message)
	return DbMessage{
		incoming: false,
		text:     mess.Message,
		time:     mess.Time,
	}
}

func (s *ChatServiceImplementation) ReadMessages(clientId string) []DbMessage {
	s.database.RemoveNotification(clientId)
	return s.database.GetMessages(clientId)
}

func (s *ChatServiceImplementation) SendNotification(clientId string) {
	s.database.AddNewMessageNotification(clientId)
}

func (s *ChatServiceImplementation) Unregister() {
	_, err := s.registerUserClient.Deregister(context.Background(), &protos.Empty{})
	if err != nil {
		log.Fatalln("registration failed:", err)
	}
}
