package service

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	"elevate/internal/config"
	"elevate/internal/models"
	"elevate/internal/repository"
)

type AssetService struct {
	cfg     *config.Config
	repo    *repository.AssetRepo
	s3      *s3.Client
	presign *s3.PresignClient
}

type AssetReference struct {
	Asset models.Asset
	URL   string
}

func NewAssetService(
	cfg *config.Config,
	repo *repository.AssetRepo,
	s3Client *s3.Client,
) *AssetService {
	var presign *s3.PresignClient

	if s3Client != nil {
		presign = s3.NewPresignClient(
			s3Client,
		)
	}

	return &AssetService{
		cfg:     cfg,
		repo:    repo,
		s3:      s3Client,
		presign: presign,
	}
}

func (s *AssetService) Upload(
	ctx context.Context,
	name string,
	assetType string,
	filename string,
	contentType string,
	size int64,
	reader io.Reader,
) (models.Asset, error) {
	if s == nil || s.repo == nil {
		return models.Asset{}, fmt.Errorf(
			"asset service is not configured",
		)
	}

	name = strings.TrimSpace(name)
	assetType = strings.ToLower(
		strings.TrimSpace(assetType),
	)
	filename = strings.TrimSpace(filename)
	contentType = strings.TrimSpace(contentType)

	if name == "" {
		return models.Asset{}, fmt.Errorf(
			"asset name is required",
		)
	}

	if assetType == "" {
		return models.Asset{}, fmt.Errorf(
			"asset type is required",
		)
	}

	if filename == "" {
		return models.Asset{}, fmt.Errorf(
			"asset filename is required",
		)
	}

	if size < 0 {
		return models.Asset{}, fmt.Errorf(
			"asset size is invalid",
		)
	}

	if contentType == "" {
		ext := strings.ToLower(
			path.Ext(filename),
		)

		if value := mime.TypeByExtension(
			ext,
		); value != "" {
			contentType = value
		}
	}

	if contentType == "" {
		contentType =
			"application/octet-stream"
	}

	if s.s3 == nil ||
		s.cfg == nil ||
		strings.TrimSpace(
			s.cfg.AWS.S3Bucket,
		) == "" {
		return models.Asset{}, fmt.Errorf(
			"S3 asset storage is not configured",
		)
	}

	extension := strings.ToLower(
		path.Ext(filename),
	)

	objectID := uuid.New()

	objectKey := path.Join(
		"assets",
		assetType,
		objectID.String()+extension,
	)

	_, err := s.s3.PutObject(
		ctx,
		&s3.PutObjectInput{
			Bucket: aws.String(
				s.cfg.AWS.S3Bucket,
			),
			Key: aws.String(
				objectKey,
			),
			Body: reader,
			ContentType: aws.String(
				contentType,
			),
		},
	)
	if err != nil {
		return models.Asset{}, fmt.Errorf(
			"upload asset to S3: %w",
			err,
		)
	}

	asset, err := s.repo.Create(
		ctx,
		name,
		assetType,
		"s3",
		objectKey,
		&contentType,
		&size,
	)
	if err != nil {
		return models.Asset{}, fmt.Errorf(
			"create asset record: %w",
			err,
		)
	}

	return asset, nil
}

func (s *AssetService) Get(
	ctx context.Context,
	id uuid.UUID,
) (models.Asset, error) {
	if s == nil || s.repo == nil {
		return models.Asset{}, fmt.Errorf(
			"asset service is not configured",
		)
	}

	return s.repo.Get(
		ctx,
		id,
	)
}

func (s *AssetService) List(
	ctx context.Context,
) ([]models.Asset, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf(
			"asset service is not configured",
		)
	}

	return s.repo.List(
		ctx,
	)
}

func (s *AssetService) Delete(
	ctx context.Context,
	id uuid.UUID,
) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf(
			"asset service is not configured",
		)
	}

	return s.repo.Delete(
		ctx,
		id,
	)
}

func (s *AssetService) Reference(
	ctx context.Context,
	id uuid.UUID,
) (AssetReference, error) {
	if s == nil || s.repo == nil {
		return AssetReference{}, fmt.Errorf(
			"asset service is not configured",
		)
	}

	asset, err := s.repo.Get(
		ctx,
		id,
	)
	if err != nil {
		return AssetReference{}, err
	}

	url, err := s.URL(
		ctx,
		asset,
		15*time.Minute,
	)
	if err != nil {
		return AssetReference{}, err
	}

	return AssetReference{
		Asset: asset,
		URL:   url,
	}, nil
}

func (s *AssetService) URL(
	ctx context.Context,
	asset models.Asset,
	expires time.Duration,
) (string, error) {
	if asset.ID == uuid.Nil {
		return "", fmt.Errorf(
			"asset ID is empty",
		)
	}

	storagePath := strings.TrimLeft(
		strings.TrimSpace(
			asset.StoragePath,
		),
		"/",
	)

	if storagePath == "" {
		return "", fmt.Errorf(
			"asset storage path is empty",
		)
	}

	switch strings.ToLower(
		strings.TrimSpace(
			asset.StorageProvider,
		),
	) {
	case "s3":
		if s == nil ||
			s.cfg == nil ||
			strings.TrimSpace(
				s.cfg.AWS.S3Bucket,
			) == "" {
			return "", fmt.Errorf(
				"S3 asset storage is not configured",
			)
		}

		if s.presign == nil {
			return "", fmt.Errorf(
				"S3 presigner is not configured",
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
						s.cfg.AWS.S3Bucket,
					),
					Key: aws.String(
						storagePath,
					),
				},
				func(
					options *s3.PresignOptions,
				) {
					options.Expires = expires
				},
			)
		if err != nil {
			return "", fmt.Errorf(
				"presign S3 asset: %w",
				err,
			)
		}

		return request.URL, nil

	case "supabase":
		if s == nil ||
			s.cfg == nil ||
			strings.TrimSpace(
				s.cfg.Supabase.URL,
			) == "" ||
			strings.TrimSpace(
				s.cfg.Supabase.StorageBucket,
			) == "" {
			return "", fmt.Errorf(
				"Supabase asset storage is not configured",
			)
		}

		return fmt.Sprintf(
			"%s/storage/v1/object/public/%s/%s",
			strings.TrimRight(
				s.cfg.Supabase.URL,
				"/",
			),
			s.cfg.Supabase.StorageBucket,
			storagePath,
		), nil

	default:
		return "", fmt.Errorf(
			"unsupported asset storage provider: %s",
			asset.StorageProvider,
		)
	}
}

func (s *AssetService) PublicURL(
	asset models.Asset,
) (string, error) {
	return s.URL(
		context.Background(),
		asset,
		15*time.Minute,
	)
}
