package migrations

import (
	"fmt"
	"pixelpunk/internal/models"
	"pixelpunk/pkg/logger"

	"gorm.io/gorm"
)

// UpdateUserStorageOptionsForAdminUsage 统一已登录用户存储时长选项：3d/7d/30d/permanent。
func UpdateUserStorageOptionsForAdminUsage(db *gorm.DB) error {
	logger.Infof("执行迁移: update_user_storage_options_for_admin_usage")

	return db.Transaction(func(tx *gorm.DB) error {
		if err := upsertUserUploadSetting(tx, models.Setting{
			Key:         "user_allowed_storage_durations",
			Value:       `["3d","7d","30d","permanent"]`,
			Type:        "array",
			Group:       "upload",
			Description: "已登录用户可选存储时长",
			IsSystem:    true,
		}); err != nil {
			return fmt.Errorf("更新 user_allowed_storage_durations 失败: %w", err)
		}

		if err := upsertUserUploadSetting(tx, models.Setting{
			Key:         "user_default_storage_duration",
			Value:       "permanent",
			Type:        "string",
			Group:       "upload",
			Description: "已登录用户默认存储时长",
			IsSystem:    true,
		}); err != nil {
			return fmt.Errorf("更新 user_default_storage_duration 失败: %w", err)
		}

		return nil
	})
}

func upsertUserUploadSetting(tx *gorm.DB, target models.Setting) error {
	var existing models.Setting
	err := tx.Where("key = ?", target.Key).First(&existing).Error
	if err == nil {
		existing.Value = target.Value
		existing.Type = target.Type
		existing.Group = target.Group
		existing.Description = target.Description
		existing.IsSystem = target.IsSystem
		return tx.Save(&existing).Error
	}

	if err == gorm.ErrRecordNotFound {
		return tx.Create(&target).Error
	}

	return err
}
