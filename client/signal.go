package client

import "log"

type SignalState[T any] interface {
	getUpdateChannel() chan T
	pushValue(T)
	getCurrentValue() T
}

type signalImplementation[T any] struct {
	value        T
	observerList []chan T
}

func (s *signalImplementation[T]) getUpdateChannel() chan T {
	newSubscriberChannel := make(chan T, 1)
	newSubscriberChannel <- s.getCurrentValue()
	s.observerList = append(s.observerList, newSubscriberChannel)
	return newSubscriberChannel
}

func (s *signalImplementation[T]) getCurrentValue() T {
	return s.value
}

func (s *signalImplementation[T]) pushValue(value T) {
	s.value = value
	for _, channel := range s.observerList {
		select {
		case channel <- value:
			log.Printf("value <%v> send to channel <%v>\n", value, channel)
		default:
			log.Fatalf("value <%v> has not beed sent. channel blocked <%v>\n", value, channel)
		}
	}
}
