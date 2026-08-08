package mail

import (
	"ai-novel-ide/be/internal/config"
	"fmt"
	"strings"
)

type Sender interface {
	Send(to string, subject string, textBody string, htmlBody string) error
}

func NewSender(cfg config.MailConfig) (Sender, error) {
	switch strings.TrimSpace(cfg.Provider) {
	case "smtp":
		if strings.TrimSpace(cfg.SMTP.Host) == "" {
			return nil, fmt.Errorf("mail.smtp.host 不能为空")
		}
		if cfg.SMTP.Port <= 0 {
			return nil, fmt.Errorf("mail.smtp.port 必须大于 0")
		}
		if strings.TrimSpace(cfg.SMTP.Username) == "" {
			return nil, fmt.Errorf("mail.smtp.username 不能为空")
		}
		if strings.TrimSpace(cfg.SMTP.Password) == "" {
			return nil, fmt.Errorf("mail.smtp.password 不能为空")
		}
		return NewSMTPSender(
			cfg.SMTP.Host,
			cfg.SMTP.Port,
			cfg.SMTP.Username,
			cfg.SMTP.Password,
		), nil
	case "resend":
		if strings.TrimSpace(cfg.Resend.ApiKey) == "" {
			return nil, fmt.Errorf("mail.resend.api_key 不能为空")
		}
		if strings.TrimSpace(cfg.Resend.From) == "" {
			return nil, fmt.Errorf("mail.resend.from 不能为空")
		}
		return NewResendSender(cfg.Resend.ApiKey, cfg.Resend.From), nil
	default:
		return nil, fmt.Errorf("mail.provider 必须是 smtp 或 resend")
	}
}
