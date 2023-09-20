package client

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"log"
	"time"
)

type TerminalApp struct {
	data        ChatService
	exitRequest chan bool

	app          *tview.Application
	pages        *tview.Pages
	chatTextView *tview.TextView
	userList     *tview.List
	focusManager Iterator[*tview.Box]

	selectedUserId SignalState[string]
}

func NewTerminalApplication(dataLayer ChatService, focusManager ActiveBoxManager, exitRequest chan bool) *tview.Application {
	terminal := TerminalApp{
		app:            tview.NewApplication(),
		pages:          tview.NewPages(),
		focusManager:   &focusManager,
		data:           dataLayer,
		selectedUserId: new(signalImplementation[string]),
		chatTextView:   tview.NewTextView(),
		userList:       tview.NewList(),
		exitRequest:    exitRequest,
	}

	// login page
	terminal.pages.AddPage("startup", terminal.loginPage(), true, true)
	terminal.app.SetRoot(terminal.pages, true).SetFocus(terminal.pages)

	return terminal.app
}

func (app *TerminalApp) activateRouting() {
	terminal := app.app
	terminal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			app.focusNextElement()
		}
		return event
	})
}

func (app *TerminalApp) loginPage() *tview.Frame {
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.SetBorderPadding(0, 0, 3, 3)

	startButton := tview.NewButton("hi")
	startButton.SetDisabled(true)
	startButton.SetSelectedFunc(func() {
		app.pages.AddAndSwitchToPage("dashboard", app.dashboardPage(), true)
	})

	usernameInput := tview.NewInputField()
	usernameInput.SetLabel("Username:")
	usernameInput.SetBorderPadding(0, 0, 2, 2)

	errorText := tview.NewTextView()
	errorText.SetTextAlign(tview.AlignCenter).SetTextColor(tcell.ColorOrangeRed)

	usernameInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			if usernameInput.GetText() == "" {
				return
			}
			username := usernameInput.GetText()
			err := app.data.Register(username)
			if err != nil {
				log.Println(err)
				startButton.SetLabel("not available")
				errorText.SetText(err.Error())
				startButton.SetDisabled(true)
			} else {
				errorText.SetText("")
				startButton.SetDisabled(false)
				startButton.SetLabel("enter to start")
				app.app.SetFocus(startButton)
			}
		}
	})

	flex.AddItem(usernameInput, 2, 1, true)
	flex.AddItem(startButton, 1, 1, true)
	flex.AddItem(errorText, 1, 1, true)

	frame := tview.NewFrame(flex).
		SetBorders(2, 2, 2, 2, 4, 4).
		AddText("Simple grpc server chat with terminal client", true, tview.AlignLeft, tcell.ColorWhite).
		AddText("github.com/olgierdjw", true, tview.AlignCenter, tcell.ColorGreen).
		AddText("Go. Protobuf. gRPC. tview.", true, tview.AlignRight, tcell.ColorWhite)

	return frame
}

func (app *TerminalApp) focusNextElement() {
	app.app.SetFocus(app.focusManager.next())
}

func (app *TerminalApp) dashboardPage() *tview.Flex {
	app.activateRouting()

	left := app.createOnlineUsersPanel()
	app.focusManager.addItem(left.Box, 10)

	center := app.createCenterFlex()

	right := tview.NewBox().SetBorder(true).SetTitle(" Right ")
	app.focusManager.addItem(right, 30)

	return tview.NewFlex().
		AddItem(left, 30, 3, true).
		AddItem(center, 0, 5, false).
		AddItem(right, 0, 2, false)
}

func (app *TerminalApp) showUpdatedList() {

	// empty list
	app.userList.Clear()

	freeIndex := 1
	for _, user := range app.data.AllUsers() {

		// user description
		id := user.id
		description := fmt.Sprintf("%s ", id[:5])
		if user.notification {
			description += "[red::bl](NEW MESSAGE)[-:-:-:-]"
		}

		// insertion order
		var index int
		if user.id == app.data.GetUserId() {
			index = 0
			description = "me"
		} else {
			index = freeIndex
			freeIndex++
		}

		// append
		app.userList.InsertItem(index, user.username, description, rune('a'+index), func() {
			log.Printf("list option selected, username <%s>\n", user.username)
			app.selectedUserId.pushValue(id)
			app.focusNextElement()
		})

	}
	log.Println("user list updated")
}

