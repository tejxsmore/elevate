package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	appconfig "elevate/internal/config"
)

type RecordingService struct {
	accountSID string
	authToken  string
	bucket     string
	region     string
	publicBase string
	apiBaseURL string
	httpClient *http.Client
	s3         *s3.Client
	presign    *s3.PresignClient
}

func NewRecordingService(
	ctx context.Context,
	cfg appconfig.AWSConfig,
	accountSID string,
	authToken string,
) (*RecordingService, error) {
	return NewRecordingServiceWithBaseURL(
		ctx,
		cfg,
		accountSID,
		authToken,
		"",
	)
}

func NewRecordingServiceWithBaseURL(
	ctx context.Context,
	cfg appconfig.AWSConfig,
	accountSID string,
	authToken string,
	apiBaseURL string,
) (*RecordingService, error) {
	apiBaseURL = strings.TrimRight(
		strings.TrimSpace(apiBaseURL),
		"/",
	)

	if strings.TrimSpace(
		cfg.S3Bucket,
	) == "" {
		return &RecordingService{
			accountSID: accountSID,
			authToken:  authToken,
			region:     cfg.Region,
			publicBase: strings.TrimRight(
				strings.TrimSpace(
					cfg.S3PublicBaseURL,
				),
				"/",
			),
			apiBaseURL: apiBaseURL,
			httpClient: &http.Client{
				Timeout: 60 * time.Second,
			},
		}, nil
	}

	awsCfg, err :=
		awsconfig.LoadDefaultConfig(
			ctx,
			awsconfig.WithRegion(
				cfg.Region,
			),
		)
	if err != nil {
		return nil, fmt.Errorf(
			"recording: load AWS config: %w",
			err,
		)
	}

	s3Client := s3.NewFromConfig(
		awsCfg,
	)

	return &RecordingService{
		accountSID: accountSID,
		authToken:  authToken,
		bucket:     cfg.S3Bucket,
		region:     cfg.Region,
		publicBase: strings.TrimRight(
			strings.TrimSpace(
				cfg.S3PublicBaseURL,
			),
			"/",
		),
		apiBaseURL: apiBaseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		s3: s3Client,
		presign: s3.NewPresignClient(
			s3Client,
		),
	}, nil
}

func (s *RecordingService) Enabled() bool {
	return s != nil &&
		s.s3 != nil &&
		strings.TrimSpace(
			s.bucket,
		) != ""
}

func (s *RecordingService) StoreTwilioRecording(
	ctx context.Context,
	callID uuid.UUID,
	recordingSID string,
	recordingURL string,
) (string, error) {
	if !s.Enabled() {
		return "", fmt.Errorf(
			"recording storage is not configured",
		)
	}

	recordingSID = strings.TrimSpace(
		recordingSID,
	)

	recordingURL = strings.TrimSpace(
		recordingURL,
	)

	if recordingSID == "" {
		return "", fmt.Errorf(
			"recording SID is empty",
		)
	}

	if recordingURL == "" {
		return "", fmt.Errorf(
			"recording URL is empty",
		)
	}

	resp, err := s.downloadTwilioRecording(
		ctx,
		recordingURL,
	)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	audio, err := io.ReadAll(
		resp.Body,
	)
	if err != nil {
		return "", fmt.Errorf(
			"recording: read Twilio recording: %w",
			err,
		)
	}

	if len(audio) == 0 {
		return "", fmt.Errorf(
			"recording: Twilio returned an empty recording",
		)
	}

	key := path.Join(
		"recordings",
		callID.String(),
		recordingSID+".mp3",
	)

	contentType := strings.TrimSpace(
		resp.Header.Get(
			"Content-Type",
		),
	)

	if contentType == "" {
		contentType = "audio/mpeg"
	}

	contentLength := int64(
		len(audio),
	)

	_, err = s.s3.PutObject(
		ctx,
		&s3.PutObjectInput{
			Bucket: aws.String(
				s.bucket,
			),
			Key: aws.String(
				key,
			),
			Body: bytes.NewReader(
				audio,
			),
			ContentLength: &contentLength,
			ContentType: aws.String(
				contentType,
			),
		},
	)
	if err != nil {
		return "", fmt.Errorf(
			"recording: upload to S3: %w",
			err,
		)
	}

	if s.apiBaseURL != "" {
		return fmt.Sprintf(
			"%s/api/v1/calls/%s/recording",
			s.apiBaseURL,
			callID.String(),
		), nil
	}

	return key, nil
}

func (s *RecordingService) SignedURL(
	ctx context.Context,
	recordingURL string,
	expires time.Duration,
) (string, error) {
	if !s.Enabled() ||
		s.presign == nil {
		return "", fmt.Errorf(
			"recording storage is not configured",
		)
	}

	key := strings.TrimSpace(
		recordingURL,
	)

	if key == "" {
		return "", fmt.Errorf(
			"recording key is empty",
		)
	}

	if strings.HasPrefix(
		key,
		"http://",
	) || strings.HasPrefix(
		key,
		"https://",
	) {
		marker := "/recordings/"

		index := strings.Index(
			key,
			marker,
		)

		if index < 0 {
			return "", fmt.Errorf(
				"recording URL does not contain a valid recording key",
			)
		}

		key = strings.TrimPrefix(
			key[index:],
			"/",
		)
	}

	if expires <= 0 {
		expires = 15 * time.Minute
	}

	request, err :=
		s.presign.PresignGetObject(
			ctx,
			&s3.GetObjectInput{
				Bucket: aws.String(
					s.bucket,
				),
				Key: aws.String(
					key,
				),
			},
			func(options *s3.PresignOptions) {
				options.Expires = expires
			},
		)
	if err != nil {
		return "", fmt.Errorf(
			"presign recording: %w",
			err,
		)
	}

	return request.URL, nil
}

func (s *RecordingService) downloadTwilioRecording(
	ctx context.Context,
	recordingURL string,
) (*http.Response, error) {
	u := strings.TrimRight(
		recordingURL,
		"/",
	) + ".mp3"

	req, err :=
		http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			u,
			nil,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"recording: build Twilio request: %w",
			err,
		)
	}

	req.SetBasicAuth(
		s.accountSID,
		s.authToken,
	)

	resp, err := s.httpClient.Do(
		req,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"recording: download Twilio recording: %w",
			err,
		)
	}

	if resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(
			io.LimitReader(
				resp.Body,
				4096,
			),
		)

		_ = resp.Body.Close()

		return nil, fmt.Errorf(
			"recording: Twilio returned %d: %s",
			resp.StatusCode,
			strings.TrimSpace(
				string(body),
			),
		)
	}

	return resp, nil
}
