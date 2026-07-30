package storage

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UploadedFile represents file metadata tracked in the agency portal database.
type UploadedFile struct {
	Key           string `gorm:"primaryKey;column:key"`
	UploadedBy    string `gorm:"column:uploaded_by"`
	CompanyID     string `gorm:"column:company_id"`
	TaskID        string `gorm:"column:task_id"`
	ConsignmentID string `gorm:"column:consignment_id"`
}

func (UploadedFile) TableName() string {
	return "uploaded_files"
}

// FileMetadataRecord contains ownership and workflow details for authorization evaluation.
type FileMetadataRecord struct {
	Key           string
	UploadedBy    string
	CompanyID     string
	TaskID        string
	ConsignmentID string
}

// KeyValidator defines authorization and key verification methods for storage operations.
type KeyValidator interface {
	KeyExists(ctx context.Context, key string) (bool, error)
	GetFileMetadata(ctx context.Context, key string) (*FileMetadataRecord, error)
	TrackUpload(ctx context.Context, file UploadedFile) error
	CanAccessFile(ctx context.Context, key string, userID string, companyID string, roles []string) (bool, error)
}

// Store manages database interactions for storage file tracking.
type Store struct {
	db *gorm.DB
}

// NewStore creates a new storage Store instance.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// KeyExists checks if a storage key is linked to an application or tracked upload.
func (s *Store) KeyExists(ctx context.Context, key string) (bool, error) {
	var count int64
	// Check application_files
	err := s.db.WithContext(ctx).
		Table("application_files").
		Where("file_key = ?", key).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}

	// Check uploaded_files
	err = s.db.WithContext(ctx).
		Model(&UploadedFile{}).
		Where("key = ?", key).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetFileMetadata retrieves stored metadata for a file key.
func (s *Store) GetFileMetadata(ctx context.Context, key string) (*FileMetadataRecord, error) {
	var record UploadedFile
	err := s.db.WithContext(ctx).Where("key = ?", key).First(&record).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &FileMetadataRecord{
		Key:           record.Key,
		UploadedBy:    record.UploadedBy,
		CompanyID:     record.CompanyID,
		TaskID:        record.TaskID,
		ConsignmentID: record.ConsignmentID,
	}, nil
}

// TrackUpload stores file metadata when an upload URL is generated.
func (s *Store) TrackUpload(ctx context.Context, file UploadedFile) error {
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&file).Error
}

// isOfficerRole checks whether any of the user's roles represent an Agency Officer.
// "Officer" and "AGENCY_OFFICER" are treated as the same role.
func isOfficerRole(roles []string) bool {
	if len(roles) == 0 {
		// Default to officer in agency portal if authenticated without explicit trader role
		return true
	}
	for _, r := range roles {
		switch r {
		case "Officer", "AGENCY_OFFICER", "Admin", "reviewer", "lab_officer", "lab_manager":
			return true
		}
	}
	return false
}

// CanAccessFile evaluates whether a caller (Agency Officer or Trader) is authorized to access a file.
//
// Authorization logic:
//  1. Fetch file metadata (or check key existence in application/uploaded files).
//  2. If caller is an Agency Officer (Officer / AGENCY_OFFICER / Admin):
//     Allow if the file exists within the agency's workflow system.
//  3. If caller is a Trader:
//     Allow if caller's company_id matches the file's company_id OR caller's user_id matches uploaded_by.
//  4. Deny with 403 if access criteria are not satisfied.
func (s *Store) CanAccessFile(ctx context.Context, key string, userID string, companyID string, roles []string) (bool, error) {
	// First verify the file exists in the system
	exists, err := s.KeyExists(ctx, key)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	// Agency Officers can access any file in the agency workflow
	if isOfficerRole(roles) {
		return true, nil
	}

	// For Traders: evaluate ownership against metadata
	meta, err := s.GetFileMetadata(ctx, key)
	if err != nil {
		return false, err
	}
	if meta != nil {
		if companyID != "" && meta.CompanyID == companyID {
			return true, nil
		}
		if userID != "" && meta.UploadedBy == userID {
			return true, nil
		}
	}

	// Check application_files join for trader company matching
	if companyID != "" {
		var count int64
		err = s.db.WithContext(ctx).
			Table("application_files").
			Joins("JOIN applications ON applications.task_id = application_files.application_id").
			Where("application_files.file_key = ? AND applications.consignment_id = ?", key, companyID).
			Count(&count).Error
		if err == nil && count > 0 {
			return true, nil
		}
	}

	return false, nil
}
