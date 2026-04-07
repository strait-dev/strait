package notify

import (
	"context"
	"fmt"
	"strings"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awsv2config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// EmailProviderAttempt captures provider-specific send settings.
type EmailProviderAttempt struct {
	Provider string

	FromEmail string

	// SES fields.
	Region           string
	ConfigurationSet string
	AccessKeyID      string
	SecretAccessKey  string
	SessionToken     string
}

// SendEmailWithProvider sends one email using the selected provider.
func SendEmailWithProvider(
	ctx context.Context,
	messageID,
	projectID,
	to,
	subject,
	htmlBody,
	textBody string,
	attempt EmailProviderAttempt,
) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(attempt.Provider))
	if provider == "" {
		provider = "ses"
	}

	switch provider {
	case "ses":
		return sendEmailWithSES(ctx, messageID, projectID, to, subject, htmlBody, textBody, attempt)
	default:
		return "", fmt.Errorf("unsupported email provider: %s", attempt.Provider)
	}
}

func sendEmailWithSES(
	ctx context.Context,
	messageID,
	projectID,
	to,
	subject,
	htmlBody,
	textBody string,
	attempt EmailProviderAttempt,
) (string, error) {
	if strings.TrimSpace(attempt.FromEmail) == "" {
		return "", fmt.Errorf("ses from email is required")
	}
	if strings.TrimSpace(htmlBody) == "" && strings.TrimSpace(textBody) == "" {
		return "", fmt.Errorf("email body is required")
	}

	loadOptions := []func(*awsv2config.LoadOptions) error{}
	if strings.TrimSpace(attempt.Region) != "" {
		loadOptions = append(loadOptions, awsv2config.WithRegion(attempt.Region))
	}
	if strings.TrimSpace(attempt.AccessKeyID) != "" || strings.TrimSpace(attempt.SecretAccessKey) != "" {
		loadOptions = append(loadOptions, awsv2config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			attempt.AccessKeyID,
			attempt.SecretAccessKey,
			attempt.SessionToken,
		)))
	}

	awsCfg, err := awsv2config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return "", fmt.Errorf("load aws config for ses: %w", err)
	}

	body := &sestypes.Body{}
	if strings.TrimSpace(htmlBody) != "" {
		body.Html = &sestypes.Content{Data: awsv2.String(htmlBody), Charset: awsv2.String("UTF-8")}
	}
	if strings.TrimSpace(textBody) != "" {
		body.Text = &sestypes.Content{Data: awsv2.String(textBody), Charset: awsv2.String("UTF-8")}
	}

	input := &sesv2.SendEmailInput{
		FromEmailAddress: awsv2.String(attempt.FromEmail),
		Destination: &sestypes.Destination{
			ToAddresses: []string{to},
		},
		Content: &sestypes.EmailContent{
			Simple: &sestypes.Message{
				Subject: &sestypes.Content{Data: awsv2.String(subject), Charset: awsv2.String("UTF-8")},
				Body:    body,
			},
		},
		EmailTags: []sestypes.MessageTag{
			{Name: awsv2.String("strait_message_id"), Value: awsv2.String(messageID)},
			{Name: awsv2.String("strait_project_id"), Value: awsv2.String(projectID)},
		},
	}
	if strings.TrimSpace(attempt.ConfigurationSet) != "" {
		input.ConfigurationSetName = awsv2.String(attempt.ConfigurationSet)
	}

	client := sesv2.NewFromConfig(awsCfg)
	resp, err := client.SendEmail(ctx, input)
	if err != nil {
		return "", fmt.Errorf("send email (ses): %w", err)
	}

	return awsv2.ToString(resp.MessageId), nil
}
