package helpers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/labstack/gommon/log"
	"github.com/resend/resend-go/v3"
)

func Notify(topic string, message string) {
	resp, err := http.Post(fmt.Sprintf("%s/%s", boot.Environment.NTFY, topic), "text/plain",
		strings.NewReader(message))

	if err != nil {
		log.Warnf("Failed to send notification: %v", err)
	}

	if resp != nil {
		log.Debugf("Notification sent with status code: %d", resp.StatusCode)
		defer func() { _ = resp.Body.Close() }()
	}
}

func ResendEmailVerificationTemplate(email string, token string) {
	client := resend.NewClient(boot.Environment.ResendApiKey)

	params := &resend.SendEmailRequest{
		From: "Verifications <verifications@auth.urx.ink>",
		To:   []string{email},
		Template: &resend.EmailTemplate{
			Id: "email-verification",
			Variables: map[string]any{
				"app_name":   "AUTH POC",
				"verify_url": fmt.Sprintf("%s/verification/%s", boot.Environment.URL, token),
				"token":      token,
			},
		},
	}

	sent, err := client.Emails.Send(params)
	if err != nil {
		log.Errorf("Failed to send email: %v", err)
		return
	}

	log.Infof("Email sent to %s with ID: %s", email, sent.Id)
}

func ResendPasswordResetTemplate(email, token string) {
	client := resend.NewClient(boot.Environment.ResendApiKey)

	params := &resend.SendEmailRequest{
		From: "Reset Password <resets@auth.urx.ink>",
		To:   []string{email},
		Template: &resend.EmailTemplate{
			Id: "password-reset-request",
			Variables: map[string]any{
				"app_name":  "AUTH POC",
				"reset_url": fmt.Sprintf("%s/reset/%s", boot.Environment.URL, token),
				"token":     token,
			},
		},
	}

	sent, err := client.Emails.Send(params)
	if err != nil {
		log.Errorf("Failed to send email: %v", err)
		return
	}

	log.Infof("Email sent to %s with ID: %s", email, sent.Id)
}
