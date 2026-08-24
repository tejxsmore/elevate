package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"elevate/internal/database"
	"elevate/internal/models"
)

type AssetRepo struct {
	db *database.DB
}

func NewAssetRepo(
	db *database.DB,
) *AssetRepo {
	return &AssetRepo{
		db: db,
	}
}

func (r *AssetRepo) Create(
	ctx context.Context,
	name string,
	assetType string,
	storageProvider string,
	storagePath string,
	mimeType *string,
	sizeBytes *int64,
) (models.Asset, error) {
	var asset models.Asset

	err := r.db.Pool.QueryRow(
		ctx,
		`
		INSERT INTO assets (
			name,
			asset_type,
			storage_provider,
			storage_path,
			mime_type,
			size_bytes
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6
		)
		RETURNING
			id,
			name,
			asset_type,
			storage_provider,
			storage_path,
			mime_type,
			size_bytes,
			created_at
		`,
		name,
		assetType,
		storageProvider,
		storagePath,
		mimeType,
		sizeBytes,
	).Scan(
		&asset.ID,
		&asset.Name,
		&asset.AssetType,
		&asset.StorageProvider,
		&asset.StoragePath,
		&asset.MimeType,
		&asset.SizeBytes,
		&asset.CreatedAt,
	)

	return asset, err
}

func (r *AssetRepo) Get(
	ctx context.Context,
	id uuid.UUID,
) (models.Asset, error) {
	var asset models.Asset

	err := r.db.Pool.QueryRow(
		ctx,
		`
		SELECT
			id,
			name,
			asset_type,
			storage_provider,
			storage_path,
			mime_type,
			size_bytes,
			created_at
		FROM assets
		WHERE id = $1
		`,
		id,
	).Scan(
		&asset.ID,
		&asset.Name,
		&asset.AssetType,
		&asset.StorageProvider,
		&asset.StoragePath,
		&asset.MimeType,
		&asset.SizeBytes,
		&asset.CreatedAt,
	)

	return asset, err
}

func (r *AssetRepo) List(
	ctx context.Context,
) ([]models.Asset, error) {
	rows, err := r.db.Pool.Query(
		ctx,
		`
		SELECT
			id,
			name,
			asset_type,
			storage_provider,
			storage_path,
			mime_type,
			size_bytes,
			created_at
		FROM assets
		ORDER BY created_at DESC
		`,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	assets := make(
		[]models.Asset,
		0,
	)

	for rows.Next() {
		var asset models.Asset

		if err := rows.Scan(
			&asset.ID,
			&asset.Name,
			&asset.AssetType,
			&asset.StorageProvider,
			&asset.StoragePath,
			&asset.MimeType,
			&asset.SizeBytes,
			&asset.CreatedAt,
		); err != nil {
			return nil, err
		}

		assets = append(
			assets,
			asset,
		)
	}

	return assets, rows.Err()
}

func (r *AssetRepo) Delete(
	ctx context.Context,
	id uuid.UUID,
) error {
	tag, err := r.db.Pool.Exec(
		ctx,
		`
		DELETE FROM assets
		WHERE id = $1
		`,
		id,
	)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
