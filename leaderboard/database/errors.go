package database

import "fmt"

// GeneralError create a redis error that is not handled
type GeneralError struct {
	msg string
}

func (ue *GeneralError) Error() string {
	return fmt.Sprintf("database error: %s", ue.msg)
}

// NewGeneralError create a new redis error that isnt handled
func NewGeneralError(msg string) *GeneralError {
	return &GeneralError{msg: msg}
}

// InvalidOrderError is an error when an invalid order was gave
type InvalidOrderError struct {
	order string
}

func (ioe *InvalidOrderError) Error() string {
	return fmt.Sprintf("invalid order: %s", ioe.order)
}

// NewInvalidOrderError create a new InvalidOrderError
func NewInvalidOrderError(order string) *InvalidOrderError {
	return &InvalidOrderError{
		order: order,
	}
}

// MemberNotFoundError is an error throw when leaderboard not have member
type MemberNotFoundError struct {
	leaderboard string
	member      string
}

func (mnfe *MemberNotFoundError) Error() string {
	return fmt.Sprintf("member %s not found in leaderboard %s", mnfe.member, mnfe.leaderboard)
}

// NewMemberNotFoundError create a new MemberNotFoundError
func NewMemberNotFoundError(leaderboard, member string) *MemberNotFoundError {
	return &MemberNotFoundError{
		leaderboard: leaderboard,
		member:      member,
	}
}