func (app *TerminalApp) createOnlineUsersPanel() *tview.List {
	list := app.userList
	list.SetBorder(true)
	list.SetTitle(" Online users ")

	list.SetSelectedFunc(func(index int, username string, shortId string, r rune) {
		log.Println("SELECTED LIST ITEM", username)
	})

	list.SetSelectedBackgroundColor(tcell.ColorLightCoral)

	app.showUpdatedList()

	// first selected user
	app.selectedUserId.pushValue(app.data.GetUserId())

	// ignore Tab key
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			return nil
		}
		return event
	})

	// handle list updates
	go func() {
		reloadUsers := app.data.OnlineUserChangedNotification()
		for {
			<-reloadUsers

			// get rid of focus on empty user
			if !app.data.CanChatWith(app.selectedUserId.getCurrentValue()) {
				log.Println("selected user is offline")
				app.selectedUserId.pushValue(app.data.GetUserId())
			}

			// draw updated list
			app.app.QueueUpdateDraw(func() {
				app.showUpdatedList()
			})
		}
	}()

	return list
}

func (app *TerminalApp) createCenterFlex() *tview.Flex {
	messages := app.createMessagePanel()
	app.focusManager.addItem(messages.Box, 22)

	chat := app.createNewMessagePanel()
	app.focusManager.addItem(chat.Box, 21)

	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(app.createInfoPanel(), 4, 1, false).
		AddItem(messages, 0, 5, true).
		AddItem(chat, 3, 1, true)
	return flex
}

func (app *TerminalApp) createInfoPanel() *tview.TextView {
	infoPanel := tview.NewTextView()
	infoPanel.SetBorder(true)
	fmt.Fprintf(infoPanel, "username: %s, id: %s\n", app.data.GetUsername(), app.data.GetUserId()[:5])
	fmt.Fprint(infoPanel, "use TAB to navigate")
	return infoPanel
}

func timeFromTimeout(messageTime time.Time) string {
	currentTime := messageTime.In(time.Now().Location())
	return currentTime.Format("15:04")
}

func printMessage(chat *tview.TextView, printableMessage *DbMessage) {
	var prefix string

	if printableMessage.incoming {
		prefix = "[white][>>]"
	} else {
		prefix = "[grey][<<][white]"
	}

	hhss := timeFromTimeout(printableMessage.time.AsTime())

	fmt.Fprint(chat, hhss+" "+prefix+" "+printableMessage.text+"\n")
}

func (app *TerminalApp) createMessagePanel() *tview.TextView {
	textView := app.chatTextView
	textView.SetDynamicColors(true)
	textView.SetTitle("Chat")
	textView.SetBorder(true)

	// display messages in the console even when it's not the active window
	textView.SetChangedFunc(func() {
		app.app.Draw()
		textView.ScrollToEnd()
	})

	fmt.Fprintln(textView, "loading...")

	go func() {
		selectedUserChannel := app.selectedUserId.getUpdateChannel()
		for {
			select {
			// show all messages on chat user change
			case currentChatUser := <-selectedUserChannel:
				textView.Clear()
				textView.SetTitle(" Chat with " + app.data.GetUserDetails(currentChatUser) + " ")
				previousMessages := app.data.ReadMessages(currentChatUser)
				for _, mes := range previousMessages {
					printMessage(textView, &mes)
				}
				app.app.QueueUpdateDraw(func() {
					app.showUpdatedList()
				})

			// append all new incoming messages
			case newMessage := <-app.data.NewMessageNotification():
				currentlyPrintableMessage := newMessage.SenderId == app.selectedUserId.getCurrentValue()
				if currentlyPrintableMessage {
					printMessage(textView, &DbMessage{
						incoming: true,
						text:     newMessage.Message,
						time:     newMessage.Time,
					})
				} else {
					app.data.SendNotification(newMessage.SenderId)
					app.app.QueueUpdateDraw(func() {
						app.showUpdatedList()
					})
				}
			}

		}
	}()

	return textView
}

func (app *TerminalApp) createNewMessagePanel() *tview.InputField {
	messageInput := tview.NewInputField()
	messageInput.SetLabel("Message:")
	messageInput.SetLabelColor(tcell.ColorWhite)
	messageInput.SetBorder(true)

	messageInput.SetFocusFunc(func() {
		messageInput.SetLabelColor(tcell.ColorRed)
	})

	messageInput.SetBlurFunc(func() {
		messageInput.SetLabelColor(tcell.ColorWhite)
	})

	messageInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			if messageInput.GetText() == "" {
				return
			}
			sendingTo := app.selectedUserId.getCurrentValue()
			messageText := messageInput.GetText()
			printable := app.data.SendMessage(sendingTo, messageText)
			printMessage(app.chatTextView, &printable)
			messageInput.SetText("")
		}
	})

	return messageInput
}

func (app *TerminalApp) serverConnectionFailed() {
	app.app.Stop()
}
