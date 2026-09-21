package emailerror

import "fmt"

// SMTPReplyError retains the command stage and server explanation so a rejected
// sender cannot be mistaken for an invalid recipient just because both use 550.
type SMTPReplyError struct {
	Stage   string
	Code    int
	Message string
}

func (e *SMTPReplyError) Error() string {
	return fmt.Sprintf("%s rejected: %d %s", e.Stage, e.Code, e.Message)
}
