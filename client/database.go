package client

import (
	"chat/protos"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	"sync"
)

type LocalDatabase interface {
	AddUser(user *protos.User)
	SaveIncomingMessage(mes protos.DirectMessage) DbMessage
	SaveOutgoingMessage(clientId string, text string)
	AddNewMessageNotification(string)
	ListAllUsers() []User
	GetUser(clientId string) User
	DeleteUser(clientId string)
	GetMessages(clientId string) []DbMessage
	RemoveNotification(clientId string)
	UserOnline(clientId string) bool
}

type DbMessage struct {
	incoming bool
	text     string
	time     *timestamppb.Timestamp
}

type User struct {
	id           string
	username     string
	notification bool
}

type UserDb struct {
	User
	messages []DbMessage
}

type InMemoryChatDatabase struct {
	sync.RWMutex
	users map[string]*UserDb
}

func (db *InMemoryChatDatabase) UserOnline(clientId string) bool {
	db.RLock()
	defer db.RUnlock()
	_, ok := db.users[clientId]
	return ok
}

func (db *InMemoryChatDatabase) RemoveNotification(clientId string) {
	db.Lock()
	defer db.Unlock()
	user, ok := db.users[clientId]
	if !ok {
		return
	}
	user.notification = false
	log.Println("user", user.username, "new message notification removed")
}

func NewInMemoryChatDatabase() *InMemoryChatDatabase {
	return &InMemoryChatDatabase{users: make(map[string]*UserDb)}
}

func (db *InMemoryChatDatabase) AddUser(user *protos.User) {
	db.Lock()
	defer db.Unlock()
	db.users[user.Id] = &UserDb{
		User: User{
			id:           user.Id,
			username:     user.Username,
			notification: false,
		},
		messages: make([]DbMessage, 0, 15),
	}
	log.Printf("user <%s> added to the db\n", user.Username)
}

func (db *InMemoryChatDatabase) DeleteUser(clientId string) {
	db.Lock()
	defer db.Unlock()
	username := db.users[clientId].username
	delete(db.users, clientId)
	log.Printf("local data about user <%s> deleted\n", username)
}

func (db *InMemoryChatDatabase) ListAllUsers() []User {
	db.RLock()
	defer db.RUnlock()
	userList := make([]User, 0, 20)
	for i := range db.users {
		u := db.users[i].User
		userList = append(userList, u)
	}
	return userList
}

func (db *InMemoryChatDatabase) GetUser(clientId string) User {
	db.RLock()
	defer db.RUnlock()
	if online := db.UserOnline(clientId); !online {
		return User{}
	}
	return db.users[clientId].User
}

func (db *InMemoryChatDatabase) AddNewMessageNotification(clientId string) {
	db.Lock()
	defer db.Unlock()
	db.users[clientId].notification = true
	log.Printf("notification about message from <%s> added\n", db.users[clientId].username)
}

func (db *InMemoryChatDatabase) SaveIncomingMessage(mes protos.DirectMessage) DbMessage {
	messageFrom := mes.SenderId
	newMessageDbObject := DbMessage{
		incoming: true,
		text:     mes.Message,
		time:     mes.Time,
	}
	db.Lock()
	defer db.Unlock()
	db.users[messageFrom].messages = append(db.users[messageFrom].messages, newMessageDbObject)
	log.Printf("saved incoming message '%s' from <%s>\n", mes.Message, db.users[mes.SenderId].username)
	return newMessageDbObject
}

func (db *InMemoryChatDatabase) SaveOutgoingMessage(clientId string, text string) {
	message := DbMessage{
		incoming: false,
		text:     text,
		time:     timestamppb.Now(),
	}

	db.Lock()
	defer db.Unlock()

	if db.users[clientId] == nil {
		log.Fatalln("could not save outgoing message: client not found")
	}
	db.users[clientId].messages = append(db.users[clientId].messages, message)
	log.Println("sent message saved in the db")

}

func (db *InMemoryChatDatabase) GetMessages(clientId string) []DbMessage {
	db.Lock()
	defer db.Unlock()
	allMessages, online := db.users[clientId]
	if !online {
		return make([]DbMessage, 0)
	}
	log.Printf("loaded message history with <%s>\n", clientId)
	return allMessages.messages
}
