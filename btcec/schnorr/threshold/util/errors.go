package util

type ErrMsgParse string

func (e ErrMsgParse) Error() string {

	if e == "" {
		return "Message parse error"
	}

	return string(e)
}

type ErrFaultyCoordinator string

func (e ErrFaultyCoordinator) Error() string {

	if e == "" {
		return "Faulty coordinator error"
	}

	return string(e)
}

type ErrFaultyParticipantOrCoordinator struct {
	Msg         string
	Participant int
}

func (e ErrFaultyParticipantOrCoordinator) Error() string {

	if e.Msg == "" {
		return "Faulty participant or coordinator error"
	}

	return e.Msg
}
