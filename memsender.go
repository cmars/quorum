package quorum

import (
	"log"

	"gopkg.in/errgo.v1"
)

type memSender struct {
	mboxes map[string]memMbox
}

type memMbox chan Ballot

// NewMemSender returns a new in-memory implementation of Sender.
func NewMemSender(names ...string) Sender {
	sender := &memSender{
		mboxes: map[string]memMbox{},
	}
	return sender
}

// ValidateRecipient implements Sender by verifying that the recipient is
// registered with the sender.
func (s *memSender) ValidateRecipient(recipient string) error {
	_, ok := s.mboxes[recipient]
	if !ok {
		return ErrNotFound
	}
	return nil
}

// Send implements Sender by sending a ballot to the intended recipient.
func (s *memSender) Send(ballot Ballot) error {
	mbox, ok := s.mboxes[ballot.Recipient]
	if !ok {
		return ErrNotFound
	}
	select {
	case mbox <- ballot:
		return nil
	}
	return errgo.Newf("%q isn't receiving messages", ballot.Recipient)
}

// Close releases all resources used by the Sender, and unregisters all
// recipients.
func (s *memSender) Close() {
	for _, mbox := range s.mboxes {
		close(mbox)
	}
	s.mboxes = map[string]memMbox{}
}

// Register registers a recipient with the Sender.
func (s *memSender) Register(recipient string, handler func(Ballot) error) error {
	mbox, ok := s.mboxes[recipient]
	if ok {
		close(mbox)
	}
	mbox = make(memMbox)
	s.mboxes[recipient] = mbox
	go func() {
		for {
			select {
			case ballot, ok := <-mbox:
				if !ok {
					return
				}
				err := handler(ballot)
				if err != nil {
					log.Println("error handling ballot for recipient %q: %v", recipient, errgo.Details(err))
				}
			}
		}
	}()
	return nil
}
