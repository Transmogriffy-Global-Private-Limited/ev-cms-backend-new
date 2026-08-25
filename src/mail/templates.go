package mail

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
	texttemplate "text/template"
)

//go:embed templates/message.html.tmpl templates/message.txt.tmpl
var messageTemplates embed.FS

type renderedMessage struct {
	RecipientName string
	Body          string
}

func renderMessageTemplates(recipientName, body string) (string, string, error) {
	if strings.TrimSpace(recipientName) == "" {
		recipientName = "there"
	}
	data := renderedMessage{RecipientName: recipientName, Body: body}
	text, err := texttemplate.ParseFS(messageTemplates, "templates/message.txt.tmpl")
	if err != nil {
		return "", "", fmt.Errorf("parse text mail template: %w", err)
	}
	html, err := template.ParseFS(messageTemplates, "templates/message.html.tmpl")
	if err != nil {
		return "", "", fmt.Errorf("parse HTML mail template: %w", err)
	}
	var textBody, htmlBody bytes.Buffer
	if err := text.Execute(&textBody, data); err != nil {
		return "", "", fmt.Errorf("render text mail template: %w", err)
	}
	if err := html.Execute(&htmlBody, data); err != nil {
		return "", "", fmt.Errorf("render HTML mail template: %w", err)
	}
	return textBody.String(), htmlBody.String(), nil
}
