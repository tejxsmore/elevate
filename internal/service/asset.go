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
	cfg       *config.Config
	repo      *repository.AssetRepo
	campaigns *repository.CampaignRepo
	s3        *s3.Client
}

type AssetReference struct {
	Asset models.Asset
	URL   string
}

type AssetObject struct {
	Asset         models.Asset
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
}

func NewAssetService(
	cfg *config.Config,
	repo *repository.AssetRepo,
	campaigns *repository.CampaignRepo,
	s3Client *s3.Client,
) *AssetService {
	return &AssetService{
		cfg:       cfg,
		repo:      repo,
		campaigns: campaigns,
		s3:        s3Client,
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

		if value := mime.TypeByExtension(ext); value != "" {
			contentType = value
		}
	}

	if contentType == "" {
		contentType = "application/octet-stream"
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
			ContentLength: aws.Int64(size),
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
		_, deleteErr := s.s3.DeleteObject(
			ctx,
			&s3.DeleteObjectInput{
				Bucket: aws.String(
					s.cfg.AWS.S3Bucket,
				),
				Key: aws.String(
					objectKey,
				),
			},
		)

		if deleteErr != nil {
			return models.Asset{}, fmt.Errorf(
				"create asset record: %w; cleanup uploaded S3 object: %v",
				err,
				deleteErr,
			)
		}

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

func (s *AssetService) Open(
	ctx context.Context,
	id uuid.UUID,
) (AssetObject, error) {
	if s == nil ||
		s.repo == nil ||
		s.s3 == nil {
		return AssetObject{}, fmt.Errorf(
			"asset service is not configured",
		)
	}

	asset, err := s.repo.Get(
		ctx,
		id,
	)
	if err != nil {
		return AssetObject{}, err
	}

	if strings.ToLower(
		strings.TrimSpace(
			asset.StorageProvider,
		),
	) != "s3" {
		return AssetObject{}, fmt.Errorf(
			"unsupported asset storage provider: %s",
			asset.StorageProvider,
		)
	}

	if s.cfg == nil ||
		strings.TrimSpace(
			s.cfg.AWS.S3Bucket,
		) == "" {
		return AssetObject{}, fmt.Errorf(
			"S3 asset storage is not configured",
		)
	}

	storagePath := strings.TrimLeft(
		strings.TrimSpace(
			asset.StoragePath,
		),
		"/",
	)

	if storagePath == "" {
		return AssetObject{}, fmt.Errorf(
			"asset storage path is empty",
		)
	}

	object, err := s.s3.GetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(
				s.cfg.AWS.S3Bucket,
			),
			Key: aws.String(
				storagePath,
			),
		},
	)
	if err != nil {
		return AssetObject{}, fmt.Errorf(
			"get asset from S3: %w",
			err,
		)
	}

	contentType := "application/octet-stream"

	if asset.MimeType != nil &&
		strings.TrimSpace(
			*asset.MimeType,
		) != "" {
		contentType = strings.TrimSpace(
			*asset.MimeType,
		)
	} else if object.ContentType != nil &&
		strings.TrimSpace(
			*object.ContentType,
		) != "" {
		contentType = strings.TrimSpace(
			*object.ContentType,
		)
	}

	contentLength := int64(-1)

	if object.ContentLength != nil {
		contentLength = *object.ContentLength
	} else if asset.SizeBytes != nil {
		contentLength = *asset.SizeBytes
	}

	return AssetObject{
		Asset:         asset,
		Body:          object.Body,
		ContentType:   contentType,
		ContentLength: contentLength,
	}, nil
}

func (s *AssetService) Delete(
	ctx context.Context,
	id uuid.UUID,
) error {
	if s == nil ||
		s.repo == nil {
		return fmt.Errorf(
			"asset service is not configured",
		)
	}

	asset, err := s.repo.Get(
		ctx,
		id,
	)
	if err != nil {
		return err
	}

	storagePath := strings.TrimLeft(
		strings.TrimSpace(
			asset.StoragePath,
		),
		"/",
	)

	if storagePath == "" {
		return fmt.Errorf(
			"asset storage path is empty",
		)
	}

	if s.campaigns != nil {
		if err := s.campaigns.ClearAssetReference(
			ctx,
			id,
		); err != nil {
			return fmt.Errorf(
				"detach asset from campaigns: %w",
				err,
			)
		}
	}

	switch strings.ToLower(
		strings.TrimSpace(
			asset.StorageProvider,
		),
	) {
	case "s3":
		if s.s3 == nil ||
			s.cfg == nil ||
			strings.TrimSpace(
				s.cfg.AWS.S3Bucket,
			) == "" {
			return fmt.Errorf(
				"S3 asset storage is not configured",
			)
		}

		_, err := s.s3.DeleteObject(
			ctx,
			&s3.DeleteObjectInput{
				Bucket: aws.String(
					s.cfg.AWS.S3Bucket,
				),
				Key: aws.String(
					storagePath,
				),
			},
		)
		if err != nil {
			return fmt.Errorf(
				"delete asset from S3: %w",
				err,
			)
		}

	case "supabase":
		return fmt.Errorf(
			"deleting Supabase assets is not supported",
		)

	default:
		return fmt.Errorf(
			"unsupported asset storage provider: %s",
			asset.StorageProvider,
		)
	}

	if err := s.repo.Delete(
		ctx,
		id,
	); err != nil {
		return fmt.Errorf(
			"delete asset record: %w",
			err,
		)
	}

	return nil
}

func (s *AssetService) AttachedCampaigns(
	ctx context.Context,
	id uuid.UUID,
) ([]repository.CampaignSummary, error) {
	if s == nil ||
		s.campaigns == nil {
		return nil, fmt.Errorf(
			"asset service is not configured",
		)
	}

	return s.campaigns.FindByAssetID(
		ctx,
		id,
	)
}

func (s *AssetService) Reference(
	ctx context.Context,
	id uuid.UUID,
) (AssetReference, error) {
	asset, err := s.repo.Get(
		ctx,
		id,
	)
	if err != nil {
		return AssetReference{}, err
	}

	return AssetReference{
		Asset: asset,
	}, nil
}

func (s *AssetService) PublicURL(
	asset models.Asset,
) (string, error) {
	if asset.ID == uuid.Nil {
		return "", fmt.Errorf(
			"asset ID is empty",
		)
	}

	return "", fmt.Errorf(
		"direct public asset URLs are disabled",
	)
}

func (s *AssetService) URL(
	ctx context.Context,
	asset models.Asset,
	expires time.Duration,
) (string, error) {
	return "", fmt.Errorf(
		"direct asset URLs are disabled",
	)
}
