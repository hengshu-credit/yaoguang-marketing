package service

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/emailerror"
	"github.com/stretchr/testify/require"
)

func TestSMTPRejectionsReachClassifierWithProviderReason(t *testing.T) {
	for _, stage := range []string{"MAIL FROM", "RCPT TO", "DATA"} {
		t.Run(stage, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				fmt.Fprint(conn, "220 local test server\r\n")
				scanner := bufio.NewScanner(conn)
				for scanner.Scan() {
					line := scanner.Text()
					if strings.HasPrefix(line, stage) {
						fmt.Fprint(conn, "550-Provider rejected request\r\n550 5.1.1 Detailed rejection\r\n")
						return
					}
					fmt.Fprint(conn, "250 OK\r\n")
				}
			}()
			port := listener.Addr().(*net.TCPAddr).Port
			err = sendRawEmailWithSettings(&domain.SMTPSettings{Host: "127.0.0.1", Port: port},
				"sender@example.com", []string{"recipient@example.com"}, []byte("body"), nil)
			require.Error(t, err)
			var reply *emailerror.SMTPReplyError
			require.ErrorAs(t, err, &reply)
			require.Equal(t, stage, reply.Stage)
			require.Equal(t, 550, reply.Code)
			require.Contains(t, reply.Message, "Detailed rejection")
			classified := emailerror.NewClassifier().Classify(err, domain.EmailProviderKindSMTP)
			if stage == "RCPT TO" {
				require.Equal(t, emailerror.ErrorTypeRecipient, classified.Type)
			} else {
				require.Equal(t, emailerror.ErrorTypeProvider, classified.Type)
			}
			require.False(t, classified.Retryable)
			<-done
		})
	}
}
